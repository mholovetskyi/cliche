package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEgressAllowed(t *testing.T) {
	e := ParseEgress([]string{"api.github.com", "*.example.com", " EXAMPLE.ORG "})
	cases := []struct {
		host string
		want bool
	}{
		{"api.github.com", true},
		{"github.com", false},        // exact rule doesn't cover bare domain
		{"docs.example.com", true},   // wildcard subdomain
		{"a.b.example.com", true},    // multi-level subdomain
		{"example.com", false},       // *.example.com excludes the bare domain
		{"example.org", true},        // case/space-normalized exact
		{"evil.com", false},          // not listed
		{"api.github.com:443", true}, // :port is stripped
	}
	for _, c := range cases {
		if got := e.Allowed(c.host); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestEgressUnconfiguredAllowsAll(t *testing.T) {
	var e Egress
	if e.Configured() {
		t.Fatal("empty egress should be unconfigured")
	}
	if !e.Allowed("anything.example") {
		t.Fatal("an unconfigured allowlist must allow everything")
	}
}

func TestWebFetchBlockedByEgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret"))
	}))
	defer srv.Close()

	// Allowlist that does NOT include the test server's host; even with --yolo,
	// the egress boundary must block the fetch before any network call.
	e := OSExecutor{
		Policy: Policy{Yolo: true, AllowWeb: true},
		Egress: ParseEgress([]string{"api.github.com"}),
	}
	r := e.Execute(context.Background(), "web_fetch", map[string]string{"url": srv.URL})
	if r.Success {
		t.Fatal("egress allowlist must block a non-listed host even under --yolo")
	}
}
