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
	body, err := a.buildRequestBody(req)
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

func TestBuildRequestBodySkipsEmptyContent(t *testing.T) {
	a := NewAnthropic("key", "m", 4096)
	body, err := a.buildRequestBody(Request{Messages: []Message{
		{Role: "assistant"}, // no text, no tool calls -> skipped
		{Role: "user", Text: "hello"},
	}})
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
