package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestAnimGatedOffStaysStatic(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true // not a styled TTY → no animation, no sleeps
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	if animOn() {
		t.Fatal("animOn must be false when styling is off")
	}
	var buf bytes.Buffer
	revealWordmark(&buf)
	if !strings.Contains(buf.String(), "█") {
		t.Fatalf("revealWordmark (off) should print the static wordmark:\n%s", buf.String())
	}

	buf.Reset()
	dispatchSweep(&buf)
	if buf.Len() != 0 {
		t.Fatalf("dispatchSweep should be a no-op when off, got %q", buf.String())
	}

	buf.Reset()
	typeLine(&buf, "  ", "hello", style.WhiteRGB)
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("typeLine (off) should print the whole line: %q", buf.String())
	}
}

func TestWordmarkFrameWidth(t *testing.T) {
	oldE := style.Enabled
	style.Enabled = true
	defer func() { style.Enabled = oldE }()

	// A mid-reveal frame: each rendered line is exactly gutter + artWidth cells,
	// so the bright pen and the blanks never skew the block.
	frame := wordmarkFrame(paddedLetterRows(), artWidth/2)
	for _, ln := range strings.Split(strings.TrimRight(frame, "\n"), "\n") {
		if style.Width(ln) != style.Gutter+artWidth {
			t.Fatalf("frame line width = %d, want %d: %q", style.Width(ln), style.Gutter+artWidth, ln)
		}
	}
}

func TestCLICHENoAnimDisables(t *testing.T) {
	oldE := style.Enabled
	style.Enabled = true
	defer func() { style.Enabled = oldE }()
	t.Setenv("CLICHE_NO_ANIM", "1")
	if animOn() {
		t.Fatal("CLICHE_NO_ANIM=1 must disable animations even on a styled TTY")
	}
}
