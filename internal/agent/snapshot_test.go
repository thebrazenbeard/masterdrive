package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func snapshotRequest(h http.Handler, rangeValue, match string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/v1/snapshot?share=docs&path=sample.txt", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	if rangeValue != "" {
		r.Header.Set("Range", rangeValue)
	}
	if match != "" {
		r.Header.Set("If-Match", match)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestSnapshotRangePreconditionsAndMutation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(file, []byte("AAAABBBB"), 0600); err != nil {
		t.Fatal(err)
	}
	h := testHandler(t, []Export{{ID: "docs", Name: "Docs", Path: dir}})
	initial := snapshotRequest(h, "", "")
	if initial.Code != 200 || initial.Body.String() != "AAAABBBB" {
		t.Fatalf("initial %d %q", initial.Code, initial.Body.String())
	}
	tag := initial.Header().Get("ETag")
	if len(tag) != 66 || !strings.HasPrefix(tag, "\"") {
		t.Fatalf("bad ETag %q", tag)
	}
	if w := snapshotRequest(h, "bytes=0-3", ""); w.Code != 412 {
		t.Fatalf("unpinned range %d", w.Code)
	}
	first := snapshotRequest(h, "bytes=0-3", tag)
	if first.Code != 206 || first.Body.String() != "AAAA" {
		t.Fatalf("first range %d %q", first.Code, first.Body.String())
	}
	if err := os.WriteFile(file, []byte("CCCCDDDD"), 0600); err != nil {
		t.Fatal(err)
	}
	second := snapshotRequest(h, "bytes=4-7", tag)
	if second.Code != 412 {
		t.Fatalf("stale range %d %q", second.Code, second.Body.String())
	}
	fresh := snapshotRequest(h, "", "")
	if fresh.Code != 200 || fresh.Body.String() != "CCCCDDDD" || fresh.Header().Get("ETag") == tag {
		t.Fatalf("fresh representation %d %q", fresh.Code, fresh.Body.String())
	}
	if fresh.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
}

func TestSnapshotRejectsOversizeAndUnauthorized(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.txt")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(SnapshotLimit + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	h := testHandler(t, []Export{{ID: "docs", Name: "Docs", Path: dir}})
	if w := snapshotRequest(h, "", ""); w.Code != 413 {
		t.Fatalf("oversize %d", w.Code)
	}
	if w := request(h, "GET", "/v1/snapshot?share=docs&path=sample.txt", "wrong"); w.Code != 401 {
		t.Fatalf("unauthorized %d", w.Code)
	}
}

func TestSnapshotCaptureCapacityFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("AAAABBBB"), 0600); err != nil {
		t.Fatal(err)
	}
	h := testHandler(t, []Export{{ID: "docs", Name: "Docs", Path: dir}})
	for i := 0; i < cap(snapshotSlots); i++ {
		snapshotSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(snapshotSlots); i++ {
			<-snapshotSlots
		}
	}()
	// Invalid and ungranted requests must not consume scarce capture slots.
	// These responses remain stable even when all capture slots are occupied.
	invalid := request(h, "GET", "/v1/snapshot?share=docs", testToken)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("missing path under saturation: %d", invalid.Code)
	}
	denied := request(h, "GET", "/v1/snapshot?share=ungranted&path=sample.txt", testToken)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("ungranted share under saturation: %d", denied.Code)
	}
	missing := request(h, "GET", "/v1/snapshot?share=docs&path=absent.txt", testToken)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing file under saturation: %d", missing.Code)
	}
	w := snapshotRequest(h, "", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("capacity response %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
}
