package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A server with a token must reject every request that doesn't carry it, and
// accept the token via header, query, or cookie. This guards the remote/cloud
// mode where the agent (which runs shell commands) is network-reachable.
func TestAuthGate(t *testing.T) {
	srv := NewServer(nil, func() State { return State{} }, nil)
	srv.SetAuth("s3cret")
	h := srv.Handler()

	do := func(req *http.Request) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// No credential → 401.
	if code := do(httptest.NewRequest("GET", "/api/state", nil)); code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d, want 401", code)
	}
	// Wrong token → 401.
	bad := httptest.NewRequest("GET", "/api/state", nil)
	bad.Header.Set("Authorization", "Bearer nope")
	if code := do(bad); code != http.StatusUnauthorized {
		t.Fatalf("wrong token: got %d, want 401", code)
	}
	// Correct bearer header → not 401.
	hdr := httptest.NewRequest("GET", "/api/state", nil)
	hdr.Header.Set("Authorization", "Bearer s3cret")
	if code := do(hdr); code == http.StatusUnauthorized {
		t.Fatalf("bearer header: got 401, want pass")
	}
	// Correct ?token= query → not 401 (EventSource path).
	if code := do(httptest.NewRequest("GET", "/api/state?token=s3cret", nil)); code == http.StatusUnauthorized {
		t.Fatalf("query token: got 401, want pass")
	}
	// Correct cookie → not 401 (set after a prior query auth).
	ck := httptest.NewRequest("GET", "/api/state", nil)
	ck.AddCookie(&http.Cookie{Name: "cliche_token", Value: "s3cret"})
	if code := do(ck); code == http.StatusUnauthorized {
		t.Fatalf("cookie token: got 401, want pass")
	}
	// CORS preflight is answered without a token.
	if code := do(httptest.NewRequest("OPTIONS", "/api/state", nil)); code != http.StatusNoContent {
		t.Fatalf("preflight: got %d, want 204", code)
	}
	// A query-token auth drops a cookie so later navigations + SSE stay authed.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/state?token=s3cret", nil))
	if c := rec.Result().Cookies(); len(c) == 0 || c[0].Name != "cliche_token" {
		t.Fatalf("query auth should set the cliche_token cookie")
	}
}

// Without a token the server stays wide open (the local-first default).
func TestNoAuthByDefault(t *testing.T) {
	srv := NewServer(nil, func() State { return State{} }, nil)
	h := srv.Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/state", nil))
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("default mode should not require auth, got 401")
	}
}
