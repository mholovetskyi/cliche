package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// SetAuth makes every request require this bearer token. It's used when Studio is
// bound to a non-loopback address — i.e. running inside a per-user cloud sandbox
// reached through the gateway, or deliberately exposed on a LAN. An empty token
// (the default) means no auth: the local-first, loopback-only mode.
//
// The agent runs arbitrary shell commands, so an exposed, UNauthenticated server
// is a remote-code-execution hole. `cliche serve` refuses to bind a non-loopback
// address without a token; this is the enforcement underneath that promise.
func (s *Server) SetAuth(token string) { s.token = token }

// authed reports whether the request carries the right token. Three carriers are
// accepted so every client works: an `Authorization: Bearer` header (fetch/XHR),
// a `?token=` query param (EventSource/SSE can't set headers), and a cookie (set
// after a successful query auth, so browser navigation + SSE then carry it).
func (s *Server) authed(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	want := []byte(s.token)
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		if eq(strings.TrimPrefix(h, "Bearer "), want) {
			return true
		}
	}
	if t := r.URL.Query().Get("token"); t != "" && eq(t, want) {
		return true
	}
	if c, err := r.Cookie("cliche_token"); err == nil && eq(c.Value, want) {
		return true
	}
	return false
}

func eq(got string, want []byte) bool {
	return subtle.ConstantTimeCompare([]byte(got), want) == 1
}

// withAuth wraps the mux: CORS for the cross-origin mobile/web client, a token
// gate on every request, and a cookie drop after a query-token auth so the
// browser's later navigations + EventSource (which can't set headers) stay authed.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !s.authed(r) {
			http.Error(w, "unauthorized — present the access token", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("token") != "" {
			http.SetCookie(w, &http.Cookie{
				Name: "cliche_token", Value: s.token, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteLaxMode,
			})
		}
		next.ServeHTTP(w, r)
	})
}
