package provider

import (
	"strings"
	"testing"
)

// A tool (e.g. screenshot) can return an image; both providers must forward it to
// the model so it can see the result — Anthropic as an image block in the same
// user/tool_result message, OpenAI as a following user vision message.
func TestToolResultCarriesImages(t *testing.T) {
	msg := Message{
		Role:        "user",
		ToolResults: []ToolResult{{ID: "t1", Content: "Captured a screenshot."}},
		Images:      []Image{{MediaType: "image/png", Data: []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}}},
	}

	ab, _ := NewAnthropic("k", "claude-x", 1024).buildRequestBody(Request{Messages: []Message{msg}}, false)
	a := string(ab)
	if !strings.Contains(a, `"type":"tool_result"`) || !strings.Contains(a, `"type":"image"`) {
		t.Fatalf("anthropic tool_result message must carry an image block:\n%s", a)
	}

	ob, _ := NewOpenAICompat("k", "gpt-x", "https://x/v1/chat/completions", 1024).buildRequestBody(Request{Messages: []Message{msg}}, false)
	o := string(ob)
	if !strings.Contains(o, `"role":"tool"`) || !strings.Contains(o, `"image_url"`) {
		t.Fatalf("openai must emit a tool message then a user image message:\n%s", o)
	}
	if strings.Index(o, `"role":"tool"`) > strings.Index(o, `"image_url"`) {
		t.Fatal("the tool reply must precede the user image message")
	}
}
