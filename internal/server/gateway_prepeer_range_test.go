package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

func TestGatewayRejectsMalformedRangeBeforePeer(t *testing.T) {
	var calls atomic.Int64
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host, err := store.Upsert("prepeer-range", peer.URL)
	if err != nil {
		t.Fatal(err)
	}
	api := New(store, map[string]string{host.ID: "test-token"}, nil)
	path := "/api/v1/hosts/" + host.ID + "/shares/docs/download?path=test.bin"
	for _, value := range []string{
		"bytes=+1-2", "bytes=1-+2", "bytes= 1-2", "bytes=1-2 ",
		"bytes=1-2-3", "bytes=1,2", "bytes=--1", "bytes=-0",
		"bytes=2-1", "bytes=-", "bytes=9223372036854775808-",
		"bytes=0-9223372036854775808", "bytes=-9223372036854775808",
	} {
		t.Run(value, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Range", value)
			req.Header.Set("If-Range", "\""+"strong-version"+"\"")
			recorder := httptest.NewRecorder()
			api.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("peer contacted %d times", got)
			}
		})
	}
}

func TestValidSingleRangeRequest(t *testing.T) {
	for _, value := range []string{"bytes=0-0", "bytes=0-", "bytes=-1", "bytes=1-2", "bytes=9223372036854775807-"} {
		if !validSingleRangeRequest(value) {
			t.Errorf("rejected valid syntax %q", value)
		}
	}
}

// Reject ambiguous validators before any peer traffic, even with a valid range.
func TestGatewayRejectsAmbiguousIfRangeBeforePeer(t *testing.T) {
	var calls atomic.Int64
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(http.StatusOK) }))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host, err := store.Upsert("validator-preflight", peer.URL)
	if err != nil {
		t.Fatal(err)
	}
	api := New(store, map[string]string{host.ID: "test-token"}, nil)
	path := "/api/v1/hosts/" + host.ID + "/shares/docs/download?path=test.bin"
	for _, tc := range []struct {
		name   string
		values []string
		status int
	}{
		{"empty", []string{""}, http.StatusBadRequest},
		{"weak", []string{"W/\"v1\""}, http.StatusBadRequest},
		{"duplicate", []string{"\"v1\"", "\"v1\""}, http.StatusBadRequest},
		{"conflicting", []string{"\"v1\"", "\"v2\""}, http.StatusBadRequest},
		{"missing", nil, http.StatusPreconditionRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Range", "bytes=0-1")
			for _, v := range tc.values {
				req.Header.Add("If-Range", v)
			}
			recorder := httptest.NewRecorder()
			api.ServeHTTP(recorder, req)
			if recorder.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.status, recorder.Body.String())
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("peer contacted %d times", got)
			}
		})
	}
}
