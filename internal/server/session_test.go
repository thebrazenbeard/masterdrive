package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestControllerSessionRequiredAndCookieActivation(t *testing.T) {
	secret, err := NewSessionToken()
	if err != nil { t.Fatal(err) }
	gate, err := RequireSession(secret, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil { t.Fatal(err) }
	protected := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	before := httptest.NewRecorder()
	gate.ServeHTTP(before, protected)
	if before.Code != http.StatusUnauthorized { t.Fatalf("ungated request status=%d", before.Code) }

	bad := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
	bad.Header.Set("X-MasterDrive-Session", strings.Repeat("x", len(secret)))
	badResponse := httptest.NewRecorder()
	gate.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusUnauthorized || len(badResponse.Result().Cookies()) != 0 {
		t.Fatal("invalid session activated")
	}

	activate := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
	activate.Header.Set("X-MasterDrive-Session", secret)
	activation := httptest.NewRecorder()
	gate.ServeHTTP(activation, activate)
	if activation.Code != http.StatusNoContent { t.Fatalf("activate status=%d", activation.Code) }
	cookies := activation.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("bad cookie properties: %+v", cookies)
	}
	if cookies[0].Value == secret { t.Fatal("browser cookie contains activation secret") }

	authorized := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	authorized.AddCookie(cookies[0])
	good := httptest.NewRecorder()
	gate.ServeHTTP(good, authorized)
	if good.Code != http.StatusNoContent { t.Fatalf("authenticated API status=%d", good.Code) }

	post := httptest.NewRequest(http.MethodPost, "/api/v1/session", nil)
	post.AddCookie(cookies[0])
	postResponse := httptest.NewRecorder()
	gate.ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusUnauthorized { t.Fatal("cookie reactivated session without secret") }
}

func TestControllerSessionRejectsWeakSecret(t *testing.T) {
	if _, err := RequireSession("short", http.NotFoundHandler()); err == nil {
		t.Fatal("accepted weak session secret")
	}
}
