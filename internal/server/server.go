// Package server exposes the local read-only gateway API. It never exposes
// peer credentials to the browser. Only explicit, previously registered peers
// may be contacted; peer registration itself is not an HTTP operation.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

const maxPeerJSON = 2 << 20

// New returns a read-only controller API. tokens is keyed by catalog host ID.
// Each token is held only in process memory, never sent to a browser or disk.
func New(store *catalog.Store, tokens map[string]string, client *http.Client) http.Handler {
	if client == nil {
		client = &http.Client{Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			MaxIdleConnsPerHost: 2,
		}, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	// Always disable redirects, including for injected test/embedder clients.
	// A 30x from a peer must never forward its share credential to a new URL.
	// Copy the client to avoid mutating a caller-owned shared instance.
	protectedClient := *client
	protectedClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	client = &protectedClient
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "read-only API")
			return
		}
		// CLI host add/remove runs as a separate process. Never serve a stale
		// in-memory catalog after an external update or damaged state file.
		if err := store.Reload(); err != nil {
			fail(w, http.StatusServiceUnavailable, "catalog_unavailable", "host registry could not be refreshed")
			return
		}
		if r.URL.Path == "/api/v1/hosts" {
			hosts := store.List()
			views := make([]hostView, 0, len(hosts))
			for _, host := range hosts {
				views = append(views, hostView{ID: host.ID, Name: host.Name, Status: "not_checked"})
			}
			respond(w, http.StatusOK, map[string]any{"hosts": views})
			return
		}
		if r.URL.Path == "/api/v1/health" {
			respond(w, http.StatusOK, map[string]string{"status": "ok", "mode": "controller"})
			return
		}
		if r.URL.Path == "/api/v1/search" {
			serveFederatedSearch(w, r, store, tokens, client)
			return
		}
		const prefix = "/api/v1/hosts/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			fail(w, http.StatusNotFound, "not_found", "unknown endpoint")
			return
		}
		segments := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
		if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
			fail(w, http.StatusNotFound, "not_found", "unknown endpoint")
			return
		}
		host, found := store.Get(segments[0])
		if !found {
			fail(w, http.StatusNotFound, "unknown_host", "host is not registered")
			return
		}
		token := tokens[host.ID]
		if token == "" {
			fail(w, http.StatusForbidden, "peer_token_missing", "host credential is not configured")
			return
		}
		var peerPath string
		var download bool
		query := url.Values{}
		switch {
		case len(segments) == 2 && segments[1] == "shares":
			peerPath = "/v1/shares"
		case len(segments) == 4 && segments[1] == "shares" && segments[2] != "" && segments[3] == "entries":
			peerPath = "/v1/list"
			query.Set("share", segments[2])
			query.Set("path", r.URL.Query().Get("path"))
			for _, key := range []string{"limit", "offset"} {
				if value := r.URL.Query().Get(key); value != "" {
					query.Set(key, value)
				}
			}
		case len(segments) == 4 && segments[1] == "shares" && segments[2] != "" && segments[3] == "download":
			peerPath = "/v1/download"
			query.Set("share", segments[2])
			query.Set("path", r.URL.Query().Get("path"))
			download = true
		default:
			fail(w, http.StatusNotFound, "not_found", "unknown endpoint")
			return
		}
		endpoint, err := url.Parse(host.BaseURL)
		if err != nil || endpoint.User != nil {
			fail(w, http.StatusBadGateway, "bad_peer", "host endpoint is invalid")
			return
		}
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + peerPath
		endpoint.RawQuery = query.Encode()
		ctx := r.Context()
		if !download {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			fail(w, http.StatusBadGateway, "bad_peer", "cannot create peer request")
			return
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Accept-Encoding", "identity")
		// A caller may assemble independent range responses only if the peer
		// offers a strong representation validator. A size/modtime guess is not
		// a strong ETag, and this controller must not invent one.
		var ifRange string
		if download {
			request.Header.Set("Accept", "application/octet-stream")
			if requestedRange := r.Header.Get("Range"); requestedRange != "" {
				if len(requestedRange) > 256 || strings.Contains(requestedRange, ",") || !strings.HasPrefix(requestedRange, "bytes=") {
					fail(w, http.StatusBadRequest, "invalid_range", "only a single byte range is supported")
					return
				}
				if values := r.Header.Values("If-Range"); len(values) != 0 {
					// HTTP dates and weak entity tags are not safe range-version
					// evidence for mutable files exposed by MasterDrive.
					if len(values) != 1 || !validStrongETag(values[0]) {
						fail(w, http.StatusBadRequest, "invalid_if_range", "a single strong ETag is required for conditional ranges")
						return
					}
					ifRange = values[0]
					request.Header.Set("If-Range", ifRange)
				}
				request.Header.Set("Range", requestedRange)
			}
		} else {
			request.Header.Set("Accept", "application/json")
		}
		response, err := client.Do(request)
		if err != nil {
			fail(w, http.StatusBadGateway, "peer_unavailable", "peer could not be reached")
			return
		}
		defer response.Body.Close()
		if download {
			requestedRange := request.Header.Get("Range")
			if encoding := response.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
				fail(w, http.StatusBadGateway, "unexpected_encoding", "peer returned a transformed file representation")
				return
			}
			// If-Range normally falls back to HTTP 200 when a representation
			// changed. The gateway intentionally fails closed: delivering a full
			// body to a range-assembling client can silently corrupt its output.
			if ifRange != "" && response.StatusCode == http.StatusOK {
				fail(w, http.StatusPreconditionFailed, "representation_changed", "the previous file version cannot be resumed")
				return
			}
			if (requestedRange == "" && response.StatusCode != http.StatusOK) ||
				(requestedRange != "" && response.StatusCode != http.StatusPartialContent) {
				fail(w, http.StatusBadGateway, "peer_error", fmt.Sprintf("peer returned unexpected HTTP %d", response.StatusCode))
				return
			}
			peerETags := response.Header.Values("ETag")
			peerETag := response.Header.Get("ETag")
			if ifRange != "" && (len(peerETags) != 1 || peerETag != ifRange) {
				fail(w, http.StatusBadGateway, "unverified_range", "peer did not confirm the requested file version")
				return
			}
			// Without an explicit byte count, an early clean EOF cannot be
			// distinguished from a complete transfer. Fail before sending data.
			if response.ContentLength < 0 {
				fail(w, http.StatusBadGateway, "missing_length", "peer did not provide a verifiable transfer length")
				return
			}
			filename := path.Base(r.URL.Query().Get("path"))
			if filename == "." || filename == "/" || filename == "" {
				fail(w, http.StatusBadRequest, "bad_path", "a file path is required")
				return
			}
			var contentRange string
			if response.StatusCode == http.StatusPartialContent {
				contentRange = response.Header.Get("Content-Range")
				if !validSingleRangeResponse(requestedRange, contentRange, response.ContentLength) {
					fail(w, http.StatusBadGateway, "invalid_range", "peer did not provide a valid range response")
					return
				}
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
			w.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
			// Forward only syntactically strong peer validators. Until an agent
			// provides them, downloads advertise no resumption guarantee.
			if len(peerETags) == 1 && validStrongETag(peerETag) {
				w.Header().Set("ETag", peerETag)
			}
			if contentRange != "" {
				w.Header().Set("Content-Range", contentRange)
			}
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(response.StatusCode)
			if copied, err := io.CopyN(w, response.Body, response.ContentLength); err != nil || copied != response.ContentLength {
				// Headers have been sent; a 502 is no longer possible. Abort the
				// connection so a client observes a failed/incomplete transfer.
				panic(http.ErrAbortHandler)
			}
			return
		}
		if response.StatusCode != http.StatusOK {
			fail(w, http.StatusBadGateway, "peer_error", fmt.Sprintf("peer returned HTTP %d", response.StatusCode))
			return
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, maxPeerJSON+1))
		if err != nil || len(body) > maxPeerJSON || !json.Valid(body) {
			fail(w, http.StatusBadGateway, "invalid_peer_response", "peer sent invalid or excessive JSON")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}

// validStrongETag conservatively recognizes the RFC 9110 strong entity-tag
// syntax. Weak tags, dates, comma-separated validators and controls fail.
func validStrongETag(value string) bool {
	if len(value) < 2 || len(value) > 256 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	for i := 1; i < len(value)-1; i++ {
		if value[i] < '!' || value[i] == '"' || value[i] == 0x7f {
			return false
		}
	}
	return true
}

func decimalDigits(value string) bool {
    if value == "" { return false }
    for i := 0; i < len(value); i++ { if value[i] < '0' || value[i] > '9' { return false } }
    return true
}

// validSingleRangeResponse checks that a peer's 206 covers exactly the bytes
// requested, with a coherent total and body length. It does not establish
// cross-request file immutability; resumes need separate entity validators.
func validSingleRangeResponse(requested, returned string, length int64) bool {
	if length < 0 || !strings.HasPrefix(requested, "bytes=") || !strings.HasPrefix(returned, "bytes ") {
		return false
	}
	rangePart, totalPart, ok := strings.Cut(strings.TrimPrefix(returned, "bytes "), "/")
	if !ok || totalPart == "*" { return false }
	startPart, endPart, ok := strings.Cut(rangePart, "-")
	if !ok { return false }
	start, errStart := strconv.ParseInt(startPart, 10, 64)
	end, errEnd := strconv.ParseInt(endPart, 10, 64)
	total, errTotal := strconv.ParseInt(totalPart, 10, 64)
	if !decimalDigits(startPart) || !decimalDigits(endPart) || !decimalDigits(totalPart) || errStart != nil || errEnd != nil || errTotal != nil || total <= 0 || start < 0 || end < start || end >= total || length != end-start+1 {
		return false
	}
	reqStartPart, reqEndPart, ok := strings.Cut(strings.TrimPrefix(requested, "bytes="), "-")
	if !ok { return false }
	if reqStartPart == "" {
		suffix, err := strconv.ParseInt(reqEndPart, 10, 64)
		if err != nil || suffix <= 0 { return false }
		if suffix > total { suffix = total }
		return start == total-suffix && end == total-1
	}
	reqStart, err := strconv.ParseInt(reqStartPart, 10, 64)
	if err != nil || reqStart < 0 || reqStart >= total { return false }
	expectedEnd := total-1
	if reqEndPart != "" {
		reqEnd, err := strconv.ParseInt(reqEndPart, 10, 64)
		if err != nil || reqEnd < reqStart { return false }
		if reqEnd < expectedEnd { expectedEnd = reqEnd }
	}
	return start == reqStart && end == expectedEnd
}

type hostView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// ValidateLoopbackListen ensures the browser controller and agent stay local.
// Remote access must be deliberately provided by a separately secured tunnel.
func ValidateLoopbackListen(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("listener must bind to loopback; use a secure overlay/tunnel for remote access")
	}
	return nil
}

// RequireLoopbackHost blocks DNS-rebinding access through arbitrary Host names.
func RequireLoopbackHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		if strings.EqualFold(host, "localhost") {
			next.ServeHTTP(w, r)
			return
		}
		ip := net.ParseIP(host)
		if ip != nil && ip.IsLoopback() {
			next.ServeHTTP(w, r)
			return
		}
		fail(w, http.StatusForbidden, "invalid_host", "local controller requires loopback Host")
	})
}
