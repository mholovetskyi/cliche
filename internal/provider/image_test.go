package provider

import (
	"strings"
	"testing"
)

// TestImageSerialization proves both backends emit the correct vision wire
// format for an attached image — the part that can't be validated against a live
// vision model in CI.
func TestImageSerialization(t *testing.T) {
	msg := Message{
		Role:   "user",
		Text:   "what is this",
		Images: []Image{{MediaType: "image/png", Data: []byte("\x89PNGdata")}},
	}

	// Anthropic: an image content block carrying a base64 source.
	body, err := NewAnthropic("k", "claude-x", 1024).buildRequestBody(Request{Messages: []Message{msg}}, false)
	if err != nil {
		t.Fatal(err)
	}
	a := string(body)
	for _, want := range []string{`"type":"image"`, `"media_type":"image/png"`, `"data":"`, `"type":"text"`, "what is this"} {
		if !strings.Contains(a, want) {
			t.Fatalf("anthropic image request missing %q:\n%s", want, a)
		}
	}

	// OpenAI-compatible: content is an array of text + image_url (data URI) parts.
	body2, err := NewOpenAICompat("k", "gpt-x", "https://x/v1/chat/completions", 1024).buildRequestBody(Request{Messages: []Message{msg}}, false)
	if err != nil {
		t.Fatal(err)
	}
	o := string(body2)
	for _, want := range []string{`"type":"image_url"`, "data:image/png;base64,", `"type":"text"`, "what is this"} {
		if !strings.Contains(o, want) {
			t.Fatalf("openai image request missing %q:\n%s", want, o)
		}
	}

	// A plain text message still serializes content as a string, not an array.
	body3, _ := NewOpenAICompat("k", "gpt-x", "https://x/v1/chat/completions", 1024).
		buildRequestBody(Request{Messages: []Message{{Role: "user", Text: "hi"}}}, false)
	if strings.Contains(string(body3), `"image_url"`) {
		t.Fatalf("a text-only message must not produce image parts:\n%s", string(body3))
	}
}
