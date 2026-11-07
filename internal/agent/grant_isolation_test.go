package agent

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedGrantsCannotCrossShareBoundaries(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, p := range []string{a, b} {
		if err := os.WriteFile(filepath.Join(p, "secret.txt"), []byte("distinct-private-content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	tokenA := "scoped-grant-A-with-at-least-thirty-two-bytes"
	tokenB := "scoped-grant-B-with-at-least-thirty-two-bytes"
	h, err := NewHandlerWithGrants([]Export{{ID: "a", Name: "Alpha", Path: a}, {ID: "b", Name: "Beta", Path: b}}, []AccessGrant{{BearerToken: tokenA, ShareIDs: []string{"a"}}, {BearerToken: tokenB, ShareIDs: []string{"b"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	routes := []string{"/v1/shares", "/v1/list?share=b", "/v1/search?share=b&q=secret", "/v1/download?share=b&path=secret.txt"}
	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			w := request(h, http.MethodGet, route, tokenA)
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
			if strings.Contains(w.Body.String(), "Beta") || strings.Contains(w.Body.String(), "distinct-private-content") || strings.Contains(w.Body.String(), b) {
				t.Fatalf("cross-grant disclosure: %s", w.Body.String())
			}
			if route == "/v1/shares" {
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Alpha") {
					t.Fatalf("own share hidden: %d %s", w.Code, w.Body.String())
				}
			} else if w.Code != http.StatusNotFound {
				t.Fatalf("cross-grant route must return 404: %s got %d body %s", route, w.Code, w.Body.String())
			}
		})
	}
	if w := request(h, http.MethodGet, "/v1/download?share=b&path=secret.txt", tokenB); w.Code != http.StatusOK {
		t.Fatalf("valid grant denied: %d %s", w.Code, w.Body.String())
	}
}
