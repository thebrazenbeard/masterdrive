package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/agent"
	"github.com/thebrazenbeard/masterdrive/internal/catalog"
	"github.com/thebrazenbeard/masterdrive/internal/server"
)

const securityFixtureToken = "masterdrive-security-test-token-with-at-least-32-bytes"

func securityExport(t *testing.T) (*agent.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "permitted.txt"), []byte("permitted content"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := agent.NewHandler([]agent.Export{{ID: "approved", Name: "Approved", Path: dir}}, securityFixtureToken)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { if err := h.Close(); err != nil { t.Error(err) } })
	return h, dir
}

func securityRequest(h http.Handler, method, target, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	if token != "" { r.Header.Set("Authorization", "Bearer "+token) }
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func securityErrorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct { Error struct { Code string `json:"code"` } `json:"error"` }
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil { t.Fatalf("invalid API error JSON: %v", err) }
	return payload.Error.Code
}

func TestSecurityAgentDeniesEveryReadRouteWithoutToken(t *testing.T) {
	h, exportDir := securityExport(t)
	routes := []string{
		"/v1/shares",
		"/v1/list?share=approved",
		"/v1/download?share=approved&path=permitted.txt",
	}
	for _, route := range routes {
		for _, token := range []string{"", "incorrect-token"} {
			w := securityRequest(h, http.MethodGet, route, token)
			if w.Code != http.StatusUnauthorized { t.Errorf("%s token=%q returned %d", route, token, w.Code) }
			if strings.Contains(w.Body.String(), "permitted content") || strings.Contains(w.Body.String(), exportDir) {
				t.Errorf("%s disclosed private content or source path", route)
			}
		}
	}
}

func TestSecurityAgentConfinementThroughWirePaths(t *testing.T) {
	h, exportDir := securityExport(t)
	// A sibling to the share root must not be addressable by a relative path.
	if err := os.WriteFile(filepath.Join(filepath.Dir(exportDir), "outside.txt"), []byte("outside-secret"), 0600); err != nil { t.Fatal(err) }
	bad := []string{"../outside.txt", "../../secret.txt", "/etc/passwd", "C:\\Windows\\win.ini", "folder/../outside.txt"}
	for _, attempted := range bad {
		for _, endpoint := range []string{"/v1/list", "/v1/download"} {
			target := endpoint + "?share=approved&path=" + url.QueryEscape(attempted)
			w := securityRequest(h, http.MethodGet, target, securityFixtureToken)
			if w.Code >= 200 && w.Code < 300 { t.Errorf("escaped via %s path %q (HTTP %d)", endpoint, attempted, w.Code) }
			if strings.Contains(w.Body.String(), "outside-secret") { t.Errorf("outside bytes leaked from %s", target) }
		}
	}
	if w := securityRequest(h, http.MethodGet, "/v1/download?share=approved&path=permitted.txt", securityFixtureToken); w.Code != http.StatusOK {
		t.Fatalf("allowed file was rejected: HTTP %d: %s", w.Code, w.Body.String())
	}
}

func TestSecurityBrowserSessionIsSeparateFromPeerCredential(t *testing.T) {
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	secret, err := server.NewSessionToken()
	if err != nil { t.Fatal(err) }
	protected, err := server.RequireSession(secret, server.New(store, nil, nil))
	if err != nil { t.Fatal(err) }
	unauthorized := securityRequest(protected, http.MethodGet, "/api/v1/hosts", "")
	if unauthorized.Code != http.StatusUnauthorized || securityErrorCode(t, unauthorized) != "session_required" {
		t.Fatalf("unauthenticated browser API returned HTTP %d", unauthorized.Code)
	}
	wrong := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
	wrong.Header.Set("X-MasterDrive-Session", strings.Repeat("x", len(secret)))
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, wrong)
	if w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 { t.Fatal("wrong local session was accepted") }
	activate := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
	activate.Header.Set("X-MasterDrive-Session", secret)
	w = httptest.NewRecorder()
	protected.ServeHTTP(w, activate)
	if w.Code != http.StatusNoContent { t.Fatalf("valid local session rejected: %d", w.Code) }
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode { t.Fatalf("session cookie lacks protections: %+v", cookies) }
	request := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	request.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	protected.ServeHTTP(w, request)
	if w.Code != http.StatusOK { t.Fatalf("activated browser session failed: %d", w.Code) }
	if strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), securityFixtureToken) { t.Fatal("secrets leaked to browser inventory") }
}

func TestSecurityMissingPeerCredentialFailsClosed(t *testing.T) {
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("uncredentialed", "http://127.0.0.1:18739")
	if err != nil { t.Fatal(err) }
	w := securityRequest(server.New(store, nil, nil), http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares", "")
	if w.Code != http.StatusForbidden || securityErrorCode(t, w) != "peer_token_missing" {
		t.Fatalf("missing peer token status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSecurityGatewayRejectsUnknownLengthAndEncodedPayload(t *testing.T) {
	for name, peerHandler := range map[string]http.HandlerFunc{
		"unknown-length": func(w http.ResponseWriter, r *http.Request) {
			w.(http.Flusher).Flush() // force chunked body without known length
			_, _ = w.Write([]byte("unverifiable"))
		},
		"compressed-payload": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", "4")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("test"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			peer := httptest.NewServer(peerHandler)
			defer peer.Close()
			store, err := catalog.Open(t.TempDir())
			if err != nil { t.Fatal(err) }
			host, err := store.Upsert(name, peer.URL)
			if err != nil { t.Fatal(err) }
			w := securityRequest(server.New(store, map[string]string{host.ID: securityFixtureToken}, nil), http.MethodGet,
				"/api/v1/hosts/"+host.ID+"/shares/approved/download?path=permitted.txt", "")
			if w.Code != http.StatusBadGateway { t.Fatalf("unverifiable peer representation accepted: HTTP %d", w.Code) }
		})
	}
}

func TestSecurityGatewayRejectsUntrustedRedirectAndMalformedRange(t *testing.T) {
	var hits atomic.Int64
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer sink.Close()
	redirect := httptest.NewServer(http.RedirectHandler(sink.URL, http.StatusFound))
	defer redirect.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("redirect", redirect.URL)
	if err != nil { t.Fatal(err) }
	w := securityRequest(server.New(store, map[string]string{host.ID: securityFixtureToken}, nil), http.MethodGet,
		"/api/v1/hosts/"+host.ID+"/shares", "")
	if w.Code != http.StatusBadGateway || hits.Load() != 0 { t.Fatal("redirect followed or leaked downstream authority") }

	for _, contentRange := range []string{
		"bytes 40-42/100", // wrong start and end
		"bytes 1-3/*",     // unverifiable total
		"bytes 1-3/2",     // impossible total
		"bytes 1-2/100",   // body length does not match claimed range
		"nonsense",        // malformed representation metadata
		"",                // missing metadata altogether
	} {
		t.Run("range="+contentRange, func(t *testing.T) {
			badPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if contentRange != "" { w.Header().Set("Content-Range", contentRange) }
				w.Header().Set("Content-Length", "3")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("bad"))
			}))
			defer badPeer.Close()
			badHost, err := store.Upsert("invalid-range", badPeer.URL)
			if err != nil { t.Fatal(err) }
			request := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+badHost.ID+"/shares/approved/download?path=permitted.txt", nil)
			request.Header.Set("Range", "bytes=1-3")
			w = httptest.NewRecorder()
			server.New(store, map[string]string{badHost.ID: securityFixtureToken}, nil).ServeHTTP(w, request)
			if w.Code != http.StatusBadGateway { t.Fatalf("mismatched range %q accepted: HTTP %d", contentRange, w.Code) }
		})
	}
}

func TestSecurityNoUnauthorizedFileMutation(t *testing.T) {
	h, dir := securityExport(t)
	w := securityRequest(h, http.MethodDelete, "/v1/download?share=approved&path=permitted.txt", securityFixtureToken)
	if w.Code < 400 { t.Fatalf("mutating method unexpectedly accepted: HTTP %d", w.Code) }
	contents, err := os.ReadFile(filepath.Join(dir, "permitted.txt"))
	if err != nil { t.Fatal(err) }
	if string(contents) != "permitted content" { t.Fatal("source file changed after denied mutation") }
}

