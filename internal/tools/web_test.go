package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
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
