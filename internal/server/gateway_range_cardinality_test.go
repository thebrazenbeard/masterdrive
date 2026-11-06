package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

func TestGatewayRejectsAmbiguousOrEmptyClientRangeBeforePeer(t *testing.T) {
	calls := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusOK) }))
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host, err := store.Upsert("range-cardinality", peer.URL)
	if err != nil {
		t.Fatal(err)
	}
	api := New(store, map[string]string{host.ID: "test-token"}, nil)
	path := "/api/v1/hosts/" + host.ID + "/shares/docs/download?path=test.bin"
	for _, tc := range []struct {
		name   string
		ranges []string
	}{
		{"empty", []string{""}},
		{"duplicate", []string{"bytes=0-1", "bytes=0-1"}},
		{"conflicting", []string{"bytes=0-1", "bytes=2-3"}},
		{"empty-first", []string{"", "bytes=0-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			for _, value := range tc.ranges {
				req.Header.Add("Range", value)
			}
			recorder := httptest.NewRecorder()
			api.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if calls != 0 {
				t.Fatalf("peer called %d times", calls)
			}
		})
	}
}
