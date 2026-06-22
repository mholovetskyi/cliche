package provider

import (
	"context"
	"strings"
	"testing"
)

func TestParseAnthropicStream(t *testing.T) {
	sse := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":5,"cache_creation_input_tokens":2}}}`,
		``,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"read_file"}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"file\":"}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"x.go\"}"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
		`data: {"type":"message_stop"}`,
	}, "\n")

	var deltas []string
	resp, emitted, _, _, err := parseAnthropicStream(context.Background(), strings.NewReader(sse), func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Hello" {
		t.Fatalf("text = %q, want Hello", resp.Text)
	}
	if !emitted || strings.Join(deltas, "|") != "Hel|lo" {
		t.Fatalf("deltas = %v emitted=%v", deltas, emitted)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "read_file" || resp.ToolCalls[0].Args["file"] != "x.go" {
		t.Fatalf("tool call mis-parsed: %+v", resp.ToolCalls)
	}
	if resp.Done {
		t.Fatal("tool_use stop should not be Done")
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 7 || resp.Usage.CacheReadTokens != 5 || resp.Usage.CacheWriteTokens != 2 {
		t.Fatalf("usage mis-parsed: %+v", resp.Usage)
	}
}

func TestParseOpenAIStream(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"read_file","arguments":"{\"file\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x.go\"}"}}]}}]}`,
		`data: {"usage":{"prompt_tokens":10,"completion_tokens":7}}`,
		`data: [DONE]`,
	}, "\n")

	var deltas []string
	resp, emitted, _, _, err := parseOpenAIStream(context.Background(), strings.NewReader(sse), func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Hello" || !emitted || strings.Join(deltas, "|") != "Hel|lo" {
		t.Fatalf("text=%q deltas=%v", resp.Text, deltas)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "read_file" || resp.ToolCalls[0].Args["file"] != "x.go" {
		t.Fatalf("tool call mis-parsed: %+v", resp.ToolCalls)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 7 {
		t.Fatalf("usage mis-parsed: %+v", resp.Usage)
	}
}

func TestStreamAbortsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	sse := "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"x\"}}\n"
	if _, _, _, _, err := parseAnthropicStream(ctx, strings.NewReader(sse), nil); err == nil {
		t.Fatal("a cancelled context should abort the stream")
	}
}
