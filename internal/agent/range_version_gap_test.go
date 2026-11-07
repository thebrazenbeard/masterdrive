package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Demonstrates that two successful legacy range responses need not refer to
// the same source version. This is a negative security qualification, not a fix.
func TestLegacyRangesCanMixSourceVersions(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "payload.txt")
	if err := os.WriteFile(file, []byte("AAAABBBB"), 0600); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 48)
	h, err := NewHandler([]Export{{ID: "docs", Name: "Docs", Path: root}}, token)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	read := func(rng string) string {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/v1/download?share=docs&path=payload.txt", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Range", rng)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusPartialContent {
			t.Fatalf("range=%s status=%d body=%s", rng, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	first := read("bytes=0-3")
	if err := os.WriteFile(file, []byte("CCCCDDDD"), 0600); err != nil {
		t.Fatal(err)
	}
	second := read("bytes=4-7")
	if first+second != "AAAADDDD" {
		t.Fatalf("expected mixed source versions, got %q", first+second)
	}
	if first+second == "AAAABBBB" || first+second == "CCCCDDDD" {
		t.Fatal("unexpected coherent snapshot")
	}
}
