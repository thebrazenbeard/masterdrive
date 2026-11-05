package server

import (
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/thebrazenbeard/masterdrive/internal/catalog"
)

func TestGatewayRejectsDuplicateClientRanges(t *testing.T) {
 store, err := catalog.Open(t.TempDir())
 if err != nil { t.Fatal(err) }
 host, err := store.Upsert("test-peer", "http://127.0.0.1:1")
 if err != nil { t.Fatal(err) }
 api := New(store, map[string]string{host.ID: "test-token"}, nil)
 for _, values := range [][]string{
  {"bytes=0-1", "bytes=2-3"},
  {"bytes=0-1", "bytes=0-1"},
  {"", "bytes=0-1"},
 } {
  req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/"+host.ID+"/shares/photos/download?path=file", nil)
  req.Header["Range"] = values
  rec := httptest.NewRecorder()
  api.ServeHTTP(rec, req)
  if rec.Code != http.StatusBadRequest { t.Fatalf("ranges=%q status=%d body=%s", values, rec.Code, rec.Body.String()) }
 }
}
