package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolInputJSONNormalizesInvalid(t *testing.T) {
	// Valid JSON is preserved verbatim.
	if got := toolInputJSON(ToolCall{Raw: json.RawMessage(`{"file":"x.go"}`)}); string(got) != `{"file":"x.go"}` {
		t.Fatalf("valid Raw should pass through, got %s", got)
	}
	// Truncated/invalid JSON (e.g. a tool call cut off by max_tokens) becomes {}.
	if got := toolInputJSON(ToolCall{Raw: json.RawMessage(`{"content":"truncat`)}); string(got) != "{}" {
		t.Fatalf("invalid Raw should normalize to {}, got %s", got)
	}
	// Empty input → {}.
	if got := toolInputJSON(ToolCall{}); string(got) != "{}" {
		t.Fatalf("empty Raw should be {}, got %s", got)
	}
}

// A tool call whose Raw is invalid JSON (the live crash: a truncated write_file)
// must not blow up the next request marshal.
func TestRequestBodySurvivesTruncatedToolCall(t *testing.T) {
	msgs := []Message{
		{Role: "user", Text: "make a big file"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "t1", Name: "write_file", Raw: json.RawMessage(`{"content":"<huge truncat`)}}},
	}
	body, err := NewAnthropic("k", "claude-x", 1024).buildRequestBody(Request{Messages: msgs}, false)
	if err != nil {
		t.Fatalf("buildRequestBody must not fail on a truncated tool call: %v", err)
	}
	if !json.Valid(body) {
		t.Fatalf("request body must be valid JSON:\n%s", body)
	}
	if !strings.Contains(string(body), `"input":{}`) {
		t.Fatalf("truncated tool input should normalize to {} in the request:\n%s", body)
	}
}
