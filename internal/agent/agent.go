// Package agent implements an opt-in, read-only filesystem export handler.
//
// The handler is a data plane, not a network listener. Its caller MUST bind it
// behind an authenticated private mesh or TLS gateway. A bearer token protects
// every endpoint but does not make cleartext HTTP safe on an untrusted network.
//
// This package requires Go 1.26.5 or newer: earlier os.Root releases have
// security bugs affecting path confinement on some platforms. It deliberately
// uses os.Root rather than string-prefix checks to prevent symlink races.
package agent

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPageSize = 100
	maxPageSize     = 200
	maxPageOffset   = 10000
)

// Export names a directory explicitly shared by the device's owner. Paths
// never appear in the wire protocol. No drives are exported automatically.
type Export struct {
	ID   string
	Name string
	Path string
}

// Share is safe to return to a remote peer; no machine-local path is included.
type Share struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Entry describes a single immediate child. A symlink is reported but its
// target is never included. Only regular files can be downloaded.
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Type    string    `json:"type"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modified_at"`
}

type service struct {
	shares []Share
	exports map[string]Export
	token  [sha256.Size]byte
}

// NewHandler validates the explicit exports and returns a fully authenticated
// HTTP handler. It does not listen on a socket or grant network access itself.
// Keep the bearer token secret and generate it with a cryptographic RNG.
func NewHandler(exports []Export, bearerToken string) (http.Handler, error) {
	if len(bearerToken) < 32 || len(bearerToken) > 4096 || strings.TrimSpace(bearerToken) != bearerToken || strings.ContainsAny(bearerToken, "\r\n\t ") {
		return nil, errors.New("agent: bearer token must contain at least 32 non-whitespace bytes")
	}

	s := &service{exports: make(map[string]Export, len(exports)), token: sha256.Sum256([]byte(bearerToken))}
	for _, e := range exports {
		if !validID(e.ID) || strings.TrimSpace(e.Name) == "" || len(e.Name) > 128 || !filepath.IsAbs(e.Path) {
			return nil, fmt.Errorf("agent: invalid export configuration for %q", e.ID)
		}
		if _, exists := s.exports[e.ID]; exists {
			return nil, fmt.Errorf("agent: duplicate export ID %q", e.ID)
		}
		root, err := os.OpenRoot(e.Path)
		if err != nil {
			return nil, fmt.Errorf("agent: cannot open export %q: %w", e.ID, err)
		}
		info, statErr := root.Stat(".")
		closeErr := root.Close()
		if statErr != nil || !info.IsDir() || closeErr != nil {
			return nil, fmt.Errorf("agent: export %q is not an accessible directory", e.ID)
		}
		s.exports[e.ID] = e
		s.shares = append(s.shares, Share{ID: e.ID, Name: e.Name})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/shares", s.getShares)
	mux.HandleFunc("GET /v1/list", s.list)
	mux.HandleFunc("GET /v1/download", s.download)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="MasterDrive Agent"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (s *service) authorized(r *http.Request) bool {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) > 4103 {
		return false
	}
	scheme, received, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || received == "" || strings.ContainsAny(received, "\r\n\t ") {
		return false
	}
	digest := sha256.Sum256([]byte(received))
	return subtle.ConstantTimeCompare(digest[:], s.token[:]) == 1
}

func (s *service) getShares(w http.ResponseWriter, _ *http.Request) {
	shares := s.shares
	if shares == nil {
		shares = []Share{}
	}
	writeJSON(w, http.StatusOK, struct {
		Shares []Share `json:"shares"`
	}{Shares: shares})
}

// localPath accepts only slash-delimited paths, independent of the host OS.
// fs.ValidPath rejects traversal, empty components, and trailing slashes.
// filepath.Localize additionally rejects Windows reserved device names.
func localPath(p string) (string, error) {
	if p == "" || p == "." {
		return ".", nil
	}
	if !fs.ValidPath(p) || strings.ContainsAny(p, "\\\x00") {
		return "", errors.New("invalid path")
	}
	return filepath.Localize(p)
}

func (s *service) open(w http.ResponseWriter, r *http.Request) (*os.Root, *os.File, string, string, bool) {
	id := r.URL.Query().Get("share")
	e, exists := s.exports[id]
	if !exists {
		writeError(w, http.StatusNotFound, "share not found")
		return nil, nil, "", "", false
	}
	p := r.URL.Query().Get("path")
	local, err := localPath(p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return nil, nil, "", "", false
	}
	root, err := os.OpenRoot(e.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "path not available")
		return nil, nil, "", "", false
	}
	f, err := root.Open(local)
	if err != nil {
		root.Close()
		writeError(w, http.StatusNotFound, "path not available")
		return nil, nil, "", "", false
	}
	return root, f, id, p, true
}

func pageValues(r *http.Request) (limit, offset int, err error) {
	limit = defaultPageSize
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > maxPageSize {
			return 0, 0, errors.New("invalid limit")
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 || offset > maxPageOffset {
			return 0, 0, errors.New("invalid offset")
		}
	}
	return limit, offset, nil
}

func (s *service) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pageValues(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	root, f, id, p, ok := s.open(w, r)
	if !ok {
		return
	}
	defer root.Close()
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not a directory")
		return
	}

	// Cap both memory and CPU work per request. Directory offsets are best
	// effort: changes made while paging can reorder the underlying listing.
	children, err := f.ReadDir(offset + limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusInternalServerError, "unable to list directory")
		return
	}
	hasMore := len(children) > offset+limit
	if offset >= len(children) {
		children = nil
	} else {
		children = children[offset:]
		if len(children) > limit {
			children = children[:limit]
		}
	}

	entries := make([]Entry, 0, len(children))
	for _, child := range children {
		childPath := path.Join(p, child.Name())
		if childPath == "." || !fs.ValidPath(childPath) || strings.ContainsAny(childPath, "\\\x00") {
			// Never advertise entries that cannot be addressed through our
			// deliberately portable slash-delimited protocol.
			continue
		}
		childInfo, err := child.Info()
		if err != nil {
			continue // a concurrent filesystem mutation may remove an entry
		}
		kind := "other"
		switch {
		case child.Type().IsRegular():
			kind = "file"
		case child.IsDir():
			kind = "directory"
		case child.Type()&fs.ModeSymlink != 0:
			kind = "symlink"
		}
		entries = append(entries, Entry{Name: child.Name(), Path: childPath, Type: kind, Size: childInfo.Size(), ModTime: childInfo.ModTime()})
	}
	result := struct {
		Share      string  `json:"share"`
		Path       string  `json:"path"`
		Entries    []Entry `json:"entries"`
		NextOffset *int    `json:"next_offset"`
	}{Share: id, Path: p, Entries: entries}
	if hasMore {
		next := offset + limit
		result.NextOffset = &next
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *service) download(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("path") == "" || r.URL.Query().Get("path") == "." {
		writeError(w, http.StatusBadRequest, "file path required")
		return
	}
	root, f, _, p, ok := s.open(w, r)
	if !ok {
		return
	}
	defer root.Close()
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusBadRequest, "path is not a regular file")
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(p)})
	w.Header().Set("Content-Disposition", disposition)
	http.ServeContent(w, r, path.Base(p), info.ModTime(), f)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
