// Package webdavpilot is a source-only read-only WebDAV protocol experiment.
// It is not wired into the MasterDrive controller and does not open a listener.
// A production mount needs OS-user-bound authorization and a version-pinned
// backend, neither of which exists in the v0.1 controller contract.
package webdavpilot

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("webdavpilot: item not found")
	ErrOffline = errors.New("webdavpilot: source offline")
	ErrUnauthorized = errors.New("webdavpilot: access denied")
	ErrStale = errors.New("webdavpilot: representation changed")
)

const maxChildren = 200
const maxPropfindBody = 8 << 10

// Key identifies one approved share and a portable relative path. Mounts must
// be restricted to explicit host/share selections outside this package.
type Key struct {
	HostID string
	ShareID string
	Relative string
}

// Metadata is supplied by a future, version-aware backend. File ETags MUST
// be strong and bound to immutable bytes, not guessed from size and mtime.
type Metadata struct {
	Name string
	Directory bool
	Size int64
	ModifiedAt time.Time
	StrongETag string
}

type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

// Backend's OpenSnapshot must either return a reader whose bytes are fixed at
// the requested strong ETag for the lifetime of the handle, or return ErrStale.
// It must not return a mutable live file stream for a successful open.
// Children MUST use a bounded snapshot cursor: return at most limit entries,
// with complete=false if any entry is omitted. A backend may inspect one
// additional cursor entry to establish incompleteness, but MUST NOT enumerate
// and allocate the entire directory before applying limit. A partial listing
// cannot be exposed as a successful 207 response.
type Backend interface {
	Stat(context.Context, Key) (Metadata, error)
	Children(context.Context, Key, int) ([]Metadata, bool, error)
	OpenSnapshot(context.Context, Key, string) (ReadSeekCloser, error)
}

// Authorize represents a REQUIRED mount-scoped local identity check. The
// production implementation needs OS-user binding and revocation, not merely
// a bearer value provided by another local process.
type Authorize func(*http.Request, Key) bool

type Handler struct {
	backend Backend
	authorize Authorize
}

func New(backend Backend, authorize Authorize) (*Handler, error) {
	if backend == nil || authorize == nil {
		return nil, errors.New("webdavpilot: backend and identity authorization are required")
	}
	return &Handler{backend: backend, authorize: authorize}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !loopbackHost(r.Host) {
		http.Error(w, "local-only endpoint", http.StatusForbidden)
		return
	}
	// Decode-once path routing below deliberately rejects encoded separators,
	// backslashes and percent signs, including double-encoded path components.
	escaped := strings.ToLower(r.URL.EscapedPath())
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") || strings.Contains(escaped, "%25") {
		http.NotFound(w, r)
		return
	}
	key, err := parseKey(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !h.authorize(r, key) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="MasterDrive WebDAV pilot"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodOptions:
		w.Header().Set("Allow", "OPTIONS, PROPFIND, GET, HEAD")
		w.Header().Set("DAV", "1")
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		h.propfind(w, r, key)
	case http.MethodGet, http.MethodHead:
		h.read(w, r, key)
	default:
		// Never forward a mutating request to a remote source.
		w.Header().Set("Allow", "OPTIONS, PROPFIND, GET, HEAD")
		http.Error(w, "read-only", http.StatusMethodNotAllowed)
	}
}

func parseKey(raw string) (Key, error) {
	const prefix = "/dav/"
	if len(raw) > 4096 || !strings.HasPrefix(raw, prefix) || strings.Contains(raw, "\\") || strings.Contains(raw, "\x00") || strings.Contains(raw, "%") {
		return Key{}, ErrNotFound
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(raw, prefix), "/"), "/")
	if len(parts) < 2 || len(parts[0]) != 32 || !validID(parts[1]) {
		return Key{}, ErrNotFound
	}
	for _, ch := range parts[0] {
		if !strings.ContainsRune("0123456789abcdef", ch) { return Key{}, ErrNotFound }
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\\x00") {
			return Key{}, ErrNotFound
		}
	}
	// Only path components under a share represent native filenames. Keep
	// the pilot's public namespace deliberately smaller than either Windows
	// or Linux until M0 specifies a reversible cross-platform name mapping.
	for _, part := range parts[2:] {
		if !validName(part) { return Key{}, ErrNotFound }
	}
	rel := strings.Join(parts[2:], "/")
	return Key{HostID: parts[0], ShareID: parts[1], Relative: rel}, nil
}

func validID(id string) bool {
	if len(id) < 1 || len(id) > 64 || !validName(id) { return false }
	for _, ch := range id {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') { return false }
	}
	return true
}

func validName(name string) bool {
	if name == "" || name == "." || name == ".." || len(name) > 255 ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") ||
		strings.ContainsAny(name, `/\\%:"<>|?*`) { return false }
	// Explicitly admit only ASCII until Unicode normalization, case folding,
	// and collision-proof translation have an owner-approved contract.
	for _, ch := range name { if ch < 0x20 || ch >= 0x7f { return false } }
	stem := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " ."))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		return false
	}
	return true
}

func expectedName(key Key) string {
	if key.Relative == "" { return key.ShareID }
	return path.Base(key.Relative)
}

func loopbackHost(hostport string) bool {
	host := hostport
	if value, _, err := net.SplitHostPort(hostport); err == nil { host = value }
	if strings.EqualFold(host, "localhost") { return true }
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func status(err error) int {
	switch {
	case errors.Is(err, ErrNotFound): return http.StatusNotFound
	case errors.Is(err, ErrUnauthorized): return http.StatusForbidden
	case errors.Is(err, ErrStale): return http.StatusPreconditionFailed
	case errors.Is(err, ErrOffline): return http.StatusServiceUnavailable
	default: return http.StatusBadGateway
	}
}

func fail(w http.ResponseWriter, err error) {
	http.Error(w, http.StatusText(status(err)), status(err))
}

func validStrongETag(tag string) bool {
	if len(tag) < 2 || len(tag) > 256 || tag[0] != '"' || tag[len(tag)-1] != '"' { return false }
	for i := 1; i < len(tag)-1; i++ {
		if tag[i] < '!' || tag[i] == '"' || tag[i] == 0x7f { return false }
	}
	return true
}

// The single-range guard must not reject a Range that RFC 9110 requires us
// to ignore after a failed If-Range validator. Keep this aligned with
// net/http.ServeContent's strong-ETag and HTTP-date validator handling.
func ifRangeMatches(value, etag string, modified time.Time) bool {
	value = strings.TrimSpace(value) // RFC field-value optional whitespace.
	if value == etag {
		return true
	}
	if strings.HasPrefix(value, "W/") {
		return false
	}
	date, err := http.ParseTime(value)
	return err == nil && !modified.IsZero() && modified.Truncate(time.Second).Equal(date)
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request, key Key) {
	meta, err := h.backend.Stat(r.Context(), key)
	if err != nil { fail(w, err); return }
	if meta.Directory || meta.Size < 0 || !validName(meta.Name) || meta.Name != expectedName(key) ||
		!validStrongETag(meta.StrongETag) || meta.ModifiedAt.IsZero() {
		// This prototype cannot claim a safe mount read without a source-issued
		// immutable version; no fallback to a live unversioned stream.
		http.Error(w, "version-pinned read unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodHead && r.Header.Get("Range") != "" {
		// RFC 9110 restricts Range to GET. A HEAD request must receive
		// headers for the full selected representation, not a range error.
		r = r.Clone(r.Context())
		r.Header.Del("Range")
	}
	if requested := r.Header.Get("Range"); requested != "" {
		validator := r.Header.Get("If-Range")
		if validator != "" && !ifRangeMatches(validator, meta.StrongETag, meta.ModifiedAt) {
			// RFC 9110 requires ignoring Range after a failed If-Range.
			// Remove it explicitly: ServeContent's ETag parser accepts some
			// malformed trailing suffixes which our strict check rejects.
			r = r.Clone(r.Context())
			r.Header.Del("Range")
		} else if len(requested) > 256 || !strings.HasPrefix(requested, "bytes=") || strings.Contains(requested, ",") {
			http.Error(w, "only a single byte range is supported", http.StatusBadRequest)
			return
		}
	}
	// ServeContent implements RFC 9110 If-Range: a failed validator causes
	// Range to be ignored and the full selected representation to be served.
	// In particular, If-Range mismatch is not an If-Match precondition failure.
	reader, err := h.backend.OpenSnapshot(r.Context(), key, meta.StrongETag)
	if err != nil { fail(w, err); return }
	defer reader.Close()
	if count, err := reader.Seek(0, io.SeekEnd); err != nil || count != meta.Size {
		http.Error(w, "inconsistent snapshot length", http.StatusBadGateway)
		return
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "snapshot cannot seek", http.StatusBadGateway)
		return
	}
	w.Header().Set("ETag", meta.StrongETag)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, meta.Name, meta.ModifiedAt, reader)
}

type davMultiStatus struct {
	XMLName xml.Name `xml:"D:multistatus"`
	XMLNS string `xml:"xmlns:D,attr"`
	Responses []davResponse `xml:"D:response"`
}
type davResponse struct {
	Href string `xml:"D:href"`
	PropStat davPropStat `xml:"D:propstat"`
}
type davPropStat struct {
	Prop davProperties `xml:"D:prop"`
	Status string `xml:"D:status"`
}
type davProperties struct {
	DisplayName string `xml:"D:displayname"`
	ResourceType davResourceType `xml:"D:resourcetype"`
	ContentLength *string `xml:"D:getcontentlength,omitempty"`
	LastModified string `xml:"D:getlastmodified,omitempty"`
	ETag string `xml:"D:getetag,omitempty"`
}
type davResourceType struct {
	Collection *struct{} `xml:"D:collection,omitempty"`
}

func href(key Key, directory bool) string {
	parts := []string{"dav", key.HostID, key.ShareID}
	if key.Relative != "" { parts = append(parts, strings.Split(key.Relative, "/")...) }
	for i, item := range parts { parts[i] = url.PathEscape(item) }
	value := "/" + strings.Join(parts, "/")
	if directory { value += "/" }
	return value
}

func davEntry(key Key, meta Metadata) davResponse {
	props := davProperties{DisplayName: meta.Name}
	if meta.Directory {
		props.ResourceType.Collection = &struct{}{}
	} else {
		length := strconv.FormatInt(meta.Size, 10)
		props.ContentLength = &length
		props.ETag = meta.StrongETag
	}
	if !meta.ModifiedAt.IsZero() { props.LastModified = meta.ModifiedAt.UTC().Format(http.TimeFormat) }
	return davResponse{Href: href(key, meta.Directory), PropStat: davPropStat{Prop: props, Status: "HTTP/1.1 200 OK"}}
}

func (h *Handler) propfind(w http.ResponseWriter, r *http.Request, key Key) {
	depth := r.Header.Get("Depth")
	if depth != "0" && depth != "1" {
		http.Error(w, "finite explicit Depth: 0 or 1 required", http.StatusForbidden)
		return
	}
	if r.ContentLength > maxPropfindBody {
		http.Error(w, "PROPFIND request too large", http.StatusRequestEntityTooLarge)
		return
	}
	body := r.Body
	if body == nil { body = http.NoBody }
	requestXML, err := io.ReadAll(io.LimitReader(body, maxPropfindBody+1))
	if err != nil || len(requestXML) > maxPropfindBody {
		http.Error(w, "invalid PROPFIND request", http.StatusBadRequest)
		return
	}
	if len(requestXML) != 0 {
		var root struct { XMLName xml.Name }
		if xml.Unmarshal(requestXML, &root) != nil || root.XMLName.Space != "DAV:" || root.XMLName.Local != "propfind" {
			http.Error(w, "unsupported PROPFIND XML", http.StatusBadRequest)
			return
		}
	}
	meta, err := h.backend.Stat(r.Context(), key)
	if err != nil { fail(w, err); return }
	if !validName(meta.Name) || meta.Name != expectedName(key) || (!meta.Directory && meta.ModifiedAt.IsZero()) {
		http.Error(w, "invalid root metadata", http.StatusBadGateway)
		return
	}
	if !meta.Directory && (!validStrongETag(meta.StrongETag) || meta.Size < 0) {
		http.Error(w, "versioned file metadata unavailable", http.StatusServiceUnavailable)
		return
	}
	responses := []davResponse{davEntry(key, meta)}
	if depth == "1" && meta.Directory {
		children, complete, err := h.backend.Children(r.Context(), key, maxChildren)
		if err != nil { fail(w, err); return }
		if !complete || len(children) > maxChildren {
			http.Error(w, "snapshot listing limit exceeded", http.StatusServiceUnavailable)
			return
		}
		seen := make(map[string]struct{}, len(children))
		for _, child := range children {
			if !validName(child.Name) || child.Size < 0 ||
				(!child.Directory && (!validStrongETag(child.StrongETag) || child.ModifiedAt.IsZero())) {
				http.Error(w, "invalid child metadata", http.StatusBadGateway)
				return
			}
			// A literal collision or case-only alias could render the same file
			// ambiguous in Windows. Fail the whole listing rather than silently
			// shadowing an item. Full Unicode folding remains an M0 requirement.
			folded := strings.ToLower(child.Name)
			if _, duplicate := seen[folded]; duplicate {
				http.Error(w, "ambiguous child identity", http.StatusConflict)
				return
			}
			seen[folded] = struct{}{}
			childKey := key
			childKey.Relative = path.Join(key.Relative, child.Name)
			responses = append(responses, davEntry(childKey, child))
		}
	}
	data, err := xml.Marshal(davMultiStatus{XMLNS: "DAV:", Responses: responses})
	if err != nil { http.Error(w, "cannot encode metadata", http.StatusBadGateway); return }
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(207)
	_, _ = w.Write(append([]byte(xml.Header), data...))
}
