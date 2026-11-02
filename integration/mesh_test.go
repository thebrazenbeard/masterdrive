package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/agent"
	"github.com/thebrazenbeard/masterdrive/internal/catalog"
	"github.com/thebrazenbeard/masterdrive/internal/server"
)

func TestReadOnlyMeshThroughLocalGateway(t *testing.T) {
	shareDir := t.TempDir()
	const fileData = "read-only mesh fixture\n"
	if err := os.WriteFile(filepath.Join(shareDir, "hello.txt"), []byte(fileData), 0600); err != nil {
		t.Fatal(err)
	}
	const token = "integration-only-32-char-secret-0123456789"
	agentHandler, err := agent.NewHandler([]agent.Export{{ID: "docs", Name: "Documents", Path: shareDir}}, token)
	if err != nil { t.Fatal(err) }
	defer agentHandler.Close()
	peer := httptest.NewServer(agentHandler)
	defer peer.Close()
	store, err := catalog.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	host, err := store.Upsert("test-pc", peer.URL)
	if err != nil { t.Fatal(err) }
	controller := httptest.NewServer(server.New(store, map[string]string{host.ID: token}, nil))
	defer controller.Close()
	base := controller.URL + "/api/v1/hosts/" + host.ID + "/shares"

	for _, item := range []struct { suffix, includes string }{
		{"", `"docs"`},
		{"/docs/entries", `"hello.txt"`},
	} {
		response, err := http.Get(base + item.suffix)
		if err != nil { t.Fatal(err) }
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil { t.Fatal(err) }
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body), item.includes) {
			t.Fatalf("GET %s status=%d body=%s", item.suffix, response.StatusCode, body)
		}
	}

	response, err := http.Get(base + "/docs/download?path=hello.txt")
	if err != nil { t.Fatal(err) }
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil { t.Fatal(err) }
	if response.StatusCode != http.StatusOK || string(body) != fileData {
		t.Fatalf("download status=%d body=%s", response.StatusCode, body)
	}

	// Disallowed traversal can never return the exported directory's parent.
	escape, err := http.Get(base + "/docs/download?path=..%2Fhello.txt")
	if err != nil { t.Fatal(err) }
	_, _ = io.Copy(io.Discard, escape.Body)
	escape.Body.Close()
	if escape.StatusCode == http.StatusOK { t.Fatal("parent traversal unexpectedly accepted") }

	if err := store.Remove(host.ID); err != nil { t.Fatal(err) }
	removed, err := http.Get(base)
	if err != nil { t.Fatal(err) }
	removed.Body.Close()
	if removed.StatusCode != http.StatusNotFound { t.Fatalf("removed host status=%d", removed.StatusCode) }
}
