package provider

import (
	"encoding/json"
	"testing"
)

func TestBuildRequestBody(t *testing.T) {
	a := NewAnthropic("key", "claude-sonnet-4-6", 4096)
	req := Request{
		System: "be careful",
		Model:  "claude-sonnet-4-6",
		Tools: []ToolSpec{{
			Name:        "read_file",
			Description: "read a file",
			Schema:      map[string]any{"type": "object"},
		}},
		Messages: []Message{
			{Role: "user", Text: "do it"},
			{Role: "assistant", Text: "reading", ToolCalls: []ToolCall{{ID: "t1", Name: "read_file", Args: map[string]string{"file": "x.go"}}}},
			{Role: "user", ToolResults: []ToolResult{{ID: "t1", Content: "package x", IsError: false}}},
		},
		MaxOutputTokens: 1000,
	}
	body, err := a.buildRequestBody(req, false)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out["max_tokens"].(float64) != 1000 {
		t.Fatalf("max_tokens should be clamped to remaining budget, got %v", out["max_tokens"])
	}
	tools := out["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	msgs := out["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// The assistant message must carry a tool_use block.
	asst := msgs[1].(map[string]any)
	blocks := asst["content"].([]any)
	found := false
	for _, b := range blocks {
		if b.(map[string]any)["type"] == "tool_use" {
			found = true
		}
	}
	if !found {
		t.Fatal("assistant message missing tool_use block")
	}
	// The user follow-up must carry a tool_result block.
	usr := msgs[2].(map[string]any)
	ub := usr["content"].([]any)[0].(map[string]any)
	if ub["type"] != "tool_result" || ub["tool_use_id"] != "t1" {
		t.Fatalf("expected tool_result for t1, got %v", ub)
	}
}

func TestBuildRequestBodyPromptCaching(t *testing.T) {
	a := NewAnthropic("key", "claude-sonnet-4-6", 4096)
	body, err := a.buildRequestBody(Request{
		System:   "be careful",
		Messages: []Message{{Role: "user", Text: "hello"}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	// System is an array of blocks with a cache_control breakpoint (caches
	// tools+system).
	sys, ok := out["system"].([]any)
	if !ok || len(sys) != 1 {
		t.Fatalf("system should be a one-block array, got %v", out["system"])
	}
	if sys[0].(map[string]any)["cache_control"] == nil {
		t.Fatal("system block should carry cache_control")
	}
	// The last message's last block carries a cache_control breakpoint (caches
	// the conversation prefix).
	msgs := out["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)["content"].([]any)
	if last[len(last)-1].(map[string]any)["cache_control"] == nil {
		t.Fatal("last message block should carry cache_control")
	}
}

func TestParseResponseCacheTokens(t *testing.T) {
	raw := []byte(`{"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":6656,"cache_creation_input_tokens":1024}}`)
	r, err := parseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Usage.CacheReadTokens != 6656 || r.Usage.CacheWriteTokens != 1024 {
		t.Fatalf("cache usage not parsed: %+v", r.Usage)
	}
}

func TestBuildRequestBodySkipsEmptyContent(t *testing.T) {
	a := NewAnthropic("key", "m", 4096)
	body, err := a.buildRequestBody(Request{Messages: []Message{
		{Role: "assistant"}, // no text, no tool calls -> skipped
		{Role: "user", Text: "hello"},
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if n := len(out["messages"].([]any)); n != 1 {
		t.Fatalf("empty assistant message should be skipped, got %d messages", n)
	}
}

func TestParseResponseToolUse(t *testing.T) {
	raw := []byte(`{
		"content": [
			{"type":"text","text":"let me read it"},
			{"type":"tool_use","id":"abc","name":"read_file","input":{"file":"main.go"}}
		],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":120,"output_tokens":45}
	}`)
	r, err := parseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Done {
		t.Fatal("tool_use stop_reason should not be Done")
	}
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "read_file" {
		t.Fatalf("expected one read_file call, got %+v", r.ToolCalls)
	}
	if r.ToolCalls[0].Args["file"] != "main.go" {
		t.Fatalf("arg not decoded: %+v", r.ToolCalls[0].Args)
	}
	if r.ToolCalls[0].Signature == "" {
		t.Fatal("signature should be populated for the governor")
	}
	if r.Usage.InputTokens != 120 || r.Usage.OutputTokens != 45 {
		t.Fatalf("usage mis-parsed: %+v", r.Usage)
	}
}

func TestParseResponseEndTurnIsDone(t *testing.T) {
	raw := []byte(`{"content":[{"type":"text","text":"all done"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":3}}`)
	r, err := parseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Done || len(r.ToolCalls) != 0 || r.Text != "all done" {
		t.Fatalf("expected a done text response, got %+v", r)
	}
}

func TestParseResponseError(t *testing.T) {
	raw := []byte(`{"error":{"type":"overloaded_error","message":"slow down"}}`)
	if _, err := parseResponse(raw); err == nil {
		t.Fatal("expected an error for an API error body")
	}
}
