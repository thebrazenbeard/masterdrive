package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

func TestSessionBootstrapSingleUseAndExistingCookie(t *testing.T) {
	secret, err := NewSessionToken()
	if err != nil { t.Fatal(err) }
	gate, err := RequireSession(secret, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil { t.Fatal(err) }
	redeem := func(s string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
		r.Header.Set("X-MasterDrive-Session", s)
		w := httptest.NewRecorder()
		gate.ServeHTTP(w, r)
		return w
	}
	if got := redeem(strings.Repeat("x", len(secret))); got.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token consumed bootstrap: %d", got.Code)
	}
	first := redeem(secret)
	if first.Code != http.StatusNoContent { t.Fatalf("first redeem status=%d", first.Code) }
	cookies := first.Result().Cookies()
	if len(cookies) != 1 { t.Fatalf("expected one session cookie, got %d", len(cookies)) }
	if got := redeem(secret); got.Code != http.StatusUnauthorized || len(got.Result().Cookies()) != 0 {
		t.Fatalf("reused bootstrap secret succeeded: status=%d cookies=%d", got.Code, len(got.Result().Cookies()))
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	req.AddCookie(cookies[0])
	authorized := httptest.NewRecorder()
	gate.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusNoContent { t.Fatalf("existing cookie invalidated: %d", authorized.Code) }
}

func TestConcurrentSessionBootstrapHasExactlyOneWinner(t *testing.T) {
	secret, err := NewSessionToken()
	if err != nil { t.Fatal(err) }
	gate, err := RequireSession(secret, http.NotFoundHandler())
	if err != nil { t.Fatal(err) }
	const attempts = 32
	var success atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
			r.Header.Set("X-MasterDrive-Session", secret)
			w := httptest.NewRecorder()
			gate.ServeHTTP(w, r)
			if w.Code == http.StatusNoContent { success.Add(1) } else if w.Code != http.StatusUnauthorized {
				t.Errorf("unexpected status=%d", w.Code)
			}
		}()
	}
	wg.Wait()
	if got := success.Load(); got != 1 { t.Fatalf("successful simultaneous redemptions=%d; want 1", got) }
}

func TestGatewayDoesNotFollowRedirectWithInjectedClient(t *testing.T) {
	var sinkRequests atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sinkRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()
	redirect := httptest.NewServer(http.RedirectHandler(sink.URL, http.StatusFound))
	defer redirect.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("redirect", redirect.URL)
	if err != nil { t.Fatal(err) }
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	api := New(store, map[string]string{host.ID: "private-token"}, client)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares", nil))
	if w.Code != http.StatusBadGateway || sinkRequests.Load() != 0 {
		t.Fatalf("followed credential-bearing peer redirect: status=%d sinkRequests=%d", w.Code, sinkRequests.Load())
	}
	if client.CheckRedirect == nil { t.Fatal("mutated caller-owned client") }
}
