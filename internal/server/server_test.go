package server

import (
	"encoding/json"
	"errors"
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

func TestControllerReloadsExternalCLIChanges(t *testing.T) {
	dir := t.TempDir()
	controllerStore, err := catalog.Open(dir)
	if err != nil { t.Fatal(err) }
	cliStore, err := catalog.Open(dir)
	if err != nil { t.Fatal(err) }
	api := New(controllerStore, nil, nil)
	host, err := cliStore.Upsert("external-device", "https://approved.example")
	if err != nil { t.Fatal(err) }
	list := httptest.NewRecorder()
	api.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), host.ID) {
		t.Fatalf("newly added host hidden until restart: %d %s", list.Code, list.Body.String())
	}
	if err := cliStore.Remove(host.ID); err != nil { t.Fatal(err) }
	removed := httptest.NewRecorder()
	api.ServeHTTP(removed, httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil))
	if removed.Code != http.StatusOK || strings.Contains(removed.Body.String(), host.ID) {
		t.Fatalf("removed host remained visible: %d %s", removed.Code, removed.Body.String())
	}
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
	if err != nil {
		// net/http may abort before flushing buffered downstream headers.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) { return }
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr == nil {
		t.Fatalf("truncated download was reported as complete: status=%d bytes=%d", resp.StatusCode, len(body))
	}
}

func TestGatewayForwardsSingleByteRange(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=1-3" || r.Header.Get("If-Range") != "\"fixture-v1\"" { http.Error(w, "unpinned range", http.StatusBadRequest); return }
		w.Header().Set("ETag", "\"fixture-v1\"")
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
	req.Header.Set("If-Range", "\"fixture-v1\"")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatal(err) }
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil { t.Fatal(err) }
	if resp.StatusCode != http.StatusPartialContent || string(body) != "bcd" || resp.Header.Get("Content-Range") != "bytes 1-3/6" {
		t.Fatalf("range status=%d bytes=%q content-range=%q", resp.StatusCode, body, resp.Header.Get("Content-Range"))
	}
}

func TestSingleRangeValidation(t *testing.T) {
	cases := []struct { request, response string; length int64; valid bool }{
		{"bytes=1-3", "bytes 1-3/6", 3, true},
		{"bytes=1-", "bytes 1-5/6", 5, true},
		{"bytes=-2", "bytes 4-5/6", 2, true},
		{"bytes=1-99", "bytes 1-5/6", 5, true},
		{"bytes=1-3", "bytes 10-12/100", 3, false},
		{"bytes=1-3", "bytes 1-3/6", 2, false},
		{"bytes=1-3", "bytes 1-4/6", 4, false},
		{"bytes=1-3", "bytes 1-3/*", 3, false},
		{"bytes=-0", "bytes 0-5/6", 6, false},
		{"bytes=6-7", "bytes 6-7/6", 2, false},
	}
	for _, tc := range cases {
		if got := validSingleRangeResponse(tc.request, tc.response, tc.length); got != tc.valid {
			t.Errorf("request=%q response=%q length=%d got=%t want=%t", tc.request, tc.response, tc.length, got, tc.valid)
		}
	}
}

func TestGatewayRejectsMismatchedPeerByteRange(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "3")
		w.Header().Set("ETag", "\"fixture-v1\"")
		w.Header().Set("Content-Range", "bytes 10-12/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("XYZ"))
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("malformed-range", peer.URL)
	if err != nil { t.Fatal(err) }
	controller := httptest.NewServer(New(store, map[string]string{host.ID: "test-secret"}, nil))
	defer controller.Close()
	req, err := http.NewRequest(http.MethodGet, controller.URL+"/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
	if err != nil { t.Fatal(err) }
	req.Header.Set("Range", "bytes=1-3")
	req.Header.Set("If-Range", "\"fixture-v1\"")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatal(err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway { t.Fatalf("mismatched peer range accepted: %d", resp.StatusCode) }
	if resp.Header.Get("Content-Disposition") != "" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("range rejection leaked attachment headers: %+v", resp.Header)
	}
	var responseBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		t.Fatalf("gateway rejection must be valid JSON: %v", err)
	}
}

func TestGatewayIfRangeRequiresStrongValidator(t *testing.T) {
	var contacted atomic.Bool
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		contacted.Store(true)
		http.Error(w, "unexpected proxy request", http.StatusInternalServerError)
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("conditional", peer.URL)
	if err != nil { t.Fatal(err) }
	api := New(store, map[string]string{host.ID: "test-secret"}, nil)
	for _, value := range []string{"W/\"prior\"", "Mon, 02 Nov 2026 00:00:00 GMT", "no-quotes", "\"newline\ninvalid\"", "\"a\", \"b\""} {
		t.Run(value, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
			request.Header.Set("Range", "bytes=2-3")
			request.Header.Set("If-Range", value)
			w := httptest.NewRecorder()
			api.ServeHTTP(w, request)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_if_range") {
				t.Fatalf("unsafe validator accepted: status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	if contacted.Load() { t.Fatal("invalid conditional range contacted peer") }
}

func TestGatewayIfRangeFailsClosedForChangedOrUnverifiablePeer(t *testing.T) {
	for _, tc := range []struct {
		name, etag string
		status, want int
		duplicate bool
	}{
		{name: "changed representation", status: http.StatusOK, etag: "\"changed\"", want: http.StatusPreconditionFailed},
		{name: "missing peer validator", status: http.StatusPartialContent, want: http.StatusBadGateway},
		{name: "different peer validator", status: http.StatusPartialContent, etag: "\"changed\"", want: http.StatusBadGateway},
		{name: "weak peer validator", status: http.StatusPartialContent, etag: "W/\"prior\"", want: http.StatusBadGateway},
		{name: "duplicated peer validator", status: http.StatusPartialContent, etag: "\"prior\"", duplicate: true, want: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "bytes=2-3" || r.Header.Get("If-Range") != "\"prior\"" {
					t.Errorf("conditional headers not forwarded: %+v", r.Header)
				}
				w.Header().Set("Content-Length", "2")
				w.Header().Set("ETag", tc.etag)
				if tc.duplicate { w.Header().Add("ETag", tc.etag) }
				if tc.status == http.StatusPartialContent { w.Header().Set("Content-Range", "bytes 2-3/4") }
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("cd"))
			}))
			defer peer.Close()
			store, err := catalog.Open(t.TempDir())
			if err != nil { t.Fatal(err) }
			host, err := store.Upsert("conditional", peer.URL)
			if err != nil { t.Fatal(err) }
			api := New(store, map[string]string{host.ID: "test-secret"}, nil)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
			request.Header.Set("Range", "bytes=2-3")
			request.Header.Set("If-Range", "\"prior\"")
			w := httptest.NewRecorder()
			api.ServeHTTP(w, request)
			if w.Code != tc.want || w.Header().Get("Content-Disposition") != "" || w.Header().Get("ETag") != "" {
				t.Fatalf("unsafe resume: status=%d want=%d headers=%v body=%s", w.Code, tc.want, w.Header(), w.Body.String())
			}
		})
	}
}

func TestGatewayIfRangeAcceptsVerifiedStrongPeerTag(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Range") != "\"stable-content-sha256\"" {
			t.Errorf("conditional tag not forwarded: %q", r.Header.Get("If-Range"))
		}
		w.Header().Set("ETag", "\"stable-content-sha256\"")
		w.Header().Set("Content-Length", "2")
		w.Header().Set("Content-Range", "bytes 2-3/4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("cd"))
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("conditional", peer.URL)
	if err != nil { t.Fatal(err) }
	api := New(store, map[string]string{host.ID: "test-secret"}, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
	request.Header.Set("Range", "bytes=2-3")
	request.Header.Set("If-Range", "\"stable-content-sha256\"")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, request)
	if w.Code != http.StatusPartialContent || w.Body.String() != "cd" || w.Header().Get("ETag") != "\"stable-content-sha256\"" {
		t.Fatalf("validated range failed: status=%d body=%q headers=%v", w.Code, w.Body.String(), w.Header())
	}
}

func TestStrongETagValidation(t *testing.T) {
	for _, tc := range []struct { value string; want bool }{
		{"\"sha256-sample\"", true},
		{"\"\"", true},
		{"W/\"sha256-sample\"", false},
		{"\"a\", \"b\"", false},
		{"\"contains space\"", false},
		{"\"contains\"quote\"", false},
		{"\"has\\tab\"", true},
		{"\"has\x7fcontrol\"", false},
	} {
		if got := validStrongETag(tc.value); got != tc.want {
			t.Errorf("validator=%q got=%t want=%t", tc.value, got, tc.want)
		}
	}
}
