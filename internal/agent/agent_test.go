package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testToken = "test-token-with-more-than-thirty-two-random-looking-bytes"

func testHandler(t *testing.T, exports []Export) http.Handler {
	t.Helper()
	h, err := NewHandler(exports, testToken)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Errorf("close export roots: %v", err)
		}
	})
	return h
}

func request(h http.Handler, method, url, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestExportRequiresExplicitConfiguration(t *testing.T) {
	root := t.TempDir()
	if _, err := NewHandler([]Export{{ID: "x", Name: "Docs", Path: root}}, "too-short"); err == nil {
		t.Fatal("accepted short token")
	}
	if _, err := NewHandler([]Export{{ID: "x", Name: "Docs", Path: "relative"}}, testToken); err == nil {
		t.Fatal("accepted relative export path")
	}
	if _, err := NewHandler([]Export{{ID: "a", Name: "Docs", Path: root}, {ID: "a", Name: "Other", Path: root}}, testToken); err == nil {
		t.Fatal("accepted duplicate share identifier")
	}
	if _, err := NewHandler([]Export{{ID: "../x", Name: "Docs", Path: root}}, testToken); err == nil {
		t.Fatal("accepted path-like share identifier")
	}
	if _, err := NewHandler([]Export{{ID: "x", Name: "Docs", Path: filepath.Join(root, "missing")}}, testToken); err == nil {
		t.Fatal("accepted missing export path")
	}
}

func TestSharesNeverExposeLocalPaths(t *testing.T) {
	root := t.TempDir()
	h := testHandler(t, []Export{{ID: "photos", Name: "Photo Archive", Path: root}})
	for _, unauthorizedToken := range []string{"", "wrong-token", testToken + "!"} {
		w := request(h, "GET", "/v1/shares", unauthorizedToken)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("unauthorized token %q: status %d", unauthorizedToken, w.Code)
		}
		if strings.Contains(w.Body.String(), "Photo Archive") {
			t.Fatal("leaked share metadata to unauthenticated client")
		}
	}
	w := request(h, "GET", "/v1/shares", testToken)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Photo Archive") || !strings.Contains(w.Body.String(), `"read_only":true`) {
		t.Fatalf("shares request status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), root) || strings.Contains(w.Body.String(), "Path") {
		t.Fatal("response disclosed local filesystem path")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store on private response")
	}
	if w := request(h, "POST", "/v1/shares", testToken); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("write method was not rejected: %d", w.Code)
	}
}

func TestPinnedRootRejectsExportPathReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows usually disallows renaming a directory held open by os.Root.
		t.Skip("root path rename is blocked by Windows directory handles")
	}
	parent := t.TempDir()
	exportPath := filepath.Join(parent, "export")
	if err := os.Mkdir(exportPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exportPath, "original.txt"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := testHandler(t, []Export{{ID: "docs", Name: "Documents", Path: exportPath}})
	if err := os.Rename(exportPath, filepath.Join(parent, "old-export")); err != nil {
		t.Skipf("root rename unsupported: %v", err)
	}
	if err := os.Mkdir(exportPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exportPath, "unexpected.txt"), []byte("bait"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := request(h, "GET", "/v1/download?share=docs&path=unexpected.txt", testToken); w.Code == http.StatusOK || strings.Contains(w.Body.String(), "bait") {
		t.Fatalf("replaced path exposed alternate content: status=%d body=%q", w.Code, w.Body.String())
	}
	if w := request(h, "GET", "/v1/download?share=docs&path=original.txt", testToken); w.Code != http.StatusOK || w.Body.String() != "original" {
		t.Fatalf("lost pinned directory after rename: status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestListPaginationAndDownloadRanges(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 205; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("part-%03d.txt", i)), []byte("sample"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "hello.txt"), []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := testHandler(t, []Export{{ID: "docs", Name: "Documents", Path: root}})

	w := request(h, "GET", "/v1/list?share=docs&limit=200", testToken)
	if w.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", w.Code, w.Body.String())
	}
	var first struct {
		Entries    []Entry `json:"entries"`
		NextOffset *int    `json:"next_offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 200 || first.NextOffset == nil || *first.NextOffset != 200 {
		t.Fatalf("unexpected first page: count=%d cursor=%v", len(first.Entries), first.NextOffset)
	}
	w = request(h, "GET", "/v1/list?share=docs&limit=200&offset=200", testToken)
	var second struct {
		Entries    []Entry `json:"entries"`
		NextOffset *int    `json:"next_offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 6 || second.NextOffset != nil {
		t.Fatalf("unexpected second page: count=%d cursor=%v", len(second.Entries), second.NextOffset)
	}
	w = request(h, "GET", "/v1/list?share=docs&path=nested", testToken)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "nested/hello.txt") {
		t.Fatalf("nested listing status=%d body=%s", w.Code, w.Body.String())
	}

	w = request(h, "GET", "/v1/download?share=docs&path=nested/hello.txt", testToken)
	if w.Code != http.StatusOK || w.Body.String() != "abcdef" {
		t.Fatalf("download status=%d body=%q", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/v1/download?share=docs&path=nested/hello.txt", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("Range", "bytes=1-3")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusPartialContent || w.Body.String() != "bcd" {
		t.Fatalf("range download status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestTraversalAndNonFileReadsFailClosed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	h := testHandler(t, []Export{{ID: "safe", Name: "Safe", Path: root}})
	bad := []string{
		"/v1/list?share=safe&path=../",
		"/v1/list?share=safe&path=../secret.txt",
		"/v1/list?share=safe&path=folder/../../secret.txt",
		"/v1/list?share=safe&path=%2Fetc%2Fpasswd",
		"/v1/list?share=safe&path=folder%5Csecret.txt",
		"/v1/list?share=safe&path=folder%2F",
		"/v1/list?share=safe&offset=-1",
		"/v1/list?share=safe&offset=10001",
		"/v1/list?share=safe&limit=201",
		"/v1/download?share=safe&path=folder",
		"/v1/download?share=safe",
	}
	for _, url := range bad {
		w := request(h, "GET", url, testToken)
		if w.Code >= 200 && w.Code < 300 {
			t.Errorf("accepted forbidden path: %s -> %d", url, w.Code)
		}
		if strings.Contains(w.Body.String(), "private") {
			t.Errorf("leaked contents from rejected path: %s", url)
		}
	}
	if runtime.GOOS == "windows" {
		// Creating Windows symlinks usually requires developer mode/privilege.
		return
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, url := range []string{
		"/v1/download?share=safe&path=escape/secret.txt",
		"/v1/list?share=safe&path=escape",
	} {
		w := request(h, "GET", url, testToken)
		if w.Code >= 200 && w.Code < 300 {
			t.Errorf("escaped export via symlink: %s -> %d", url, w.Code)
		}
	}
}

func TestScopedGrantsHideOtherSharesAndBlockTheirBytes(t *testing.T) {
	photos := t.TempDir()
	private := t.TempDir()
	if err := os.WriteFile(filepath.Join(photos, "photo.txt"), []byte("public-photo"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(private, "passwords.txt"), []byte("never-expose-me"), 0600); err != nil { t.Fatal(err) }
	photoToken := "photo-only-test-token-0123456789abcdefghijklmnopqrstuvwxyz"
	privateToken := "private-only-test-token-0123456789abcdefghijklmnopqrstuvwxyz"
	h, err := NewHandlerWithGrants([]Export{
		{ID:"photos", Name:"Photographs", Path:photos},
		{ID:"private", Name:"Private", Path:private},
	}, []AccessGrant{
		{BearerToken:photoToken, ShareIDs:[]string{"photos"}},
		{BearerToken:privateToken, ShareIDs:[]string{"private"}},
	})
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ if err := h.Close(); err != nil {t.Error(err)} })
	for _, tc := range []struct{token, allowedShare, deniedShare, allowedFile, deniedFile, deniedName string}{
		{photoToken,"photos","private","photo.txt","passwords.txt","Private"},
		{privateToken,"private","photos","passwords.txt","photo.txt","Photographs"},
	} {
		w := request(h, http.MethodGet, "/v1/shares", tc.token)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.allowedShare) ||
			strings.Contains(w.Body.String(), tc.deniedShare) || strings.Contains(w.Body.String(), tc.deniedName) {
			t.Fatalf("grant inventory leaked share: %d %s", w.Code, w.Body.String())
		}
		for _, route := range []string{
			"/v1/list?share="+tc.deniedShare,
			"/v1/download?share="+tc.deniedShare+"&path="+tc.deniedFile,
		} {
			w = request(h, http.MethodGet, route, tc.token)
			if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "never-expose-me") {
				t.Fatalf("unauthorized share was disclosed: %s -> %d %s", route, w.Code, w.Body.String())
			}
		}
		for _, route := range []string{
			"/v1/list?share="+tc.allowedShare,
			"/v1/download?share="+tc.allowedShare+"&path="+tc.allowedFile,
		} {
			w = request(h, http.MethodGet, route, tc.token)
			if w.Code != http.StatusOK { t.Fatalf("authorized share was denied: %s -> %d %s", route,w.Code,w.Body.String()) }
		}
	}
	if w := request(h, http.MethodGet, "/v1/shares", "wrong"); w.Code != http.StatusUnauthorized {
		t.Fatalf("unconfigured token status=%d", w.Code)
	}
}

func TestScopedGrantConfigurationMustBeExplicitAndNonAmbiguous(t *testing.T) {
	exports := []Export{{ID:"photos",Name:"Photos",Path:t.TempDir()}}
	goodToken := "sufficient-length-grant-token-abcdefghijklmnopqrstuvwxyz"
	bad := [][]AccessGrant{
		nil,
		{},
		{{BearerToken:"short",ShareIDs:[]string{"photos"}}},
		{{BearerToken:goodToken,ShareIDs:nil}},
		{{BearerToken:goodToken,ShareIDs:[]string{"missing"}}},
		{{BearerToken:goodToken,ShareIDs:[]string{"photos","photos"}}},
		{{BearerToken:goodToken,ShareIDs:[]string{"photos"}},
		 {BearerToken:goodToken,ShareIDs:[]string{"photos"}}},
	}
	for i, grants := range bad {
		if h, err := NewHandlerWithGrants(exports, grants); err == nil {
			_ = h.Close()
			t.Errorf("accepted unsafe grants case %d", i)
		}
	}
}

func TestLegacyHandlerTokenRetainsAccessToEveryExplicitExport(t *testing.T) {
	first, second := t.TempDir(),t.TempDir()
	h := testHandler(t, []Export{{ID:"one",Name:"One",Path:first},{ID:"two",Name:"Two",Path:second}})
	w := request(h, http.MethodGet, "/v1/shares", testToken)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "One") || !strings.Contains(w.Body.String(), "Two") {
		t.Fatalf("legacy API unexpectedly lost all-shares access: %d %s",w.Code,w.Body.String())
	}
}

func TestReplacingHandlerDropsOldGrantButNotExistingFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("allowed-file"), 0600); err != nil { t.Fatal(err) }
	exports := []Export{{ID:"approved", Name:"Approved", Path:root}}
	oldToken := "old-scoped-grant-token-abcdefghijklmnopqrstuvwxyz-00001"
	newToken := "new-scoped-grant-token-abcdefghijklmnopqrstuvwxyz-00002"
	oldHandler, err := NewHandlerWithGrants(exports, []AccessGrant{{BearerToken:oldToken, ShareIDs:[]string{"approved"}}})
	if err != nil { t.Fatal(err) }
	if w := request(oldHandler,http.MethodGet,"/v1/download?share=approved&path=fixture.txt",oldToken); w.Code != http.StatusOK {
		t.Fatalf("initial grant unavailable: HTTP %d",w.Code)
	}
	if err := oldHandler.Close(); err != nil { t.Fatal(err) }
	newHandler, err := NewHandlerWithGrants(exports, []AccessGrant{{BearerToken:newToken, ShareIDs:[]string{"approved"}}})
	if err != nil { t.Fatal(err) }
	defer newHandler.Close()
	if w := request(newHandler,http.MethodGet,"/v1/download?share=approved&path=fixture.txt",oldToken); w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(),"allowed-file") {
		t.Fatalf("removed grant remained authorized: HTTP %d %s",w.Code,w.Body.String())
	}
	if w := request(newHandler,http.MethodGet,"/v1/download?share=approved&path=fixture.txt",newToken); w.Code != http.StatusOK || w.Body.String() != "allowed-file" {
		t.Fatalf("replacement grant failed: HTTP %d %s",w.Code,w.Body.String())
	}
}
