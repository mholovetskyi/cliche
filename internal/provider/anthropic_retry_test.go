package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryOn429ThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	a := NewAnthropic("key", "m", 100)
	a.baseURL = srv.URL
	a.retryBase = time.Millisecond

	resp, err := a.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("unexpected text %q", resp.Text)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls (1 retry), got %d", got)
	}
}

func TestNonRetryableStatusStops(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key"}}`))
	}))
	defer srv.Close()

	a := NewAnthropic("key", "m", 100)
	a.baseURL = srv.URL
	a.retryBase = time.Millisecond
	if _, err := a.Complete(context.Background(), Request{}); err == nil {
		t.Fatal("401 should not be retried and should error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("401 must not retry, got %d calls", got)
	}
}

func TestRawToolInputRoundTrips(t *testing.T) {
	a := NewAnthropic("k", "m", 100)
	req := Request{Messages: []Message{
		{Role: "assistant", ToolCalls: []ToolCall{{
			ID: "t1", Name: "calc",
			Raw: json.RawMessage(`{"n":5,"flag":true}`),
		}}},
	}}
	body, err := a.buildRequestBody(req, false)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	block := out["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	input := block["input"].(map[string]any)
	if input["n"].(float64) != 5 {
		t.Fatalf("number arg should round-trip as a number, got %T %v", input["n"], input["n"])
	}
	if input["flag"].(bool) != true {
		t.Fatalf("bool arg should round-trip as a bool, got %v", input["flag"])
	}
}

func TestParseRetryAfterAndRetryable(t *testing.T) {
	if d := parseRetryAfter("3"); d != 3*time.Second {
		t.Fatalf("parseRetryAfter(3) = %v", d)
	}
	if d := parseRetryAfter(""); d != 0 {
		t.Fatalf("empty Retry-After should be 0, got %v", d)
	}
	for _, s := range []int{429, 500, 502, 503, 529, 408} {
		if !isRetryable(s) {
			t.Fatalf("%d should be retryable", s)
		}
	}
	for _, s := range []int{400, 401, 403, 404, 422} {
		if isRetryable(s) {
			t.Fatalf("%d should NOT be retryable", s)
		}
	}
}
