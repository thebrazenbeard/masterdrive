package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A malformed Authorization header must fail before any endpoint routing,
// including unsupported methods and unknown paths.
func TestMalformedAuthorizationNeverReachesAgentRoutes(t *testing.T) {
	root := t.TempDir()
	h := testHandler(t, []Export{{ID: "private", Name: "Secret Share", Path: root}})
	cases := []struct {
		name    string
		headers []string
	}{
		{"absent", nil},
		{"empty", []string{""}},
		{"wrong scheme", []string{"Basic " + testToken}},
		{"bearer without space", []string{"Bearer" + testToken}},
		{"bearer double space", []string{"Bearer  " + testToken}},
		{"bearer trailing space", []string{"Bearer " + testToken + " "}},
		{"duplicate valid", []string{"Bearer " + testToken, "Bearer " + testToken}},
		{"valid plus invalid", []string{"Bearer " + testToken, "Basic " + testToken}},
		{"wrong credential", []string{"Bearer " + strings.Repeat("x", 40)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, route := range []string{"/v1/shares", "/v1/list?share=private", "/v1/search?share=private&q=secret", "/v1/download?share=private&path=secret.txt", "/not-found"} {
				for _, method := range []string{http.MethodGet, http.MethodPost} {
					req := httptest.NewRequest(method, route, nil)
					for _, v := range tc.headers {
						req.Header.Add("Authorization", v)
					}
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, req)
					if rec.Code != http.StatusUnauthorized {
						t.Errorf("%s %s: status=%d", method, route, rec.Code)
					}
					if rec.Header().Get("Cache-Control") != "no-store" {
						t.Errorf("%s %s: missing no-store", method, route)
					}
					if strings.Contains(rec.Body.String(), "Secret Share") || strings.Contains(rec.Body.String(), root) {
						t.Errorf("%s %s: metadata leak", method, route)
					}
				}
			}
		})
	}
}
