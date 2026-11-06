package server

import (
	"github.com/thebrazenbeard/masterdrive/internal/catalog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayRejectsAmbiguousPeerContentRange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
	}{
		{"duplicate", []string{"bytes 2-3/4", "bytes 2-3/4"}},
		{"conflicting", []string{"bytes 2-3/4", "bytes 0-1/4"}},
		{"missing", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", "\"stable\"")
				w.Header().Set("Content-Length", "2")
				for _, v := range tc.values {
					w.Header().Add("Content-Range", v)
				}
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("cd"))
			}))
			defer peer.Close()
			store, err := catalog.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			host, err := store.Upsert("ambiguous", peer.URL)
			if err != nil {
				t.Fatal(err)
			}
			api := New(store, map[string]string{host.ID: "test-secret"}, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
			req.Header.Set("Range", "bytes=2-3")
			req.Header.Set("If-Range", "\"stable\"")
			w := httptest.NewRecorder()
			api.ServeHTTP(w, req)
			if w.Code != http.StatusBadGateway || w.Header().Get("Content-Disposition") != "" {
				t.Fatalf("ambiguous range accepted: status=%d headers=%v", w.Code, w.Header())
			}
		})
	}
}
