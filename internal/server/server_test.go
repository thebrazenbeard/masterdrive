package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	received := false
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
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
	if w.Code != http.StatusBadGateway || received { t.Fatalf("redirect followed=%v, status=%d", received, w.Code) }
}
