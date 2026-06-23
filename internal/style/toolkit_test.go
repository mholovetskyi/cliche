package style

import (
	"math"
	"strings"
	"testing"
)

func TestWidthGlyphTable(t *testing.T) {
	// Every glyph the UI actually emits must measure as a single cell, or the
	// rail/columns drift. Combining marks are 0; genuine emoji are 2.
	single := []string{"│", "┃", "└", "├", "─", "◇", "◆", "❯", "▰", "▱", "▌", "■", "✓", "✗", "⬡", "⚠", "⚡", "•", "›", "→"}
	for _, g := range single {
		if w := Width(g); w != 1 {
			t.Errorf("Width(%q) = %d, want 1", g, w)
		}
	}
	if w := Width("é"); w != 1 { // e + combining acute = one cell
		t.Errorf("Width(combining é) = %d, want 1", w)
	}
	if w := Width("世界"); w != 4 { // CJK, 2 cells each
		t.Errorf("Width(CJK) = %d, want 4", w)
	}
	if w := Width("📖"); w != 2 { // emoji is double-width
		t.Errorf("Width(emoji) = %d, want 2", w)
	}
}

func TestWidthIgnoresANSI(t *testing.T) {
	old := Enabled
	Enabled = true
	defer func() { Enabled = old }()
	plain := "Read main.go"
	styled := Red("Read") + " " + White("main.go")
	if Width(styled) != Width(plain) {
		t.Fatalf("Width should ignore ANSI escapes: styled=%d plain=%d", Width(styled), Width(plain))
	}
	if Width(plain) != len(plain) { // all single-byte here
		t.Fatalf("Width(%q) = %d, want %d", plain, Width(plain), len(plain))
	}
}

func TestPadToDisplayCells(t *testing.T) {
	old := Enabled
	Enabled = true
	defer func() { Enabled = old }()
	// Pad measures display cells, not bytes — a colored token pads correctly.
	got := Pad(Red("Edit"), 8)
	if Width(got) != 8 {
		t.Fatalf("Pad to 8 cells gave width %d", Width(got))
	}
	if Pad("already wider", 3) != "already wider" {
		t.Fatal("Pad must not truncate")
	}
}

func TestTruncateRuneSafe(t *testing.T) {
	if got := Truncate("hello world", 5); got != "hell…" {
		t.Fatalf("Truncate = %q, want %q", got, "hell…")
	}
	if got := Truncate("short", 10); got != "short" {
		t.Fatalf("Truncate should pass through short strings, got %q", got)
	}
	// Must not split a multibyte rune into a replacement char.
	got := Truncate("café société", 6)
	if !strings.HasSuffix(got, "…") || strings.ContainsRune(got, '�') {
		t.Fatalf("Truncate produced bad output: %q", got)
	}
	if Width(got) > 6 {
		t.Fatalf("Truncate exceeded budget: width %d", Width(got))
	}
}

func TestRailPrefixesAndFallsBack(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()

	Enabled = false
	got := Rail("a\nb", '│', RGB{1, 2, 3})
	if got != "| a\n| b" {
		t.Fatalf("NO_COLOR rail = %q, want %q", got, "| a\n| b")
	}
	// A trailing empty line (body ending in \n) must not dangle a bare bar.
	if got := Rail("a\n", '│', RGB{1, 2, 3}); got != "| a" {
		t.Fatalf("rail with trailing newline = %q, want %q", got, "| a")
	}

	Enabled = true
	styled := Rail("x", '│', RGB{1, 2, 3})
	if Width(styled) != 3 { // bar + space + x
		t.Fatalf("railed line width = %d, want 3", Width(styled))
	}
}

func TestGaugeAndBadgeDegrade(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()

	Enabled = false
	if Gauge(0.5, 6) != "" {
		t.Fatal("Gauge must be empty when styling is off (caller prints the %)")
	}
	if Badge("full", RGB{255, 255, 255}, RGB{1, 2, 3}) != "[full]" {
		t.Fatal("Badge must degrade to [text] under NO_COLOR")
	}

	Enabled = true
	if Width(Gauge(0.5, 6)) != 6 {
		t.Fatalf("Gauge should be exactly width cells, got %d", Width(Gauge(0.5, 6)))
	}
}

func TestGaugeEdgeCases(t *testing.T) {
	old := Enabled
	Enabled = true
	defer func() { Enabled = old }()

	// width 1, the boundary fractions, and out-of-range / non-finite inputs must
	// all stay exactly `width` cells and never panic.
	cases := []struct {
		frac float64
		w    int
	}{
		{0, 1}, {1, 1}, {0.5, 1},
		{0, 8}, {1, 8}, {-0.3, 8}, {1.7, 8},
		{math.NaN(), 8}, {math.Inf(1), 8}, {math.Inf(-1), 8},
	}
	for _, c := range cases {
		got := Gauge(c.frac, c.w)
		if Width(got) != c.w {
			t.Errorf("Gauge(%v,%d) width = %d, want %d", c.frac, c.w, Width(got), c.w)
		}
	}
	if Gauge(0, 0) != "" || Gauge(0.5, -1) != "" {
		t.Fatal("Gauge must be empty for non-positive width")
	}
}

func TestWidthMalformedOSC(t *testing.T) {
	// An OSC sequence (ESC ]) with neither a BEL nor an ST terminator must be
	// consumed without hanging and contribute no display cells.
	if w := Width("\x1b]8;;no-terminator-here"); w != 0 {
		t.Fatalf("unterminated OSC width = %d, want 0 (consumed)", w)
	}
	// A properly terminated OSC (BEL) followed by text counts only the text.
	if w := Width("\x1b]0;title\x07hi"); w != 2 {
		t.Fatalf("terminated OSC + text width = %d, want 2", w)
	}
}

func TestWidthGradientPreservesCells(t *testing.T) {
	old := Enabled
	Enabled = true
	defer func() { Enabled = old }()
	// Gradient embeds a color escape before every non-space rune; Width must see
	// straight through them, including CJK (2 cells) and a combining mark (0).
	s := "café 世界"
	if Width(Gradient(s)) != Width(s) {
		t.Fatalf("Gradient changed measured width: %d vs %d", Width(Gradient(s)), Width(s))
	}
	if Width(s) != 4+1+4 { // café=4, space=1, 世界=4
		t.Fatalf("baseline Width(%q) = %d, want 9", s, Width(s))
	}
}

func TestPadCenterAndPadLeft(t *testing.T) {
	if got := PadCenter("hi", 6); got != "  hi  " {
		t.Fatalf("PadCenter = %q, want %q", got, "  hi  ")
	}
	// Odd slack: floor-left, ceil-right.
	if got := PadCenter("hi", 5); got != " hi  " {
		t.Fatalf("PadCenter odd = %q, want %q", got, " hi  ")
	}
	if PadCenter("toolong", 3) != "toolong" {
		t.Fatal("PadCenter must not truncate")
	}
	if got := padLeft("x", 4); got != "   x" {
		t.Fatalf("padLeft = %q, want %q", got, "   x")
	}
}

func TestTruncateLeftKeepsTail(t *testing.T) {
	got := TruncateLeft("/a/b/c/internal/cli/session.go", 12)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, ".go") {
		t.Fatalf("TruncateLeft should keep the tail behind an ellipsis: %q", got)
	}
	if Width(got) > 12 {
		t.Fatalf("TruncateLeft exceeded budget: width %d (%q)", Width(got), got)
	}
	if TruncateLeft("short", 10) != "short" {
		t.Fatal("TruncateLeft should pass through short strings")
	}
}

func TestTableRowColumnAlignment(t *testing.T) {
	old := Enabled
	Enabled = true
	defer func() { Enabled = old }()
	// Styled cells must still land on fixed column boundaries.
	row := TableRow([]string{Red("Read"), White("main.go")}, []int{8, 12}, []Align{AlignLeft, AlignLeft})
	if Width(row) != 8+1+12 {
		t.Fatalf("TableRow width = %d, want 21", Width(row))
	}
	// Right alignment pads on the left.
	if got := TableRow([]string{"x"}, []int{5}, []Align{AlignRight}); got != "    x" {
		t.Fatalf("right-aligned cell = %q, want %q", got, "    x")
	}
}

func TestBoxFramesUniformWidth(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()

	Enabled = true
	box := Box("status", "mode  suggest\nmodel "+White("gpt-4o-mini"), RGB{229, 72, 77})
	lines := strings.Split(box, "\n")
	if len(lines) != 4 { // top + 2 body + bottom
		t.Fatalf("box should have 4 lines, got %d:\n%s", len(lines), box)
	}
	w0 := Width(lines[0])
	for i, ln := range lines {
		if Width(ln) != w0 {
			t.Fatalf("box line %d width %d != top width %d:\n%s", i, Width(ln), w0, box)
		}
	}
	if !strings.Contains(box, "status") {
		t.Fatal("box should inset its title")
	}

	// NO_COLOR: degrade to a bracketed title over an indented body.
	Enabled = false
	plain := Box("status", "a\nb", RGB{1, 2, 3})
	if !strings.Contains(plain, "[status]") || !strings.Contains(plain, "  a") {
		t.Fatalf("NO_COLOR box should be [title] + indented body, got %q", plain)
	}
}
