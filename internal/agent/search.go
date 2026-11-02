package agent

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	searchDefaultMatches = 25
	searchMaxMatches = 50
	searchScanWindow = 256
)

// search is a deliberately shallow, bounded filename filter. It never walks
// into child directories, reads file contents, or builds a background index.
// The owner must have already exported the requested share, and the caller
// must identify the exact share and folder. This is NOT an exhaustive recursive
// search endpoint.
func (s *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 || len(q) > 128 || !utf8.ValidString(q) || strings.ContainsAny(q, "\x00\r\n") {
		writeError(w, http.StatusBadRequest, "search query must be 2-128 UTF-8 bytes")
		return
	}
	limit, offset, err := searchPageValues(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f, shareID, folder, ok := s.open(w, r)
	if !ok { return }
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "search path is not a directory")
		return
	}

	// ReadDir(n) yields at most n records. An offset is bounded too, so each
	// request examines at most 256 children and reads at most 10,257 names.
	children, err := f.ReadDir(offset + searchScanWindow + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusInternalServerError, "unable to search directory")
		return
	}
	entries := make([]Entry, 0, limit)
	scanned := 0
	if offset < len(children) {
		end := offset + searchScanWindow
		if end > len(children) { end = len(children) }
		needle := strings.ToLower(q)
		for _, child := range children[offset:end] {
			scanned++
			childPath := path.Join(folder, child.Name())
			if !fs.ValidPath(childPath) || strings.ContainsAny(childPath, "\\\x00") ||
				!strings.Contains(strings.ToLower(child.Name()), needle) {
				continue
			}
			childInfo, err := child.Info()
			if err != nil { continue }
			kind := "other"
			switch {
			case child.Type().IsRegular(): kind = "file"
			case child.IsDir(): kind = "directory"
			case child.Type()&fs.ModeSymlink != 0: kind = "symlink"
			}
			entries = append(entries, Entry{Name: child.Name(), Path: childPath, Type: kind, Size: childInfo.Size(), ModTime: childInfo.ModTime()})
			if len(entries) == limit { break }
		}
	}
	result := struct {
		Share string `json:"share"`
		Path string `json:"path"`
		Query string `json:"query"`
		Scope string `json:"scope"`
		Entries []Entry `json:"entries"`
		Scanned int `json:"scanned"`
		NextOffset *int `json:"next_offset"`
		Truncated bool `json:"truncated"`
	}{Share: shareID, Path: folder, Query: q, Scope: "immediate_children", Entries: entries, Scanned: scanned}
	if offset + scanned < len(children) {
		next := offset + scanned
		if next <= maxPageOffset {
			result.NextOffset = &next
		} else {
			result.Truncated = true // offset cap: further continuation is unavailable
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func searchPageValues(r *http.Request) (limit, offset int, err error) {
	limit = searchDefaultMatches
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > searchMaxMatches {
			return 0, 0, errors.New("invalid search limit")
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 || offset > maxPageOffset {
			return 0, 0, errors.New("invalid search offset")
		}
	}
	return limit, offset, nil
}
