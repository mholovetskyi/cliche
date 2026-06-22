package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIBuildRequestToolRoundTrip(t *testing.T) {
	o := NewOpenAICompat("k", "openai/gpt-4o-mini", "http://x", 4096)
	req := Request{
		System: "be careful",
		Tools:  []ToolSpec{{Name: "read_file", Description: "read", Schema: map[string]any{"type": "object"}}},
		Messages: []Message{
			{Role: "user", Text: "go"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "read_file", Raw: json.RawMessage(`{"file":"x.go"}`)}}},
			{Role: "user", ToolResults: []ToolResult{{ID: "c1", Content: "package x"}}},
		},
	}
	body, err := o.buildRequestBody(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	msgs := out["messages"].([]any)
	// system, user, assistant(tool_calls), tool
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	asst := msgs[2].(map[string]any)
	tc := asst["tool_calls"].([]any)[0].(map[string]any)
	fn := tc["function"].(map[string]any)
	if fn["name"] != "read_file" || fn["arguments"].(string) != `{"file":"x.go"}` {
		t.Fatalf("assistant tool_call not encoded as OpenAI function-call: %v", fn)
	}
	toolMsg := msgs[3].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "c1" {
		t.Fatalf("tool result not encoded with tool_call_id: %v", toolMsg)
	}
}

func TestParseOpenAIResponseToolCall(t *testing.T) {
	raw := []byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"edit_file","arguments":"{\"file\":\"a.go\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7}}`)
	r, err := parseOpenAIResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Done {
		t.Fatal("a tool_calls turn must not be Done")
	}
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "edit_file" || r.ToolCalls[0].Args["file"] != "a.go" {
		t.Fatalf("tool call not parsed: %+v", r.ToolCalls)
	}
	if r.Usage.InputTokens != 12 || r.Usage.OutputTokens != 7 {
		t.Fatalf("usage mis-parsed: %+v", r.Usage)
	}
}

func TestOpenAICompleteAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"all done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	}))
	defer srv.Close()

	o := NewOpenAICompat("key", "m", srv.URL, 100)
	o.retryBase = time.Millisecond
	resp, err := o.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Text: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "all done" || !resp.Done {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
