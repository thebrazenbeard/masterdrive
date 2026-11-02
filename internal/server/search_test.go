package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/agent"
	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

const federatedSearchToken = "only-for-federated-search-tests-0123456789"

func TestExplicitFederatedSearchAcrossSelectedShares(t *testing.T) {
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	tokens := make(map[string]string)
	targets := make([]string, 0, 2)
	for _, name := range []string{"west", "east"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "Report-"+name+".txt"), []byte("fixture"), 0600); err != nil { t.Fatal(err) }
		handler, err := agent.NewHandler([]agent.Export{{ID:"docs", Name:"Documents", Path:dir}}, federatedSearchToken)
		if err != nil { t.Fatal(err) }
		peer := httptest.NewServer(handler)
		t.Cleanup(func(){ peer.Close(); _ = handler.Close() })
		host, err := store.Upsert(name, peer.URL)
		if err != nil { t.Fatal(err) }
		tokens[host.ID] = federatedSearchToken
		targets = append(targets, host.ID+":docs")
	}
	params := url.Values{"q": {"report"}}
	for _, target := range targets { params.Add("target", target) }
	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?"+params.Encode(), nil)
	w := httptest.NewRecorder()
	New(store, tokens, nil).ServeHTTP(w, request)
	if w.Code != http.StatusOK { t.Fatalf("federated search failed: %d %s", w.Code, w.Body.String()) }
	var body struct {
		Scope string `json:"scope"`
		Complete bool `json:"complete"`
		Sources []struct {
			HostID string `json:"host_id"`
			ShareID string `json:"share_id"`
			Entries []struct { Name string `json:"name"` } `json:"entries"`
			Error string `json:"error"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil { t.Fatal(err) }
	if !body.Complete || body.Scope != "immediate_children" || len(body.Sources) != 2 {
		t.Fatalf("incorrect scope/completion: %+v", body)
	}
	for _, source := range body.Sources {
		if source.ShareID != "docs" || source.Error != "" || len(source.Entries) != 1 || !strings.HasPrefix(source.Entries[0].Name, "Report-") {
			t.Fatalf("missing source result: %+v", source)
		}
	}
	if strings.Contains(w.Body.String(), federatedSearchToken) { t.Fatal("peer credential leaked into search response") }
}

func TestFederatedSearchFailsClosedAndSurfacesPartialSources(t *testing.T) {
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("uncredentialed", "http://127.0.0.1:28991")
	if err != nil { t.Fatal(err) }
	api := New(store, nil, nil)
	for _, target := range []string{
		"/api/v1/search?q=ok", "/api/v1/search?q=ok&target=invalid:docs",
		"/api/v1/search?q=ok&target="+host.ID+":../secret",
		"/api/v1/search?q=ok&target="+host.ID+":docs&offset=10001",
	} {
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusBadRequest { t.Fatalf("invalid request accepted: %s -> %d", target, w.Code) }
	}
	w := httptest.NewRecorder()
	api.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=ok&target="+host.ID+":docs", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "peer_token_missing") || !strings.Contains(w.Body.String(), `"complete":false`) {
		t.Fatalf("missing credential did not fail closed: %d %s", w.Code, w.Body.String())
	}
}

func TestFederatedSearchRejectsForgedPeerPathsAndPagination(t *testing.T) {
	for _, tc := range []struct { name, peerBody string }{
		{"cross-folder path", `{"share":"docs","path":"folder","query":"report","scope":"immediate_children","entries":[{"name":"report.txt","path":"other/report.txt","type":"file"}],"scanned":1}`},
		{"traversal basename", `{"share":"docs","path":"folder","query":"report","scope":"immediate_children","entries":[{"name":"../report.txt","path":"folder/report.txt","type":"file"}],"scanned":1}`},
		{"inconsistent page", `{"share":"docs","path":"folder","query":"report","scope":"immediate_children","entries":[],"scanned":3,"next_offset":42}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.peerBody))
			}))
			defer peer.Close()
			store, err := catalog.Open(t.TempDir())
			if err != nil { t.Fatal(err) }
			host, err := store.Upsert("untrusted-search-peer", peer.URL)
			if err != nil { t.Fatal(err) }
			request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=report&path=folder&target="+host.ID+":docs", nil)
			w := httptest.NewRecorder()
			New(store, map[string]string{host.ID:federatedSearchToken}, nil).ServeHTTP(w, request)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"error":"invalid_peer_response"`) || !strings.Contains(w.Body.String(), `"complete":false`) {
				t.Fatalf("forged search response accepted: status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
