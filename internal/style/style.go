// Package style renders Cliche's brand palette (red / black / white) as ANSI
// truecolor escapes. It is zero-dependency and auto-disables on a non-TTY or
// when NO_COLOR is set, so piped/CI output stays plain.
package style

import (
	"fmt"
	"os"
	"strings"
)

// Enabled controls whether styling is emitted. Defaults to on for a
// color-capable stdout; overridable (e.g. tests set it false).
var Enabled = detect()

func detect() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICHE_FORCE_COLOR") != "" {
		return true // force color even when piped (e.g. into `less -R`)
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	// Brand palette (truecolor): coral-red, near-white, mid-gray.
	red   = "\x1b[38;2;229;72;77m"
	white = "\x1b[38;2;237;237;237m"
	gray  = "\x1b[38;2;138;138;138m"
)

func wrap(seq, s string) string {
	if !Enabled || s == "" {
		return s
	}
	return seq + s + reset
}

// Red is the accent color (halts, the é, flagged verdicts, the prompt).
func Red(s string) string { return wrap(red, s) }

// White is primary text.
func White(s string) string { return wrap(white, s) }

// Gray is secondary/label text.
func Gray(s string) string { return wrap(gray, s) }

// Dim is de-emphasized text.
func Dim(s string) string { return wrap(dim, s) }

// Bold emphasizes without changing color.
func Bold(s string) string { return wrap(bold, s) }

// BoldWhite / BoldRed are the wordmark weights.
func BoldWhite(s string) string { return wrap(bold+white, s) }
func BoldRed(s string) string   { return wrap(bold+red, s) }

// RGB is a 24-bit color used for gradients.
type RGB struct{ R, G, B int }

// BrandGradient is the signature coral sweep (deep red → coral → warm peach)
// used for the wordmark and rules. Cohesive with the red accent, but alive.
var BrandGradient = []RGB{{229, 72, 77}, {255, 121, 99}, {255, 179, 128}}

func (c RGB) seq() string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B) }

func lerp(a, b RGB, t float64) RGB {
	return RGB{
		R: a.R + int(float64(b.R-a.R)*t+0.5),
		G: a.G + int(float64(b.G-a.G)*t+0.5),
		B: a.B + int(float64(b.B-a.B)*t+0.5),
	}
}

// colorAt samples a multi-stop gradient at position t in [0,1].
func colorAt(stops []RGB, t float64) RGB {
	if len(stops) == 1 {
		return stops[0]
	}
	if t <= 0 {
		return stops[0]
	}
	if t >= 1 {
		return stops[len(stops)-1]
	}
	seg := t * float64(len(stops)-1)
	i := int(seg)
	if i >= len(stops)-1 {
		i = len(stops) - 2
	}
	return lerp(stops[i], stops[i+1], seg-float64(i))
}

// Gradient colors each rune of s across the given stops (left→right). Whitespace
// is passed through uncolored. Falls back to plain text when styling is off.
func Gradient(s string, stops ...RGB) string {
	if len(stops) == 0 {
		stops = BrandGradient
	}
	if !Enabled || s == "" {
		return s
	}
	rs := []rune(s)
	denom := len(rs) - 1
	var b strings.Builder
	for i, r := range rs {
		if r == ' ' || r == '\t' || r == '\n' {
			b.WriteRune(r)
			continue
		}
		t := 0.0
		if denom > 0 {
			t = float64(i) / float64(denom)
		}
		c := colorAt(stops, t)
		b.WriteString(c.seq())
		b.WriteRune(r)
	}
	b.WriteString(reset)
	return b.String()
}

// GradientRule returns a horizontal rule of the given width drawn with a
// gradient (the brand sweep by default).
func GradientRule(width int, stops ...RGB) string {
	if width <= 0 {
		width = 48
	}
	return Gradient(strings.Repeat("─", width), stops...)
}

// GradientBold is Gradient with bold weight, for headline text.
func GradientBold(s string, stops ...RGB) string {
	if !Enabled || s == "" {
		return s
	}
	return bold + Gradient(s, stops...)
}

// Color paints s a single 24-bit color.
func Color(s string, c RGB) string { return wrap(c.seq(), s) }

// Sample returns the brand-gradient color at position t in [0,1] — handy for
// tinting a sequence of single glyphs (a ribbon, a spinner frame).
func Sample(t float64) RGB { return colorAt(BrandGradient, t) }
