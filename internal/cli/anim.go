package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/style"
)

// This file holds the timed terminal effects — the animated splash reveal, the
// boot sequence, the typewriter, and the dispatch flourish. They are all gated
// on animOn(), so a pipe / NO_COLOR / CI / `CLICHE_NO_ANIM` gets the final static
// frame instantly with no sleeps, and unit tests never block.

// animOn reports whether timed effects should play: only on a styled TTY, and
// never when the user opts out via CLICHE_NO_ANIM.
func animOn() bool {
	return style.Enabled && os.Getenv("CLICHE_NO_ANIM") == ""
}

func frameSleep(d time.Duration) { time.Sleep(d) }

// cursorUp moves the cursor up n lines (column unchanged) for in-place redraw.
func cursorUp(out io.Writer, n int) {
	if n > 0 {
		fmt.Fprintf(out, "\x1b[%dA", n)
	}
}

// wordmarkFrame renders the six wordmark rows revealed up to column e: revealed
// cells take the brand gradient (the final E forced red), the leading edge is
// bright white (the "drawing pen"), and unrevealed cells are blank.
func wordmarkFrame(rows [][]rune, e int) string {
	var b strings.Builder
	n := len(rows)
	for r, rs := range rows {
		b.WriteString("  ")
		for c, ch := range rs {
			if ch == ' ' || c > e {
				b.WriteByte(' ')
				continue
			}
			var col style.RGB
			switch {
			case c == e:
				col = style.WhiteRGB // the bright leading edge
			case c >= eBlockCol:
				col = style.RedRGB
			default:
				col = style.GradientAt(c, r, artWidth, n)
			}
			b.WriteString(style.Color(string(ch), col))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// revealWordmark draws the CLICHÉ block in left-to-right behind a bright pen,
// then settles into the static gradient. Prints the final frame instantly when
// animations are off.
func revealWordmark(out io.Writer) {
	lines := heroLetterLines()
	if !animOn() {
		for _, ln := range lines {
			fmt.Fprintln(out, ln)
		}
		return
	}
	rows := paddedLetterRows()
	n := len(rows)
	style.HideCursor(out)
	defer style.ShowCursor(out)
	for i := 0; i < n; i++ {
		fmt.Fprintln(out) // reserve the block's lines so cursorUp has room
	}
	const steps = 20
	for s := 0; s <= steps; s++ {
		cursorUp(out, n)
		fmt.Fprint(out, wordmarkFrame(rows, artWidth*s/steps))
		frameSleep(16 * time.Millisecond)
	}
	cursorUp(out, n)
	for _, ln := range lines {
		fmt.Fprintln(out, ln) // settle: the clean static gradient, no bright edge
	}
}

// revealLines prints lines one at a time with a small delay — the "system coming
// online" cadence of the boot sequence.
func revealLines(out io.Writer, lines []string, delay time.Duration) {
	for _, ln := range lines {
		fmt.Fprintln(out, ln)
		if animOn() {
			frameSleep(delay)
		}
	}
}

// typeLine writes text a character at a time in color c (instant when off).
func typeLine(out io.Writer, prefix, text string, c style.RGB) {
	if !animOn() {
		fmt.Fprintln(out, prefix+style.Color(text, c))
		return
	}
	fmt.Fprint(out, prefix)
	for _, r := range text {
		fmt.Fprint(out, style.Color(string(r), c))
		frameSleep(7 * time.Millisecond)
	}
	fmt.Fprintln(out)
}

// bootLine renders one "◇ label ▸ value" status row of the splash boot sequence.
func bootLine(label, value string) string {
	return "  " + style.Color(gl("◇", "*"), style.Sample(0)) + " " +
		style.Gray(style.Pad(label, 13)) + style.Color(gl("▸", ">"), style.Sample(0.55)) +
		" " + style.White(value)
}

// loginIntro renders the login wizard's opener — the same animated wordmark
// reveal as a chat session, over the "connect a provider" frame — or the static
// loginBanner when animations are off.
func loginIntro(out io.Writer) {
	if !animOn() {
		fmt.Fprint(out, loginBanner())
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, heroAccentLine())
	revealWordmark(out)
	fmt.Fprintln(out)
	revealLines(out, []string{
		"  " + style.GradientRule(artWidth),
		"  " + style.White("connect a model provider"),
		"  " + style.Gray("BYO-key · stored locally (0600) · never sent anywhere but the provider"),
		"  " + style.GradientRule(artWidth),
	}, 40*time.Millisecond)
}

// dispatchSweep plays a quick left-to-right gradient pulse on a single line when
// a prompt is sent, then clears it — a brief "sending" flourish before the
// thinking spinner. No-op when animations are off.
func dispatchSweep(out io.Writer) {
	if !animOn() {
		return
	}
	style.HideCursor(out)
	defer style.ShowCursor(out)
	const w = 36
	for e := 2; e <= w; e += 2 {
		var b strings.Builder
		b.WriteString("\r  ")
		for i := 0; i < w; i++ {
			if i < e {
				b.WriteString(style.Color("▰", style.GradientAt(i, 0, w, 1)))
			} else {
				b.WriteByte(' ')
			}
		}
		fmt.Fprint(out, b.String())
		frameSleep(10 * time.Millisecond)
	}
	fmt.Fprint(out, "\r\x1b[2K") // wipe the sweep line; the spinner takes over
}
