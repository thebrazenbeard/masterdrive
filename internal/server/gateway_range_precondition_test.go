package server

import (
	"github.com/thebrazenbeard/masterdrive/internal/catalog"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGatewayRejectsUnpinnedAndAmbiguousRangesBeforePeer(t *testing.T) {
	var contacted atomic.Int64
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted.Add(1)
		w.Header().Set("Content-Length", "6")
		_, _ = w.Write([]byte("secret"))
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host, err := store.Upsert("preconditions", peer.URL)
	if err != nil {
		t.Fatal(err)
	}
	api := New(store, map[string]string{host.ID: "test-secret"}, nil)
	target := "/api/v1/hosts/" + host.ID + "/shares/docs/download?path=test.bin"
	for _, tc := range []struct {
		name       string
		ranges     []string
		validators []string
		want       int
		code       string
	}{
		{"missing validator", []string{"bytes=0-2"}, nil, http.StatusPreconditionRequired, "range_not_pinned"},
		{"duplicate ranges", []string{"bytes=0-2", "bytes=3-5"}, []string{"\"v1\""}, http.StatusBadRequest, "invalid_range"},
		{"empty range", []string{""}, nil, http.StatusBadRequest, "invalid_range"},
		{"orphan validator", nil, []string{"\"v1\""}, http.StatusBadRequest, "invalid_if_range"},
		{"duplicate validators", []string{"bytes=0-2"}, []string{"\"v1\"", "\"v1\""}, http.StatusBadRequest, "invalid_if_range"},
		{"weak validator", []string{"bytes=0-2"}, []string{"W/\"v1\""}, http.StatusBadRequest, "invalid_if_range"},
		{"date validator", []string{"bytes=0-2"}, []string{"Mon, 02 Nov 2026 00:00:00 GMT"}, http.StatusBadRequest, "invalid_if_range"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, target, nil)
			for _, v := range tc.ranges {
				r.Header.Add("Range", v)
			}
			for _, v := range tc.validators {
				r.Header.Add("If-Range", v)
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, r)
			if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("status=%d body=%q want=%d code=%q", w.Code, w.Body.String(), tc.want, tc.code)
			}
			if w.Header().Get("Content-Disposition") != "" || w.Header().Get("ETag") != "" {
				t.Fatalf("rejection leaked file headers: %v", w.Header())
			}
		})
	}
	if contacted.Load() != 0 {
		t.Fatalf("peer contacted %d times for rejected requests", contacted.Load())
	}
}

func TestGatewayUnversionedFullDownloadDoesNotAdvertiseRanges(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4")
		_, _ = w.Write([]byte("data"))
	}))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host, err := store.Upsert("full", peer.URL)
	if err != nil {
		t.Fatal(err)
	}
	api := New(store, map[string]string{host.ID: "test-secret"}, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(b) != "data" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, b)
	}
	if resp.Header.Get("Accept-Ranges") != "" || resp.Header.Get("ETag") != "" {
		t.Fatalf("unversioned body advertised resumable range: %v", resp.Header)
	}
}
