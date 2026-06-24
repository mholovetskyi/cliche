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

func TestPDFSerialization(t *testing.T) {
	msg := Message{Role: "user", Text: "summarize", Images: []Image{{MediaType: "application/pdf", Data: []byte("%PDF-1.4")}}}

	// Anthropic: a document content block (not an image block).
	body, _ := NewAnthropic("k", "claude-x", 1024).buildRequestBody(Request{Messages: []Message{msg}}, false)
	if !strings.Contains(string(body), `"type":"document"`) || !strings.Contains(string(body), "application/pdf") {
		t.Fatalf("anthropic PDF should be a document block:\n%s", string(body))
	}

	// OpenAI: PDFs aren't a supported image_url part — the text survives, no PDF.
	body2, _ := NewOpenAICompat("k", "gpt-x", "https://x", 1024).buildRequestBody(Request{Messages: []Message{msg}}, false)
	if strings.Contains(string(body2), "application/pdf") || strings.Contains(string(body2), "image_url") {
		t.Fatalf("openai must not emit a PDF/image_url part:\n%s", string(body2))
	}
	if !strings.Contains(string(body2), "summarize") {
		t.Fatalf("openai should keep the message text:\n%s", string(body2))
	}
}
