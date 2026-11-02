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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "read-only API")
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
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			fail(w, http.StatusBadGateway, "peer_unavailable", "peer could not be reached")
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			fail(w, http.StatusBadGateway, "peer_error", fmt.Sprintf("peer returned HTTP %d", response.StatusCode))
			return
		}
		if download {
			filename := path.Base(r.URL.Query().Get("path"))
			if filename == "." || filename == "/" || filename == "" {
				fail(w, http.StatusBadRequest, "bad_path", "a file path is required")
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, response.Body)
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
