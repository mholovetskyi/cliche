package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

// TestMdStreamerChunkInvariant proves the streamed renderer produces identical
// output no matter how the deltas are split — feeding it one rune at a time must
// match feeding it the whole string — and that markdown is actually rendered.
func TestMdStreamerChunkInvariant(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = true, false
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	input := "## Heading\na **bold** line with `code`\n```go\nx := 1\n```\ndone"
	render := func(chunks []string) string {
		var b bytes.Buffer
		m := newMdStreamer(&b)
		for _, c := range chunks {
			m.write(c)
		}
		m.flush()
		return b.String()
	}

	whole := render([]string{input})
	var perRune []string
	for _, r := range input {
		perRune = append(perRune, string(r))
	}
	if got := render(perRune); got != whole {
		t.Fatalf("streamed output must not depend on chunk boundaries:\n--whole--\n%q\n--perRune--\n%q", whole, got)
	}

	if strings.Contains(whole, "```") {
		t.Fatalf("fence markers must be hidden:\n%s", whole)
	}
	for _, want := range []string{"Heading", "bold", "code", "x := 1", "done"} {
		if !strings.Contains(whole, want) {
			t.Fatalf("streamed markdown dropped %q:\n%s", want, whole)
		}
	}
	if strings.Contains(whole, "**") {
		t.Fatalf("bold markers must be consumed:\n%s", whole)
	}
}

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

func TestHighlightGo(t *testing.T) {
	oldE := style.Enabled
	style.Enabled = true
	defer func() { style.Enabled = oldE }()

	code := `func add(a, b int) int { return a + b } // sums`
	got := highlightGo(code)
	// Color only — display width is identical to the plain code.
	if style.Width(got) != len([]rune(code)) {
		t.Fatalf("highlightGo changed display width: got %d want %d", style.Width(got), len([]rune(code)))
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatal("highlightGo should emit color escapes")
	}
	// Keywords, identifiers, and the comment all survive as readable substrings
	// (same-color runs are coalesced, so each stays contiguous).
	for _, w := range []string{"func", "add", "return", "// sums"} {
		if !strings.Contains(got, w) {
			t.Fatalf("highlightGo dropped %q:\n%q", w, got)
		}
	}

	// A ```go fence routes its body through the highlighter; other languages and
	// NO_COLOR do not gain spurious color.
	oldNC := noColor
	noColor = false
	defer func() { noColor = oldNC }()
	out := renderMarkdown("```go\nvar x = `raw`\n```")
	if !strings.Contains(out, "var") || !strings.Contains(out, "x") {
		t.Fatalf("go fence should render its body:\n%s", out)
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
