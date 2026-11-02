package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebrazenbeard/masterdrive/internal/agent"
)

const (
	agentGrantDocsToken   = "docs-only-grant-secret-0123456789abcdef"
	agentGrantPhotosToken = "photos-only-grant-secret-fedcba9876543210"
)

func agentConfigLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func cliAgentRequest(h http.Handler, target, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if token != "" { r.Header.Set("Authorization", "Bearer "+token) }
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAgentGrantsJSONRejectsAmbiguityWithoutLeakingSecrets(t *testing.T) {
	const sensitive = "never-show-this-secret-in-errors-0123456789"
	invalid := []string{
		"", "null", "[]", "{}", "[", `[{"bearer_token":"`+sensitive+`","share_ids":["docs"],"unknown":"`+sensitive+`"}]`,
		`[{"bearer_token":"`+sensitive+`","share_ids":["docs"]}] {"other":"data"}`,
	}
	for _, raw := range invalid {
		_, err := parseAgentGrantsJSON(raw)
		if err == nil { t.Errorf("accepted invalid grants JSON (length=%d)", len(raw)); continue }
		if strings.Contains(err.Error(), sensitive) { t.Fatal("validation error disclosed a bearer secret") }
	}
	if _, err := parseAgentGrantsJSON(strings.Repeat("x", maxAgentGrantsJSONBytes+1)); err == nil {
		t.Fatal("oversized credential configuration was accepted")
	}
	valid := `[{"bearer_token":"`+agentGrantDocsToken+`","share_ids":["docs"]}]`
	grants, err := parseAgentGrantsJSON(valid)
	if err != nil { t.Fatal(err) }
	if len(grants) != 1 || grants[0].BearerToken != agentGrantDocsToken || len(grants[0].ShareIDs) != 1 || grants[0].ShareIDs[0] != "docs" {
		t.Fatalf("credential configuration changed unexpectedly")
	}
}

func TestAgentCLISelectsOnlyOneCredentialMode(t *testing.T) {
	exports := []agent.Export{{ID: "docs", Name: "Documents", Path: t.TempDir()}}
	for _, tc := range []struct {
		name string
		env map[string]string
	}{
		{"unset", nil},
		{"both", map[string]string{"MASTERDRIVE_AGENT_TOKEN":agentGrantDocsToken, "MASTERDRIVE_AGENT_GRANTS_JSON":"[]"}},
		{"empty-scoped", map[string]string{"MASTERDRIVE_AGENT_GRANTS_JSON":""}},
		{"empty-legacy", map[string]string{"MASTERDRIVE_AGENT_TOKEN":""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := configuredAgentHandler(exports, agentConfigLookup(tc.env))
			if h != nil { _ = h.Close(); t.Fatal("invalid credential mode was accepted") }
			if err == nil { t.Fatal("expected credential configuration failure") }
		})
	}
}

func TestAgentCLIRejectsInvalidShareGrantsWithoutEchoingCredentials(t *testing.T) {
	exports := []agent.Export{{ID:"docs", Name:"Documents", Path:t.TempDir()}}
	for _, raw := range []string{
		`[{"bearer_token":"`+agentGrantDocsToken+`","share_ids":["missing"]}]`,
		`[{"bearer_token":"`+agentGrantDocsToken+`","share_ids":["docs","docs"]}]`,
		`[{"bearer_token":"`+agentGrantDocsToken+`","share_ids":["docs"]},{"bearer_token":"`+agentGrantDocsToken+`","share_ids":["docs"]}]`,
	} {
		h, err := configuredAgentHandler(exports, agentConfigLookup(map[string]string{"MASTERDRIVE_AGENT_GRANTS_JSON":raw}))
		if h != nil { _ = h.Close(); t.Fatal("invalid grant was accepted") }
		if err == nil || strings.Contains(err.Error(), agentGrantDocsToken) {
			t.Fatalf("invalid grant was accepted or credential leaked: %v", err)
		}
	}
}

func TestAgentCLIScopedGrantsAndLegacyCompatibility(t *testing.T) {
	docs := t.TempDir()
	photos := t.TempDir()
	if err := os.WriteFile(filepath.Join(docs, "report.txt"), []byte("approved documents"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(photos, "photo.txt"), []byte("private photo"), 0600); err != nil { t.Fatal(err) }
	exports := []agent.Export{{ID:"docs", Name:"Documents", Path:docs}, {ID:"photos", Name:"Photos", Path:photos}}
	scopedJSON := `[{"bearer_token":"`+agentGrantDocsToken+`","share_ids":["docs"]},`+
		`{"bearer_token":"`+agentGrantPhotosToken+`","share_ids":["photos"]}]`
	h, err := configuredAgentHandler(exports, agentConfigLookup(map[string]string{"MASTERDRIVE_AGENT_GRANTS_JSON":scopedJSON}))
	if err != nil { t.Fatal(err) }
	defer h.Close()
	for _, tc := range []struct {
		token, visible, hidden string
	}{
		{agentGrantDocsToken, "docs", "photos"},
		{agentGrantPhotosToken, "photos", "docs"},
	} {
		w := cliAgentRequest(h, "/v1/shares", tc.token)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"`+tc.visible+`"`) ||
			strings.Contains(w.Body.String(), `"id":"`+tc.hidden+`"`) {
			t.Fatalf("scoped inventory incorrect: HTTP %d %s", w.Code, w.Body.String())
		}
		denied := cliAgentRequest(h, "/v1/download?share="+tc.hidden+"&path=report.txt", tc.token)
		if denied.Code != http.StatusNotFound || strings.Contains(denied.Body.String(), "private photo") {
			t.Fatalf("ungranted source was disclosed: HTTP %d %s", denied.Code, denied.Body.String())
		}
	}
	if denied := cliAgentRequest(h, "/v1/shares", "not-the-secret"); denied.Code != http.StatusUnauthorized {
		t.Fatalf("incorrect source token accepted: HTTP %d", denied.Code)
	}
	legacy, err := configuredAgentHandler(exports, agentConfigLookup(map[string]string{"MASTERDRIVE_AGENT_TOKEN":agentGrantDocsToken}))
	if err != nil { t.Fatal(err) }
	defer legacy.Close()
	inventory := cliAgentRequest(legacy, "/v1/shares", agentGrantDocsToken)
	if inventory.Code != http.StatusOK || !bytes.Contains(inventory.Body.Bytes(), []byte(`"id":"docs"`)) ||
		!bytes.Contains(inventory.Body.Bytes(), []byte(`"id":"photos"`)) {
		t.Fatalf("legacy all-share behavior regressed: HTTP %d %s", inventory.Code, inventory.Body.String())
	}
}

func TestAgentCLIAmbiguousCredentialsFailBeforeReadinessMessage(t *testing.T) {
	t.Setenv("MASTERDRIVE_AGENT_TOKEN", "")
	t.Setenv("MASTERDRIVE_AGENT_GRANTS_JSON", `[{"bearer_token":"sensitive"}]`)
	var stdout bytes.Buffer
	err := runAgent([]string{"-export", "docs=" + t.TempDir()}, &stdout)
	if err == nil || stdout.Len() != 0 {
		t.Fatalf("invalid startup announced readiness: err=%v stdout=%q", err, stdout.String())
	}
}
