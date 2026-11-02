package server

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionCookieCannotBeForgedFromFragment(t *testing.T) {
	secret, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	gate, err := RequireSession(secret, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}
	// This is the cookie that the previous implementation deterministically
	// derived from the fragment secret. It must have no authority now, even
	// after a legitimate activation has consumed the fragment.
	oldHash := sha256.Sum256([]byte("masterdrive-local-cookie-v1:" + secret))
	forged := &http.Cookie{
		Name:  sessionCookie,
		Value: base64.RawURLEncoding.EncodeToString(oldHash[:]),
	}
	getWithCookie := func(cookie *http.Cookie) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		gate.ServeHTTP(res, req)
		return res.Code
	}
	if got := getWithCookie(forged); got != http.StatusUnauthorized {
		t.Fatalf("forged pre-redemption cookie authorized: status=%d", got)
	}
	activate := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
	activate.Header.Set("X-MasterDrive-Session", secret)
	response := httptest.NewRecorder()
	gate.ServeHTTP(response, activate)
	if response.Code != http.StatusNoContent {
		t.Fatalf("legitimate activation failed: status=%d", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	if cookies[0].Value == forged.Value {
		t.Fatal("issued cookie was derived from fragment")
	}
	if got := getWithCookie(forged); got != http.StatusUnauthorized {
		t.Fatalf("forged post-redemption cookie authorized: status=%d", got)
	}
	if got := getWithCookie(cookies[0]); got != http.StatusNoContent {
		t.Fatalf("issued cookie rejected: status=%d", got)
	}
}

func TestSessionCookieIndependentBetweenServerInstances(t *testing.T) {
	secret, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	activate := func() (http.Handler, *http.Cookie) {
		t.Helper()
		gate, err := RequireSession(secret, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
		req.Header.Set("X-MasterDrive-Session", secret)
		res := httptest.NewRecorder()
		gate.ServeHTTP(res, req)
		if res.Code != http.StatusNoContent {
			t.Fatalf("activation failed: status=%d", res.Code)
		}
		cookies := res.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("expected one cookie, got %d", len(cookies))
		}
		return gate, cookies[0]
	}
	_, first := activate()
	secondGate, second := activate()
	if first.Value == second.Value {
		t.Fatal("independent instances issued identical cookies")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	req.AddCookie(first)
	w := httptest.NewRecorder()
	secondGate.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("cookie from other instance accepted: status=%d", w.Code)
	}
}
