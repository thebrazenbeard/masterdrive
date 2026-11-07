package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// SnapshotLimit bounds per-request memory use. This is not a persistent snapshot lease.
const SnapshotLimit int64 = 8 << 20

// snapshotDownload captures a bounded representation before serving any response bytes.
// The digest identifies captured bytes, not an atomic filesystem version.
func (s *Handler) snapshotDownload(w http.ResponseWriter, r *http.Request) {
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
	if info.Size() > SnapshotLimit {
		writeError(w, http.StatusRequestEntityTooLarge, "snapshot limit exceeded")
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, SnapshotLimit+1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "snapshot read failed")
		return
	}
	if int64(len(data)) > SnapshotLimit {
		writeError(w, http.StatusRequestEntityTooLarge, "snapshot limit exceeded")
		return
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || after.Size() != int64(len(data)) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		writeError(w, http.StatusConflict, "source changed during snapshot")
		return
	}
	digest := sha256.Sum256(data)
	etag := "\"" + hex.EncodeToString(digest[:]) + "\""
	// Every partial request must identify its expected representation.
	if r.Header.Get("Range") != "" {
		if strings.TrimSpace(r.Header.Get("If-Match")) != etag {
			writeError(w, http.StatusPreconditionFailed, "snapshot version precondition failed")
			return
		}
	}
	if match := strings.TrimSpace(r.Header.Get("If-Match")); match != "" && match != etag {
		writeError(w, http.StatusPreconditionFailed, "snapshot version precondition failed")
		return
	}
	if r.Header.Get("Range") != "" && r.Header.Get("If-Range") != "" && r.Header.Get("If-Range") != etag {
		writeError(w, http.StatusPreconditionFailed, "snapshot If-Range mismatch")
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Snapshot-Bytes", strconv.Itoa(len(data)))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(p)}))
	http.ServeContent(w, r, path.Base(p), info.ModTime(), bytes.NewReader(data))
}
