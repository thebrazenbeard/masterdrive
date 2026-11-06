package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

func TestGatewayRejectsAmbiguousPeerContentEncoding(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
	}{
		{"duplicate_identity", []string{"identity", "identity"}},
		{"identity_then_gzip", []string{"identity", "gzip"}},
		{"gzip_then_identity", []string{"gzip", "identity"}},
		{"comma_joined", []string{"identity, gzip"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "2")
				for _, v := range tc.values {
					w.Header().Add("Content-Encoding", v)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}))
			defer peer.Close()
			store, err := catalog.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			host, err := store.Upsert("encoding", peer.URL)
			if err != nil {
				t.Fatal(err)
			}
			api := New(store, map[string]string{host.ID: "test-secret"}, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
			recorder := httptest.NewRecorder()
			api.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadGateway || recorder.Header().Get("Content-Disposition") != "" {
				t.Fatalf("ambiguous encoding accepted: status=%d headers=%v", recorder.Code, recorder.Header())
			}
		})
	}
}
