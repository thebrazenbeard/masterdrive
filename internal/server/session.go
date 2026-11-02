package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"sync/atomic"
)

const sessionCookie = "masterdrive_session"

// NewSessionToken creates an ephemeral, unguessable local controller secret.
// It must not be persisted in the host registry or logged after startup.
func NewSessionToken() (string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret[:]), nil
}

// RequireSession protects the local browser API, distinct from peer tokens.
// A one-time fragment token is exchanged for an HttpOnly, same-site session
// cookie. Only the first successful exchange can redeem the fragment; later
// requests must use the issued cookie or restart the controller to get a new
// bootstrap link. Downloads can then use ordinary same-origin links safely.
func RequireSession(secret string, next http.Handler) (http.Handler, error) {
	if len(secret) < 40 {
		return nil, errors.New("controller session secret is too short")
	}
	const context = "masterdrive-local-cookie-v1:"
	data := sha256.Sum256([]byte(context + secret))
	cookieValue := base64.RawURLEncoding.EncodeToString(data[:])
	expectedHeader := sha256.Sum256([]byte(secret))
	expectedCookie := sha256.Sum256([]byte(cookieValue))
	var redeemed atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/api/v1/session" {
			if r.Method != http.MethodPost {
				fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "session activation requires POST")
				return
			}
			received := r.Header.Get("X-MasterDrive-Session")
			if len(received) != len(secret) {
				fail(w, http.StatusUnauthorized, "session_required", "valid local session required")
				return
			}
			actual := sha256.Sum256([]byte(received))
			if subtle.ConstantTimeCompare(expectedHeader[:], actual[:]) != 1 {
				fail(w, http.StatusUnauthorized, "session_required", "valid local session required")
				return
			}
			if !redeemed.CompareAndSwap(false, true) {
				fail(w, http.StatusUnauthorized, "session_required", "bootstrap link has already been redeemed")
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name: sessionCookie, Value: cookieValue, Path: "/api/v1/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || len(cookie.Value) != len(cookieValue) {
			fail(w, http.StatusUnauthorized, "session_required", "valid local session required")
			return
		}
		actual := sha256.Sum256([]byte(cookie.Value))
		if subtle.ConstantTimeCompare(expectedCookie[:], actual[:]) != 1 {
			fail(w, http.StatusUnauthorized, "session_required", "valid local session required")
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}
