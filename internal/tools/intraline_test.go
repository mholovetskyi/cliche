package tools

import (
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestIntralineDiffHighlightsChangedWords(t *testing.T) {
	old := style.Enabled
	defer func() { style.Enabled = old }()

	style.Enabled = true
	o, n := intralineDiff("return foo(x)", "return bar(x)")

	// Content is preserved exactly — the styling escapes carry zero display width.
	if style.Width(o) != style.Width("return foo(x)") {
		t.Fatalf("old half lost content: width %d", style.Width(o))
	}
	if style.Width(n) != style.Width("return bar(x)") {
		t.Fatalf("new half lost content: width %d", style.Width(n))
	}
	if !strings.Contains(o, "foo") || !strings.Contains(n, "bar") {
		t.Fatalf("changed tokens missing: o=%q n=%q", o, n)
	}
	// The differing word is bold (bright); shared words are not.
	if !strings.Contains(o, "\x1b[1m") || !strings.Contains(n, "\x1b[1m") {
		t.Fatalf("changed token should be bold: o=%q n=%q", o, n)
	}

	style.Enabled = false
	if o2, n2 := intralineDiff("a b c", "a x c"); o2 != "a b c" || n2 != "a x c" {
		t.Fatalf("disabled should be plain, got %q / %q", o2, n2)
	}
}

// A single-line edit gets the word-level treatment; a multi-line block replace
// does not (its lines render plainly, never mis-paired).
func TestRenderDiffPairingScope(t *testing.T) {
	old := style.Enabled
	style.Enabled = true
	defer func() { style.Enabled = old }()

	// Single changed line → both halves present, word-highlighted.
	single := changePreview("a\nfoo bar\nc\n", "a\nfoo baz\nc\n")
	if !strings.Contains(ansiRe.ReplaceAllString(single, ""), "- foo bar") ||
		!strings.Contains(ansiRe.ReplaceAllString(single, ""), "+ foo baz") {
		t.Fatalf("single-line replace lost its lines:\n%s", single)
	}

	// A block replace still renders every changed line (no crash, no loss).
	block := changePreview("a\nb\n", "x\ny\n")
	plain := ansiRe.ReplaceAllString(block, "")
	for _, want := range []string{"- a", "- b", "+ x", "+ y"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("block replace missing %q:\n%s", want, block)
		}
	}
}
