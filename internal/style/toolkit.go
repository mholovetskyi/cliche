package style

import (
	"io"
	"strings"
)

// This file adds the layout primitives the UI is built from. They are
// measurement-aware (Width), so alignment never drifts once color escapes or
// wide runes are present, and they all degrade cleanly when styling is off.

// ---- display-cell measurement ----

// Width reports the number of terminal display cells s occupies: ANSI escape
// sequences contribute 0, combining marks 0, East-Asian-wide / emoji runes 2,
// everything else 1. This is the keystone — utf8.RuneCountInString miscounts
// once Gradient() and friends embed escapes, which is the root of every
// alignment bug. The wide table is deliberately narrow: it covers genuinely
// double-width blocks (CJK, fullwidth, emoji) but NOT the geometric/box-drawing
// glyphs the UI actually emits (│ ┃ ◇ ❯ ▰ ▱ ⬡ ✓ ✗ ─), which render single-width
// on the dev terminals Cliche targets.
func Width(s string) int {
	w := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 0x1b { // ESC: skip an ANSI control sequence
			i++
			switch {
			case i < len(rs) && rs[i] == '[': // CSI: params/intermediates then a final 0x40-0x7e
				i++
				for i < len(rs) && (rs[i] < 0x40 || rs[i] > 0x7e) {
					i++
				}
			case i < len(rs) && rs[i] == ']': // OSC: until BEL or ST (ESC \)
				i++
				for i < len(rs) && rs[i] != 0x07 {
					if rs[i] == 0x1b && i+1 < len(rs) && rs[i+1] == '\\' {
						i++
						break
					}
					i++
				}
			}
			continue
		}
		w += runeWidth(r)
	}
	return w
}

func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x20 || (r >= 0x7f && r < 0xa0): // C0/C1 controls
		return 0
	case inRanges(r, combiningRanges):
		return 0
	case inRanges(r, wideRanges):
		return 2
	default:
		return 1
	}
}

func inRanges(r rune, ranges [][2]rune) bool {
	for _, rg := range ranges {
		if r < rg[0] {
			return false // ranges are sorted
		}
		if r <= rg[1] {
			return true
		}
	}
	return false
}

var combiningRanges = [][2]rune{
	{0x0300, 0x036F}, {0x0483, 0x0489}, {0x0591, 0x05BD}, {0x0610, 0x061A},
	{0x064B, 0x065F}, {0x0670, 0x0670}, {0x06D6, 0x06DC}, {0x1AB0, 0x1AFF},
	{0x1DC0, 0x1DFF}, {0x20D0, 0x20FF}, {0xFE20, 0xFE2F},
}

var wideRanges = [][2]rune{
	{0x1100, 0x115F}, {0x2329, 0x232A}, {0x2E80, 0x303E}, {0x3041, 0x33FF},
	{0x3400, 0x4DBF}, {0x4E00, 0x9FFF}, {0xA000, 0xA4CF}, {0xAC00, 0xD7A3},
	{0xF900, 0xFAFF}, {0xFE10, 0xFE19}, {0xFE30, 0xFE6F}, {0xFF00, 0xFF60},
	{0xFFE0, 0xFFE6}, {0x1F000, 0x1F0FF}, {0x1F300, 0x1FAFF}, {0x20000, 0x3FFFD},
}

// Pad right-pads s with spaces to at least n display cells (no truncation).
func Pad(s string, n int) string {
	if d := n - Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// Truncate clips s to at most n display cells on a rune boundary, appending an
// ellipsis when it cuts. Intended for plain (unstyled) text such as paths and
// commands, so it never severs a multibyte rune into a replacement char.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if Width(s) <= n {
		return s
	}
	w, budget := 0, n-1 // reserve one cell for the ellipsis
	var b strings.Builder
	for _, r := range s {
		cw := runeWidth(r)
		if w+cw > budget {
			break
		}
		b.WriteRune(r)
		w += cw
	}
	return b.String() + "…"
}

// ---- the rail: the one structural gutter ----

// Gutter is the display width every railed line is indented by (bar + space).
const Gutter = 2

// Rail prefixes every line of body with a single-cell colored vertical bar and a
// space — the one structural primitive that makes the whole transcript read as
// one contained unit. Under NO_COLOR the bar is an ASCII '|'. A single trailing
// empty line (from a body ending in "\n") is dropped so the rail never dangles.
func Rail(body string, bar rune, c RGB) string {
	prefix := "| "
	if Enabled {
		prefix = Color(string(bar), c) + " "
	}
	lines := strings.Split(body, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, ln := range lines {
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
}

// Indent left-pads every line of body by Gutter blank cells — the unbarred
// counterpart to Rail, for content that should align with railed content.
func Indent(body string) string {
	pad := strings.Repeat(" ", Gutter)
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		lines[i] = pad + ln
	}
	return strings.Join(lines, "\n")
}

// ---- the success accent ----

// Green is the success / added accent that sits beside coral-red.
func Green(s string) string { return Color(s, GreenRGB) }

// BoldGreen is Green at bold weight.
func BoldGreen(s string) string { return boldColor(s, GreenRGB) }

// Exported palette colors for callers that need an RGB (badges, chevrons, gauges).
var (
	GreenRGB = RGB{120, 200, 120} // sage success accent
	RedRGB   = RGB{229, 72, 77}   // coral-red accent
	GrayRGB  = RGB{138, 138, 138} // metadata/secondary
	WhiteRGB = RGB{237, 237, 237} // primary text
	InkRGB   = RGB{20, 20, 22}    // near-black, for text on a colored badge
)

// ---- badges ----

// Badge renders text as a small pill (fg on a bg fill), used sparingly for the
// mode chip at the prompt and the verdict chip at the outcome. Degrades to a
// bracketed [text] under NO_COLOR so the state still reads when piped.
func Badge(text string, fg, bg RGB) string {
	if !Enabled {
		return "[" + text + "]"
	}
	return fg.seq() + bg.bgSeq() + " " + text + " " + reset
}

// ---- gauge ----

// Gauge renders a proportion as a small coral bar of the given cell width
// (filled ▰ / empty ▱). Returns "" when styling is off, so callers pair it with
// a numeric "NN%" that carries the meaning under NO_COLOR.
func Gauge(frac float64, width int) string {
	if !Enabled || width <= 0 {
		return ""
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac*float64(width) + 0.5)
	denom := float64(width - 1)
	if denom < 1 {
		denom = 1
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			b.WriteString(Color("▰", Sample(float64(i)/denom)))
		} else {
			b.WriteString(Gray("▱"))
		}
	}
	return b.String()
}

// ---- cursor control ----

// HideCursor / ShowCursor toggle the terminal cursor (no-op when styling is off).
// Callers should `defer ShowCursor(out)` at entry points so a panic can never
// strand a hidden cursor.
func HideCursor(w io.Writer) {
	if Enabled {
		io.WriteString(w, "\x1b[?25l")
	}
}

func ShowCursor(w io.Writer) {
	if Enabled {
		io.WriteString(w, "\x1b[?25h")
	}
}

// ---- diagonal hero sweep ----

// Gradient2D colors s as one row of a stacked block, sampling the gradient on a
// diagonal (t advances with both column and row) so that rows rendered together
// form a single corner-to-corner sheen rather than identical horizontal sweeps.
func Gradient2D(s string, row, rows int, stops ...RGB) string {
	if len(stops) == 0 {
		stops = BrandGradient
	}
	if !Enabled || s == "" {
		return s
	}
	rs := []rune(s)
	denom := float64((len(rs) - 1) + (rows - 1))
	if denom <= 0 {
		denom = 1
	}
	var b strings.Builder
	for i, r := range rs {
		if r == ' ' || r == '\t' || r == '\n' {
			b.WriteRune(r)
			continue
		}
		c := colorAt(stops, float64(i+row)/denom)
		b.WriteString(c.seq())
		b.WriteRune(r)
	}
	b.WriteString(reset)
	return b.String()
}
