package agent

import (
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"
)

// Positive control for the If-Range mismatch regression: a matching
// representation validator must permit a bounded partial response.
// All files and requests are synthetic; no host shares are accessed.
func TestSnapshotMatchingIfRangeAllowsPartialRead(t *testing.T) {
    if len(snapshotOpenSlots) != 0 || len(snapshotSlots) != 0 {
        t.Fatal("snapshot admission channels occupied before test")
    }
    dir := t.TempDir()
    if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("contents"), 0600); err != nil {
        t.Fatal(err)
    }
    h := testHandler(t, []Export{{ID: "docs", Name: "Docs", Path: dir}})
    makeRequest := func(rangeHeader, match, ifRange string) *httptest.ResponseRecorder {
        t.Helper()
        req := httptest.NewRequest(http.MethodGet, "/v1/snapshot?share=docs&path=sample.txt", nil)
        req.Header.Set("Authorization", "Bearer "+testToken)
        if rangeHeader != "" { req.Header.Set("Range", rangeHeader) }
        if match != "" { req.Header.Set("If-Match", match) }
        if ifRange != "" { req.Header.Set("If-Range", ifRange) }
        w := httptest.NewRecorder()
        h.ServeHTTP(w, req)
        return w
    }
    initial := makeRequest("", "", "")
    if initial.Code != http.StatusOK {
        t.Fatalf("initial status = %d, want 200", initial.Code)
    }
    etag := initial.Header().Get("ETag")
    if etag == "" { t.Fatal("missing representation ETag") }
    partial := makeRequest("bytes=0-2", etag, etag)
    if partial.Code != http.StatusPartialContent {
        t.Fatalf("partial status = %d, want 206; body=%q", partial.Code, partial.Body.String())
    }
    if got := partial.Body.String(); got != "con" {
        t.Fatalf("partial body = %q, want con", got)
    }
    if got := partial.Header().Get("Content-Range"); got != "bytes 0-2/8" {
        t.Fatalf("Content-Range = %q", got)
    }
    if got := partial.Header().Get("ETag"); got != etag {
        t.Fatalf("partial ETag = %q, want %q", got, etag)
    }
    if len(snapshotOpenSlots) != 0 || len(snapshotSlots) != 0 {
        t.Fatalf("admission tokens leaked: open=%d capture=%d", len(snapshotOpenSlots), len(snapshotSlots))
    }
}
