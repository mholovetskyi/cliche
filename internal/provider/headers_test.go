package provider

import (
	"net/http"
	"testing"
)

func TestOpenAICustomHeaders(t *testing.T) {
	o := NewOpenAICompat("k", "m", "https://gw/v1/chat/completions", 1024)
	o.SetHeaders(map[string]string{
		"x-gateway-key": "secret",
		"authorization": "Bearer override", // custom headers win over the default
	})
	req, _ := http.NewRequest(http.MethodPost, o.baseURL, nil)
	o.setHeaders(req, false)

	if got := req.Header.Get("x-gateway-key"); got != "secret" {
		t.Fatalf("custom header = %q, want secret", got)
	}
	if got := req.Header.Get("authorization"); got != "Bearer override" {
		t.Fatalf("custom authorization should override the default, got %q", got)
	}
	if req.Header.Get("content-type") != "application/json" {
		t.Fatal("standard headers should still be applied")
	}

	// With no custom headers, the default auth is intact.
	o2 := NewOpenAICompat("mykey", "m", "https://x", 1024)
	req2, _ := http.NewRequest(http.MethodPost, o2.baseURL, nil)
	o2.setHeaders(req2, true)
	if req2.Header.Get("authorization") != "Bearer mykey" {
		t.Fatalf("default authorization = %q", req2.Header.Get("authorization"))
	}
	if req2.Header.Get("accept") != "text/event-stream" {
		t.Fatal("stream requests should set the SSE accept header")
	}
}
