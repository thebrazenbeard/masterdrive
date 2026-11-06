package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

func TestGatewayRejectsContentRangeOnFullPeerResponse(t *testing.T) {
	for _, value := range []string{"bytes 0-1/2", "bytes 0-1/2, bytes 0-1/2"} {
		t.Run(value, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Range", value)
				w.Header().Set("Content-Length", "2")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}))
			defer peer.Close()
			store, err := catalog.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			host, err := store.Upsert("peer", peer.URL)
			if err != nil {
				t.Fatal(err)
			}
			api := New(store, map[string]string{host.ID: "test-secret"}, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares/docs/download?path=test.bin", nil)
			recorder := httptest.NewRecorder()
			api.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadGateway || recorder.Header().Get("Content-Disposition") != "" {
				t.Fatalf("accepted malformed 200 response: status=%d headers=%v", recorder.Code, recorder.Header())
			}
		})
	}
}
