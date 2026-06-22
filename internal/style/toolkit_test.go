package style

import (
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
