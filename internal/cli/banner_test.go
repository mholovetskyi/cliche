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

	// Rendered (styling on): emits ANSI and the block wordmark.
	style.Enabled = true
	s := splash()
	if !strings.Contains(s, "\x1b[38;2;") {
		t.Fatal("styled splash should contain truecolor escapes")
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
	want := 43
	for i, row := range clicheLetters {
		if n := utf8.RuneCountInString(row); n != want {
			t.Errorf("clicheLetters[%d] is %d runes, want %d: %q", i, n, want, row)
		}
	}
	if accentCol+2 > want {
		t.Fatalf("accent at col %d (+2) would exceed art width %d", accentCol, want)
	}
}
