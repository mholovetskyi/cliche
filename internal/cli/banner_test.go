package cli

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestSplash(t *testing.T) {
	old := style.Enabled
	defer func() { style.Enabled = old }()

	// Rendered (styling on): emits ANSI escapes (the exact form depends on the
	// terminal's color tier, so just assert color is present).
	style.Enabled = true
	s := splash()
	if !strings.Contains(s, "\x1b[") {
		t.Fatal("styled splash should contain ANSI color escapes")
	}

	// The content (provider-agnostic checks; works styled or plain).
	style.Enabled = false
	p := splash()
	for _, want := range []string{
		"██████╗",      // the block wordmark
		"╱╱",           // the é acute accent
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
