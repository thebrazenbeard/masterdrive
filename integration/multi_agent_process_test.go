package integration_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thebrazenbeard/masterdrive/internal/agent"
	"github.com/thebrazenbeard/masterdrive/internal/catalog"
	"github.com/thebrazenbeard/masterdrive/internal/server"
)

// TestMeshSubprocessAgent is run by this test binary only when explicitly
// selected by the parent test through an environment variable. Each source
// agent runs in a separate OS process with its own token, pinned export root,
// and ephemeral loopback-only listener. No real user directories are used.
func TestMeshSubprocessAgent(t *testing.T) {
	if os.Getenv("MASTERDRIVE_TEST_CHILD_ROLE") != "agent" {
		return
	}
	exportDir := os.Getenv("MASTERDRIVE_TEST_CHILD_EXPORT")
	secret := os.Getenv("MASTERDRIVE_TEST_CHILD_TOKEN")
	id := os.Getenv("MASTERDRIVE_TEST_CHILD_SHARE")
	h, err := agent.NewHandler([]agent.Export{{ID: id, Name: "Child " + id, Path: exportDir}}, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	// This is the only stdout protocol message. Do not expose the token or
	// local filesystem path to the parent process logs.
	fmt.Fprintf(os.Stdout, "READY %s\n", listener.Addr().String())
	if err := http.Serve(listener, h); err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatal(err)
	}
}

type isolatedAgentProcess struct {
	url  string
	stop func()
}

func startIsolatedAgentProcess(t *testing.T, share, dir, token string) isolatedAgentProcess {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^TestMeshSubprocessAgent$")
	child.Env = append(os.Environ(),
		"MASTERDRIVE_TEST_CHILD_ROLE=agent",
		"MASTERDRIVE_TEST_CHILD_EXPORT="+dir,
		"MASTERDRIVE_TEST_CHILD_TOKEN="+token,
		"MASTERDRIVE_TEST_CHILD_SHARE="+share)
	pipe, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostic bytes.Buffer
	child.Stderr = &diagnostic
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_ = child.Process.Kill()
		_ = child.Wait()
	}
	t.Cleanup(stop)
	type startupResult struct {
		line string
		err  error
	}
	ready := make(chan startupResult, 1)
	go func() {
		line, err := bufio.NewReader(pipe).ReadString('\n')
		ready <- startupResult{line: strings.TrimSpace(line), err: err}
	}()
	select {
	case result := <-ready:
		if result.err != nil || !strings.HasPrefix(result.line, "READY 127.0.0.1:") {
			stop()
			t.Fatalf("agent subprocess never became ready: line=%q err=%v diagnostic=%q", result.line, result.err, diagnostic.String())
		}
		return isolatedAgentProcess{url: "http://" + strings.TrimPrefix(result.line, "READY "), stop: stop}
	case <-time.After(10 * time.Second):
		stop()
		t.Fatalf("agent subprocess timed out before binding loopback: %q", diagnostic.String())
		return isolatedAgentProcess{}
	}
}

type meshHTTPResponse struct {
	status int
	body   []byte
	header http.Header
}

func meshRequest(t *testing.T, client *http.Client, method, target, token string, cookie *http.Cookie) meshHTTPResponse {
	t.Helper()
	request, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return meshHTTPResponse{status: response.StatusCode, body: body, header: response.Header.Clone()}
}

func TestTwoIndependentAgentsThroughOneController(t *testing.T) {
	const tokenA = "isolated-child-token-a-0123456789abcdef"
	const tokenB = "isolated-child-token-b-fedcba9876543210"
	const contentA = "Only approved source A can read this file.\n"
	const contentB = "Only approved source B can read that file.\n"
	exportA := t.TempDir()
	exportB := t.TempDir()
	if err := os.WriteFile(filepath.Join(exportA, "alpha.txt"), []byte(contentA), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exportB, "beta.txt"), []byte(contentB), 0600); err != nil {
		t.Fatal(err)
	}
	a := startIsolatedAgentProcess(t, "share-a", exportA, tokenA)
	b := startIsolatedAgentProcess(t, "share-b", exportB, tokenB)
	client := &http.Client{Timeout: 10 * time.Second}

	// Distinct source credentials are enforced even though both processes
	// share the same local machine and private loopback transport.
	for _, tc := range []struct {
		name, url, token string
		want int
	}{
		{"a missing token", a.url, "", http.StatusUnauthorized},
		{"a rejects b token", a.url, tokenB, http.StatusUnauthorized},
		{"b rejects a token", b.url, tokenA, http.StatusUnauthorized},
		{"a accepts its token", a.url, tokenA, http.StatusOK},
		{"b accepts its token", b.url, tokenB, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := meshRequest(t, client, http.MethodGet, tc.url+"/v1/shares", tc.token, nil)
			if reply.status != tc.want {
				t.Fatalf("HTTP %d, want %d; body=%s", reply.status, tc.want, reply.body)
			}
		})
	}
	wrongShare := meshRequest(t, client, http.MethodGet,
		a.url+"/v1/list?share=share-b", tokenA, nil)
	if wrongShare.status != http.StatusNotFound {
		t.Fatalf("source A exposed source B's share namespace: HTTP %d", wrongShare.status)
	}

	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hostA, err := store.Upsert("first isolated peer", a.url)
	if err != nil {
		t.Fatal(err)
	}
	hostB, err := store.Upsert("second isolated peer", b.url)
	if err != nil {
		t.Fatal(err)
	}
	sessionToken, err := server.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	gateway := server.New(store, map[string]string{hostA.ID: tokenA, hostB.ID: tokenB}, nil)
	protected, err := server.RequireSession(sessionToken, gateway)
	if err != nil {
		t.Fatal(err)
	}
	controller := httptest.NewServer(server.RequireLoopbackHost(protected))
	defer controller.Close()

	unauthenticated := meshRequest(t, client, http.MethodGet, controller.URL+"/api/v1/hosts", "", nil)
	if unauthenticated.status != http.StatusUnauthorized {
		t.Fatalf("controller exposed catalog without browser session: %d", unauthenticated.status)
	}
	activate, err := http.NewRequest(http.MethodPost, controller.URL+"/api/v1/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	activate.Header.Set("X-MasterDrive-Session", sessionToken)
	sessionResponse, err := client.Do(activate)
	if err != nil {
		t.Fatal(err)
	}
	sessionResponse.Body.Close()
	cookies := sessionResponse.Cookies()
	if sessionResponse.StatusCode != http.StatusNoContent || len(cookies) != 1 ||
		!cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session bootstrap not constrained: status=%d cookies=%v", sessionResponse.StatusCode, cookies)
	}
	cookie := cookies[0]

	inventory := meshRequest(t, client, http.MethodGet, controller.URL+"/api/v1/hosts", "", cookie)
	if inventory.status != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", inventory.status, inventory.body)
	}
	if bytes.Contains(inventory.body, []byte(tokenA)) || bytes.Contains(inventory.body, []byte(tokenB)) {
		t.Fatal("controller exposed source credentials to browser inventory")
	}
	var hosts struct {
		Hosts []struct{ ID string `json:"id"` } `json:"hosts"`
	}
	if err := json.Unmarshal(inventory.body, &hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts.Hosts) != 2 || hosts.Hosts[0].ID == hosts.Hosts[1].ID {
		t.Fatalf("expected two distinct configured sources, got %+v", hosts.Hosts)
	}

	// Exercise each source through the real HTTP controller-to-agent path:
	// both shared metadata and bytes must stay on their owning peer.
	for _, tc := range []struct {
		hostID, shareID, name, content string
	}{
		{hostA.ID, "share-a", "alpha.txt", contentA},
		{hostB.ID, "share-b", "beta.txt", contentB},
	} {
		t.Run(tc.shareID+" gateway", func(t *testing.T) {
			base := controller.URL + "/api/v1/hosts/" + tc.hostID + "/shares"
			shares := meshRequest(t, client, http.MethodGet, base, "", cookie)
			if shares.status != http.StatusOK || !bytes.Contains(shares.body, []byte(tc.shareID)) {
				t.Fatalf("share unavailable: %d %s", shares.status, shares.body)
			}
			entries := meshRequest(t, client, http.MethodGet, base+"/"+tc.shareID+"/entries", "", cookie)
			if entries.status != http.StatusOK || !bytes.Contains(entries.body, []byte(tc.name)) {
				t.Fatalf("folder listing unavailable: %d %s", entries.status, entries.body)
			}
			file := meshRequest(t, client, http.MethodGet, base+"/"+tc.shareID+"/download?path="+tc.name, "", cookie)
			if file.status != http.StatusOK || string(file.body) != tc.content {
				t.Fatalf("wrong source bytes: status=%d body=%q", file.status, file.body)
			}
		})
	}

	// A failed source cannot make an unrelated source appear offline, and a
	// removed catalog entry does NOT revoke a token at the still-live source.
	b.stop()
	bGone := meshRequest(t, client, http.MethodGet,
		controller.URL+"/api/v1/hosts/"+hostB.ID+"/shares", "", cookie)
	if bGone.status != http.StatusBadGateway {
		t.Fatalf("offline source did not fail with gateway error: %d %s", bGone.status, bGone.body)
	}
	aStillOnline := meshRequest(t, client, http.MethodGet,
		controller.URL+"/api/v1/hosts/"+hostA.ID+"/shares", "", cookie)
	if aStillOnline.status != http.StatusOK {
		t.Fatalf("other live source affected by offline peer: %d", aStillOnline.status)
	}
	if err := store.Remove(hostA.ID); err != nil {
		t.Fatal(err)
	}
	forgotten := meshRequest(t, client, http.MethodGet,
		controller.URL+"/api/v1/hosts/"+hostA.ID+"/shares", "", cookie)
	if forgotten.status != http.StatusNotFound {
		t.Fatalf("removed registration remained authorized at controller: %d", forgotten.status)
	}
	direct := meshRequest(t, client, http.MethodGet, a.url+"/v1/shares", tokenA, nil)
	if direct.status != http.StatusOK {
		t.Fatalf("catalog removal was confused with source credential revocation: %d", direct.status)
	}
}
