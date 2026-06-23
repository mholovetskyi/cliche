package cli

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestSplash(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	// Rendered (styling on): emits ANSI escapes (the exact form depends on the
	// terminal's color tier, so just assert color is present), and the acute
	// accent stroke is drawn over the final E.
	style.Enabled, noColor = true, false
	s := splash()
	if !strings.Contains(s, "\x1b[") {
		t.Fatal("styled splash should contain ANSI color escapes")
	}
	if !strings.Contains(s, "╱") {
		t.Fatal("styled splash should render the acute accent stroke")
	}

	// The content (provider-agnostic checks; works styled or plain).
	style.Enabled, noColor = false, true
	p := splash()
	for _, want := range []string{
		"██████╗",      // the block wordmark
		"███████╗",     // the final E block (forced red, present in plain text)
		"cli·ché",      // dictionary motif
		"loop breaker", // accent phrase
		"the AI coding agent you can actually leave running.",
		"login", "chat", "demo", // command palette
		"get started",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("splash missing %q:\n%s", want, p)
		}
	}
}

// TestHeroAccentAndEBlock guards the rank-1 brand fix: the accent degrades
// safely under NO_COLOR, and the gradient/red-E split never skews the block.
func TestHeroAccentAndEBlock(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	style.Enabled, noColor = true, false
	hero := heroLogo()
	if !strings.Contains(hero, "╱") {
		t.Fatal("styled hero should render the acute accent stroke")
	}
	// Every letter row stays exactly gutter+artWidth cells despite the embedded
	// gradient + red escapes (the alignment keystone for the wordmark).
	for _, ln := range strings.Split(strings.TrimRight(hero, "\n"), "\n") {
		if strings.Contains(ln, "█") && style.Width(ln) != style.Gutter+artWidth {
			t.Fatalf("hero letter row width = %d, want %d: %q", style.Width(ln), style.Gutter+artWidth, ln)
		}
	}

	// NO_COLOR: no raw box-drawing accent leaks, but the block wordmark remains.
	style.Enabled, noColor = false, true
	plain := heroLogo()
	if strings.Contains(plain, "╱") {
		t.Fatal("NO_COLOR hero must not emit the box-drawing accent stroke")
	}
	if !strings.Contains(plain, "█") {
		t.Fatal("hero should still render the block wordmark under NO_COLOR")
	}
}

// TestSplashArtAligned guards the invariant that makes the gradient form a
// coherent vertical band: every wordmark row must be the same rune width once
// padded (8+8+3+8+8+8 = 43 for the letters; the accent row is shorter and gets
// padded up).
func TestSplashArtAligned(t *testing.T) {
	for i, row := range clicheLetters {
		if n := utf8.RuneCountInString(row); n != artWidth {
			t.Errorf("clicheLetters[%d] is %d runes, want %d: %q", i, n, artWidth, row)
		}
	}
	// The é acute must sit within the final-E span (columns 35-42), or it reads
	// as a detached mark rather than the accent on cliché.
	if accentCol < 35 || accentCol+2 > 43 {
		t.Fatalf("accent at col %d (+2) is not within the E span 35-42", accentCol)
	}
}
