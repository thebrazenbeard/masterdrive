package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const searchTestToken = "only-for-search-tests-0123456789-0123456789"

func TestAgentSearchIsAuthenticatedShallowAndBounded(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil { t.Fatal(err) }
	for path, body := range map[string]string{
		"Report.txt": "visible", "unrelated.txt": "ignore", "nested/Report-secret.txt": "not recursively found",
	} {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0600); err != nil { t.Fatal(err) }
	}
	h, err := NewHandler([]Export{{ID:"docs", Name:"Documents", Path:dir}}, searchTestToken)
	if err != nil { t.Fatal(err) }
	defer h.Close()

	request := func(target, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		if token != "" { r.Header.Set("Authorization", "Bearer "+token) }
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("/v1/search?share=docs&q=report", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("search did not require bearer token: %d", w.Code)
	}
	w := request("/v1/search?share=docs&q=report", searchTestToken)
	if w.Code != http.StatusOK { t.Fatalf("search failed: %d %s", w.Code, w.Body.String()) }
	var body struct {
		Scope string `json:"scope"`
		Entries []Entry `json:"entries"`
		NextOffset *int `json:"next_offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil { t.Fatal(err) }
	if body.Scope != "immediate_children" || len(body.Entries) != 1 || body.Entries[0].Name != "Report.txt" || body.NextOffset != nil {
		t.Fatalf("search leaked nested file or wrong scope: %+v", body)
	}
	for _, target := range []string{
		"/v1/search?share=docs&q=%20", "/v1/search?share=docs&q=ok&limit=51",
		"/v1/search?share=docs&q=ok&offset=10001", "/v1/search?share=docs&q=ok&path=..%2F",
	} {
		if bad := request(target, searchTestToken); bad.Code < 400 {
			t.Fatalf("invalid search accepted: %s -> %d", target, bad.Code)
		}
	}
}

func TestAgentSearchWindowMakesIncompleteResultsExplicit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 300; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("item-%03d.txt", i)), []byte("x"), 0600); err != nil { t.Fatal(err) }
	}
	h, err := NewHandler([]Export{{ID:"docs", Name:"Documents", Path:dir}}, searchTestToken)
	if err != nil { t.Fatal(err) }
	defer h.Close()
	query := func(offset int) struct {
		Entries []Entry `json:"entries"`
		Scanned int `json:"scanned"`
		NextOffset *int `json:"next_offset"`
	} {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/search?share=docs&q=nomatch&offset=%d", offset), nil)
		r.Header.Set("Authorization", "Bearer "+searchTestToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK { t.Fatalf("search failed: %d %s", w.Code, w.Body.String()) }
		var response struct {
			Entries []Entry `json:"entries"`
			Scanned int `json:"scanned"`
			NextOffset *int `json:"next_offset"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil { t.Fatal(err) }
		return response
	}
	first := query(0)
	if first.Scanned != 256 || first.NextOffset == nil || *first.NextOffset != 256 || len(first.Entries) != 0 {
		t.Fatalf("unbounded or incomplete window not marked: %+v", first)
	}
	second := query(*first.NextOffset)
	if second.Scanned != 44 || second.NextOffset != nil || len(second.Entries) != 0 {
		t.Fatalf("continuation incomplete: %+v", second)
	}
	if strings.Contains(fmt.Sprint(second), "item-") { t.Fatal("nonmatching filenames surfaced") }
}

func TestScopedSearchCannotEnumerateAnUngrantedShare(t *testing.T) {
	docs := t.TempDir()
	private := t.TempDir()
	if err := os.WriteFile(filepath.Join(docs, "public-report.txt"), []byte("okay"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(private, "secret-report.txt"), []byte("hidden"), 0600); err != nil { t.Fatal(err) }
	h, err := NewHandlerWithGrants([]Export{
		{ID:"docs", Name:"Documents", Path:docs},
		{ID:"secret", Name:"Private", Path:private},
	}, []AccessGrant{
		{BearerToken:searchTestToken, ShareIDs:[]string{"docs"}},
		{BearerToken:"only-for-private-search-tests-0123456789", ShareIDs:[]string{"secret"}},
	})
	if err != nil { t.Fatal(err) }
	defer h.Close()
	get := func(share string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/v1/search?share="+share+"&q=report", nil)
		r.Header.Set("Authorization", "Bearer "+searchTestToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w,r)
		return w
	}
	if w := get("docs"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "public-report.txt") {
		t.Fatalf("granted search failed: %d %s", w.Code, w.Body.String())
	}
	for _, share := range []string{"secret", "missing"} {
		if w := get(share); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "secret-report") {
			t.Fatalf("ungranted share was exposed: %s -> %d %s", share, w.Code, w.Body.String())
		}
	}
}
