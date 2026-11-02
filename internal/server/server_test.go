package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

func TestControllerListsAndProxiesExplicitHost(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer correct-test-token" {
			http.Error(w, "bad credential", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/shares" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"shares":[{"id":"photos","name":"Photos"}]}`))
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("test-node", peer.URL)
	if err != nil { t.Fatal(err) }
	api := New(store, map[string]string{host.ID: "correct-test-token"}, nil)

	list := httptest.NewRecorder()
	api.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil))
	if list.Code != http.StatusOK { t.Fatalf("hosts status=%d: %s", list.Code, list.Body.String()) }
	var inventory struct { Hosts []struct { ID, Name, Status string } `json:"hosts"` }
	if err := json.Unmarshal(list.Body.Bytes(), &inventory); err != nil { t.Fatal(err) }
	if len(inventory.Hosts) != 1 || inventory.Hosts[0].ID != host.ID || inventory.Hosts[0].Status != "not_checked" {
		t.Fatalf("unexpected host inventory: %+v", inventory.Hosts)
	}
	shares := httptest.NewRecorder()
	api.ServeHTTP(shares, httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares", nil))
	if shares.Code != http.StatusOK || !strings.Contains(shares.Body.String(), `"photos"`) {
		t.Fatalf("shares status=%d body=%s", shares.Code, shares.Body.String())
	}
}

func TestControllerRejectsUnregisteredAndMissingCredential(t *testing.T) {
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	api := New(store, nil, nil)
	missing := httptest.NewRecorder()
	api.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/hosts/missing/shares", nil))
	if missing.Code != http.StatusNotFound { t.Fatalf("unknown host status=%d", missing.Code) }
	host, err := store.Upsert("loopback", "http://127.0.0.1:8898")
	if err != nil { t.Fatal(err) }
	unauthorized := httptest.NewRecorder()
	api.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares", nil))
	if unauthorized.Code != http.StatusForbidden { t.Fatalf("missing token status=%d", unauthorized.Code) }
	writes := httptest.NewRecorder()
	api.ServeHTTP(writes, httptest.NewRequest(http.MethodPost, "/api/v1/hosts", nil))
	if writes.Code != http.StatusMethodNotAllowed { t.Fatalf("mutation status=%d", writes.Code) }
}

func TestLocalBindAndDNSRebindingGuard(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8788", "[::1]:8788", "localhost:8788"} {
		if err := ValidateLoopbackListen(address); err != nil { t.Errorf("%s rejected: %v", address, err) }
	}
	for _, address := range []string{":8788", "0.0.0.0:8788", "192.168.1.2:8788"} {
		if err := ValidateLoopbackListen(address); err == nil { t.Errorf("%s accepted", address) }
	}
	guard := RequireLoopbackHost(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	bad := httptest.NewRequest(http.MethodGet, "http://attacker.example/api/v1/hosts", nil)
	bad.Host = "attacker.example:8788"
	response := httptest.NewRecorder()
	guard.ServeHTTP(response, bad)
	if response.Code != http.StatusForbidden { t.Fatalf("host rebinding status=%d", response.Code) }
}

func TestDefaultPeerClientDoesNotFollowRedirect(t *testing.T) {
	var received atomic.Bool
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
	}))
	defer sink.Close()
	redirect := httptest.NewServer(http.RedirectHandler(sink.URL, http.StatusFound))
	defer redirect.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("redirect", redirect.URL)
	if err != nil { t.Fatal(err) }
	api := New(store, map[string]string{host.ID: "private-token"}, nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares", nil))
	if w.Code != http.StatusBadGateway || received.Load() { t.Fatalf("redirect followed=%v, status=%d", received.Load(), w.Code) }
}

func TestGatewayAbortsTruncatedPeerDownload(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/download" { http.NotFound(w, r); return }
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short"))
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("truncated", peer.URL)
	if err != nil { t.Fatal(err) }
	controller := httptest.NewServer(New(store, map[string]string{host.ID: "test-secret"}, nil))
	defer controller.Close()
	resp, err := http.Get(controller.URL + "/api/v1/hosts/" + host.ID + "/shares/docs/download?path=test.bin")
	if err != nil { t.Fatal(err) }
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr == nil {
		t.Fatalf("truncated download was reported as complete: status=%d bytes=%d", resp.StatusCode, len(body))
	}
}

func TestGatewayForwardsSingleByteRange(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=1-3" { http.Error(w, "range missing", http.StatusBadRequest); return }
		w.Header().Set("Content-Length", "3")
		w.Header().Set("Content-Range", "bytes 1-3/6")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("bcd"))
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("range", peer.URL)
	if err != nil { t.Fatal(err) }
	controller := httptest.NewServer(New(store, map[string]string{host.ID: "test-secret"}, nil))
	defer controller.Close()
	req, err := http.NewRequest(http.MethodGet, controller.URL+"/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
	if err != nil { t.Fatal(err) }
	req.Header.Set("Range", "bytes=1-3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatal(err) }
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil { t.Fatal(err) }
	if resp.StatusCode != http.StatusPartialContent || string(body) != "bcd" || resp.Header.Get("Content-Range") != "bytes 1-3/6" {
		t.Fatalf("range status=%d bytes=%q content-range=%q", resp.StatusCode, body, resp.Header.Get("Content-Range"))
	}
}
