package cli

import (
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestRenderMarkdown(t *testing.T) {
	old := style.Enabled
	defer func() { style.Enabled = old }()

	// Styling off → returned unchanged (pipes/CI/tests stay plain).
	style.Enabled = false
	// Use a ## heading (bold, not per-char gradient) so words stay contiguous.
	in := "## Title\n- a `code` item\n```go\nfmt.Println()\n```"
	if renderMarkdown(in) != in {
		t.Fatal("with styling off, markdown should pass through unchanged")
	}

	// Styling on → escapes emitted; the ``` fence markers are hidden but the
	// code line survives; inline code/bold are styled.
	style.Enabled = true
	out := renderMarkdown(in)
	if strings.Contains(out, "```") {
		t.Fatalf("fence markers should be hidden:\n%s", out)
	}
	if !strings.Contains(out, "fmt.Println()") {
		t.Fatalf("code content should survive:\n%s", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("styled output should contain ANSI escapes:\n%s", out)
	}
	// "Title" and "code" both still present (markers stripped).
	for _, want := range []string{"Title", "code", "item"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in rendered output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "**") || strings.Contains(out, "## Title") {
		t.Fatalf("markdown markers should be consumed:\n%s", out)
	}
}

func TestInlineMarkdown(t *testing.T) {
	old := style.Enabled
	style.Enabled = true
	defer func() { style.Enabled = old }()
	got := inlineMarkdown("use **bold** and `code` here")
	if strings.Contains(got, "**") || strings.Contains(got, "`") {
		t.Fatalf("inline markers should be consumed: %q", got)
	}
	for _, w := range []string{"bold", "code", "here", "use"} {
		if !strings.Contains(got, w) {
			t.Fatalf("text dropped: %q missing from %q", w, got)
		}
	}
}
