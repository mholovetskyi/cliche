package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWebFetchHTMLToText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		w.Write([]byte(`<html><head><style>.x{}</style><script>bad()</script></head><body><h1>Title</h1><p>Hello &amp; welcome</p></body></html>`))
	}))
	defer srv.Close()

	e := OSExecutor{Policy: Policy{AllowWeb: true}}
	r := e.Execute(context.Background(), "web_fetch", map[string]string{"url": srv.URL})
	if !r.Success {
		t.Fatalf("fetch should succeed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Title") || !strings.Contains(r.Output, "Hello & welcome") {
		t.Fatalf("HTML should be reduced to readable text: %q", r.Output)
	}
	if strings.Contains(r.Output, "bad()") || strings.Contains(r.Output, "<h1>") {
		t.Fatalf("scripts/tags should be stripped: %q", r.Output)
	}
}

func TestWebFetchRedirectCannotEscapeAllowlist(t *testing.T) {
	// An allowlisted host (127.0.0.1) that 302-redirects to a DIFFERENT host not
	// on the allowlist. CheckRedirect must block the hop before it is contacted,
	// so the redirect target ("internal.invalid") is never actually reached — the
	// test is hermetic regardless.
	reached := false
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/next" {
			reached = true // would only fire if the allowlist were bypassed
			w.Write([]byte("INTERNAL-SECRET"))
			return
		}
		http.Redirect(w, r, "http://internal.invalid/secret", http.StatusFound)
	}))
	defer redirector.Close()

	// Allowlist ONLY the redirector's host (127.0.0.1); the redirect to
	// internal.invalid must be refused by the egress re-check on the hop.
	e := OSExecutor{
		Policy: Policy{Yolo: true, AllowWeb: true},
		Egress: ParseEgress([]string{mustHost(t, redirector.URL)}),
	}
	r := e.Execute(context.Background(), "web_fetch", map[string]string{"url": redirector.URL})
	if r.Success {
		t.Fatalf("a redirect to a non-allowlisted host must be blocked, got: %q", r.Output)
	}
	if !strings.Contains(r.Output, "disallowed host") {
		t.Fatalf("expected an egress redirect-block error, got: %q", r.Output)
	}
	if reached {
		t.Fatal("the redirect target was contacted — egress allowlist bypassed")
	}
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname()
}

func TestWebFetchGating(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// No AllowWeb and no approver → denied (never hits the network).
	e := OSExecutor{}
	if r := e.Execute(context.Background(), "web_fetch", map[string]string{"url": srv.URL}); r.Success {
		t.Fatal("web_fetch must be denied without --allow-web / approval")
	}
	// A non-http scheme is rejected before any request.
	if r := e.Execute(context.Background(), "web_fetch", map[string]string{"url": "file:///etc/passwd"}); r.Success {
		t.Fatal("non-http(s) url must be rejected")
	}
}
