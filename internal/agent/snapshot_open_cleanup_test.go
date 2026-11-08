package agent

import (
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"
)

// The open-admission token must be released on every early-return path.
// These tests use only disposable files and synthetic HTTP requests.
func TestSnapshotOpenBudgetReleasesOnEarlyReturns(t *testing.T) {
    if len(snapshotOpenSlots) != 0 || len(snapshotSlots) != 0 {
        t.Fatal("snapshot admission channels unexpectedly occupied before test")
    }
    dir := t.TempDir()
    if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("contents"), 0600); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(filepath.Join(dir, "directory-placeholder"), []byte("x"), 0600); err != nil {
        t.Fatal(err)
    }
    oversized, err := os.Create(filepath.Join(dir, "large.txt"))
    if err != nil {
        t.Fatal(err)
    }
    if err := oversized.Truncate(SnapshotLimit + 1); err != nil {
        oversized.Close()
        t.Fatal(err)
    }
    if err := oversized.Close(); err != nil {
        t.Fatal(err)
    }

    h := testHandler(t, []Export{{ID: "docs", Name: "Docs", Path: dir}})
    cases := []struct {
        name string
        url string
        want int
    }{
        {"ungranted-share", "/v1/snapshot?share=other&path=sample.txt", http.StatusNotFound},
        {"missing-file", "/v1/snapshot?share=docs&path=missing.txt", http.StatusNotFound},
        {"directory", "/v1/snapshot?share=docs&path=.", http.StatusBadRequest},
        {"oversized", "/v1/snapshot?share=docs&path=large.txt", http.StatusRequestEntityTooLarge},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            w := request(h, "GET", tc.url, testToken)
            if w.Code != tc.want {
                t.Fatalf("status = %d, want %d", w.Code, tc.want)
            }
            if got := len(snapshotOpenSlots); got != 0 {
                t.Fatalf("leaked open-admission tokens: %d", got)
            }
            if got := len(snapshotSlots); got != 0 {
                t.Fatalf("leaked capture-admission tokens: %d", got)
            }
        })
    }

    // Obtain the exact representation ETag before testing range parsing.
    // Without If-Match, the handler returns 412 before reaching ServeContent.
    initial := snapshotRequest(h, "", "")
    if initial.Code != http.StatusOK {
        t.Fatalf("initial snapshot status = %d", initial.Code)
    }
    etag := initial.Header().Get("ETag")
    if etag == "" {
        t.Fatal("initial snapshot missing ETag")
    }
    if got := len(snapshotOpenSlots); got != 0 {
        t.Fatalf("open-admission token leaked after initial snapshot: %d", got)
    }
    if got := len(snapshotSlots); got != 0 {
        t.Fatalf("capture-admission token leaked after initial snapshot: %d", got)
    }
    t.Run("unsatisfiable-range", func(t *testing.T) {
        w := snapshotRequest(h, "bytes=999-1000", etag)
        if w.Code != http.StatusRequestedRangeNotSatisfiable {
            t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestedRangeNotSatisfiable)
        }
        if got := len(snapshotOpenSlots); got != 0 {
            t.Fatalf("leaked open-admission tokens: %d", got)
        }
        if got := len(snapshotSlots); got != 0 {
            t.Fatalf("leaked capture-admission tokens: %d", got)
        }
    })

    // If-Range must agree with the captured representation even when
    // If-Match is valid; this path is distinct from stale If-Match.
    t.Run("stale-if-range", func(t *testing.T) {
        req := httptest.NewRequest(http.MethodGet, "/v1/snapshot?share=docs&path=sample.txt", nil)
        req.Header.Set("Authorization", "Bearer "+testToken)
        req.Header.Set("Range", "bytes=0-2")
        req.Header.Set("If-Match", etag)
        req.Header.Set("If-Range", "\"stale\"")
        w := httptest.NewRecorder()
        h.ServeHTTP(w, req)
        if w.Code != http.StatusPreconditionFailed {
            t.Fatalf("status = %d, want %d", w.Code, http.StatusPreconditionFailed)
        }
        if len(snapshotOpenSlots) != 0 || len(snapshotSlots) != 0 {
            t.Fatalf("admission tokens leaked: open=%d capture=%d", len(snapshotOpenSlots), len(snapshotSlots))
        }
    })

    // Representation precondition and range failures happen after capture.
    // They must release both budgets, not just early filesystem failures.
    for _, tc := range []struct {
        name string
        rangeHeader string
        match string
        want int
    }{
        {"stale-version", "bytes=0-2", "\"stale\"", http.StatusPreconditionFailed},
        {"unversioned-range", "bytes=0-2", "", http.StatusPreconditionFailed},
    } {
        t.Run(tc.name, func(t *testing.T) {
            w := snapshotRequest(h, tc.rangeHeader, tc.match)
            if w.Code != tc.want {
                t.Fatalf("status = %d, want %d", w.Code, tc.want)
            }
            if got := len(snapshotOpenSlots); got != 0 {
                t.Fatalf("leaked open-admission tokens: %d", got)
            }
            if got := len(snapshotSlots); got != 0 {
                t.Fatalf("leaked capture-admission tokens: %d", got)
            }
        })
    }

    // Saturating capture admission forces the post-open rejection path.
    for i := 0; i < cap(snapshotSlots); i++ {
        snapshotSlots <- struct{}{}
    }
    defer func() {
        for i := 0; i < cap(snapshotSlots); i++ {
            <-snapshotSlots
        }
    }()
    w := snapshotRequest(h, "", "")
    if w.Code != http.StatusServiceUnavailable {
        t.Fatalf("capture saturation status = %d", w.Code)
    }
    if got := len(snapshotOpenSlots); got != 0 {
        t.Fatalf("open-admission token leaked after capture saturation: %d", got)
    }
}
