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
	"context"
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
	"sync"
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

// AccessGrant gives exactly one credential permission to read specific named
// shares. BearerToken is never returned by the API or persisted by Handler.
// Existing NewHandler remains available for single-token, all-share mode.
type AccessGrant struct {
	BearerToken string
	ShareIDs []string
}

// Share is safe to return to a remote peer; no machine-local path is included.
type Share struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ReadOnly bool   `json:"read_only"`
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

// Handler pins opened export roots for its lifetime. Replacing a configured
// path (or any ancestor) after registration cannot silently redirect exports.
// Call Close after shutting down the HTTP server to release the directory
// handles. A Handler must not be reused after Close.
type Handler struct {
	shares    []Share
	roots     map[string]*os.Root
	grants    []preparedGrant
	mux       *http.ServeMux
	closeOnce sync.Once
	closeErr  error
}

type preparedGrant struct {
	digest [sha256.Size]byte
	shares map[string]struct{}
}

type grantContextKey struct{}

// NewHandler validates the explicit exports and returns a fully authenticated
// HTTP handler. It does not listen on a socket or grant network access itself.
// Keep the bearer token secret and generate it with a cryptographic RNG.
// Its caller owns the returned Handler and must call Close on shutdown.
func NewHandler(exports []Export, bearerToken string) (*Handler, error) {
	ids := make([]string, 0, len(exports))
	for _, e := range exports { ids = append(ids, e.ID) }
	return newHandler(exports, []AccessGrant{{BearerToken: bearerToken, ShareIDs: ids}}, true)
}

// NewHandlerWithGrants opts into separately scoped credentials. Each grant
// MUST name at least one existing share, and every token must be unique. An
// authorized client sees and reads only its listed shares; unknown share IDs
// return 404, matching nonexistent share behavior.
//
// To revoke a grant in this v1 experiment, stop the listener, close this
// handler, then start a new handler without that grant. Already-open streams
// need listener shutdown/drain; this is not a live revocation API.
func NewHandlerWithGrants(exports []Export, grants []AccessGrant) (*Handler, error) {
	return newHandler(exports, grants, false)
}

func validBearerToken(bearerToken string) bool {
	return len(bearerToken) >= 32 && len(bearerToken) <= 4096 &&
		strings.TrimSpace(bearerToken) == bearerToken && !strings.ContainsAny(bearerToken, "\r\n\t ")
}

func newHandler(exports []Export, grants []AccessGrant, legacyEmptyShares bool) (*Handler, error) {
	if len(grants) == 0 || len(grants) > 256 {
		return nil, errors.New("agent: a bounded nonempty set of access grants is required")
	}
	s := &Handler{roots: make(map[string]*os.Root, len(exports))}
	fail := func(err error) (*Handler, error) {
		_ = s.Close()
		return nil, err
	}
	for _, e := range exports {
		if !validID(e.ID) || strings.TrimSpace(e.Name) == "" || len(e.Name) > 128 || !filepath.IsAbs(e.Path) {
			return fail(fmt.Errorf("agent: invalid export configuration for %q", e.ID))
		}
		if _, exists := s.roots[e.ID]; exists {
			return fail(fmt.Errorf("agent: duplicate export ID %q", e.ID))
		}
		root, err := os.OpenRoot(e.Path)
		if err != nil {
			return fail(fmt.Errorf("agent: cannot open export %q: %w", e.ID, err))
		}
		info, statErr := root.Stat(".")
		if statErr != nil || !info.IsDir() {
			_ = root.Close()
			return fail(fmt.Errorf("agent: export %q is not an accessible directory", e.ID))
		}
		s.roots[e.ID] = root
		s.shares = append(s.shares, Share{ID: e.ID, Name: e.Name, ReadOnly: true})
	}

	seen := make(map[[sha256.Size]byte]struct{}, len(grants))
	for _, g := range grants {
		if !validBearerToken(g.BearerToken) || (len(g.ShareIDs) == 0 && !legacyEmptyShares) {
			return fail(errors.New("agent: grant needs a strong token and at least one share"))
		}
		digest := sha256.Sum256([]byte(g.BearerToken))
		if _, duplicate := seen[digest]; duplicate {
			return fail(errors.New("agent: duplicate bearer credential"))
		}
		seen[digest] = struct{}{}
		allowed := make(map[string]struct{}, len(g.ShareIDs))
		for _, id := range g.ShareIDs {
			if _, exists := s.roots[id]; !exists {
				return fail(fmt.Errorf("agent: grant references unknown share %q", id))
			}
			if _, duplicate := allowed[id]; duplicate {
				return fail(fmt.Errorf("agent: duplicate share in grant %q", id))
			}
			allowed[id] = struct{}{}
		}
		s.grants = append(s.grants, preparedGrant{digest: digest, shares:allowed})
	}

	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /v1/shares", s.getShares)
	s.mux.HandleFunc("GET /v1/list", s.list)
	s.mux.HandleFunc("GET /v1/download", s.download)
	return s, nil
}

// ServeHTTP authenticates a token and enforces its share scope on metadata and
// bytes alike. A valid token by itself cannot address a share outside its grant.
func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		allowed, authorized := s.authorized(r)
		if !authorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="MasterDrive Agent"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		s.mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), grantContextKey{}, allowed)))
}

// Close releases pinned roots. Shutdown the HTTP server before calling Close.
// Subsequent calls are safe and return the same error, if any.
func (s *Handler) Close() error {
	s.closeOnce.Do(func() {
		for _, root := range s.roots {
			if err := root.Close(); err != nil && s.closeErr == nil {
				s.closeErr = err
			}
		}
	})
	return s.closeErr
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

func (s *Handler) authorized(r *http.Request) (map[string]struct{}, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) > 4103 {
		return nil, false
	}
	scheme, received, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || received == "" || strings.ContainsAny(received, "\r\n\t ") {
		return nil, false
	}
	digest := sha256.Sum256([]byte(received))
	var allowed map[string]struct{}
	// Do not reveal a token's grant position through early-return timing.
	for _, g := range s.grants {
		if subtle.ConstantTimeCompare(digest[:], g.digest[:]) == 1 {
			allowed = g.shares
		}
	}
	return allowed, allowed != nil
}

func permittedShares(r *http.Request) map[string]struct{} {
	allowed, _ := r.Context().Value(grantContextKey{}).(map[string]struct{})
	return allowed
}

func (s *Handler) getShares(w http.ResponseWriter, r *http.Request) {
	allowed := permittedShares(r)
	shares := make([]Share, 0, len(allowed))
	for _, share := range s.shares {
		if _, ok := allowed[share.ID]; ok {
			shares = append(shares, share)
		}
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

func (s *Handler) open(w http.ResponseWriter, r *http.Request) (*os.File, string, string, bool) {
	id := r.URL.Query().Get("share")
	if _, allowed := permittedShares(r)[id]; !allowed {
		writeError(w, http.StatusNotFound, "share not found")
		return nil, "", "", false
	}
	root, exists := s.roots[id]
	if !exists {
		writeError(w, http.StatusNotFound, "share not found")
		return nil, "", "", false
	}
	p := r.URL.Query().Get("path")
	local, err := localPath(p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return nil, "", "", false
	}
	f, err := root.Open(local)
	if err != nil {
		writeError(w, http.StatusNotFound, "path not available")
		return nil, "", "", false
	}
	return f, id, p, true
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

func (s *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pageValues(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f, id, p, ok := s.open(w, r)
	if !ok {
		return
	}
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

func (s *Handler) download(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("path") == "" || r.URL.Query().Get("path") == "." {
		writeError(w, http.StatusBadRequest, "file path required")
		return
	}
	f, _, p, ok := s.open(w, r)
	if !ok {
		return
	}
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
