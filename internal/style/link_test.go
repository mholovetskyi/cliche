package style

import (
	"strings"
	"testing"
)

func TestHyperlinkWrapsWhenEnabled(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()
	Enabled = true

	got := Hyperlink("click me", "https://example.com")
	if !strings.Contains(got, "\x1b]8;;https://example.com\x1b\\") || !strings.HasSuffix(got, "\x1b]8;;\x1b\\") {
		t.Fatalf("missing OSC 8 framing: %q", got)
	}
	if !strings.Contains(got, "click me") {
		t.Fatalf("link text dropped: %q", got)
	}
	// A hyperlink adds zero display width — it measures the same as its text.
	if Width(got) != Width("click me") {
		t.Fatalf("hyperlink width %d != text width %d", Width(got), Width("click me"))
	}
}

func TestHyperlinkPlainWhenDisabled(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()
	Enabled = false
	if got := Hyperlink("text", "https://x"); got != "text" {
		t.Fatalf("disabled should be plain, got %q", got)
	}
}

func TestHyperlinkRejectsControlChars(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()
	Enabled = true
	if got := Hyperlink("text", "https://x\x1b]evil"); got != "text" {
		t.Fatalf("a control char in the url must fall back to plain text, got %q", got)
	}
}

func TestLinkURLLinksTokenKeepsProse(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()
	Enabled = true

	in := "console.anthropic.com → API keys"
	got := LinkURL(in)
	if !strings.Contains(got, "\x1b]8;;https://console.anthropic.com\x1b\\") {
		t.Fatalf("token not linked with an https scheme: %q", got)
	}
	if !strings.HasSuffix(got, " → API keys") {
		t.Fatalf("trailing prose must stay plain: %q", got)
	}
	if Width(got) != Width(in) {
		t.Fatalf("visible width changed: %d vs %d", Width(got), Width(in))
	}
}

func TestLinkURLBareTokenAndExistingScheme(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()
	Enabled = true

	if got := LinkURL("console.x.ai"); !strings.Contains(got, "\x1b]8;;https://console.x.ai\x1b\\") {
		t.Fatalf("a bare token should link to https://<token>: %q", got)
	}
	if got := LinkURL("https://go.dev/dl/"); strings.Contains(got, "https://https://") {
		t.Fatalf("an existing scheme must not be double-prefixed: %q", got)
	}
}

func TestLinkURLPlainWhenDisabled(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()
	Enabled = false
	in := "console.anthropic.com → API keys"
	if got := LinkURL(in); got != in {
		t.Fatalf("disabled should be unchanged, got %q", got)
	}
}
