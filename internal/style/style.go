// Package style renders Cliche's brand palette (red / black / white) as ANSI
// truecolor escapes. It is zero-dependency and auto-disables on a non-TTY or
// when NO_COLOR is set, so piped/CI output stays plain.
package style

import (
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
)

func wrap(seq, s string) string {
	if !Enabled || s == "" {
		return s
	}
	return seq + s + reset
}

// Hyperlink renders text as an OSC 8 terminal hyperlink pointing at url, so
// terminals that support it (Windows Terminal, iTerm2, VS Code, kitty, WezTerm,
// GNOME Terminal, …) make `text` clickable. Like color, it is gated on Enabled:
// when output is piped, NO_COLOR is set, or the terminal is plain, it returns
// text unchanged. The escape carries zero display width — Width() already skips
// OSC sequences — so it never disturbs alignment. text may itself be colored.
func Hyperlink(text, url string) string {
	// A URL with a control byte could break out of the escape — fall back to
	// plain text rather than emit something malformed.
	if !Enabled || url == "" || strings.ContainsAny(url, "\x1b\x07\n\r") {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// LinkURL makes a compact, possibly scheme-less and prose-decorated location
// clickable: it links only the leading URL token (up to the first space),
// leaving any trailing description plain, and prepends https:// when no scheme
// is present. E.g. "console.anthropic.com → API keys" links the host and keeps
// " → API keys" as text. The visible text is unchanged; only the token becomes
// a hyperlink (when Enabled).
func LinkURL(s string) string {
	if !Enabled || s == "" {
		return s
	}
	tok, rest := s, ""
	if i := strings.IndexByte(s, ' '); i >= 0 {
		tok, rest = s[:i], s[i:]
	}
	if tok == "" {
		return s
	}
	url := tok
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	return Hyperlink(tok, url) + rest
}

// boldColor wraps s in bold + a tier-quantized color.
func boldColor(s string, c RGB) string {
	if !Enabled || s == "" {
		return s
	}
	return bold + c.seq() + s + reset
}

// The brand palette routes through RGB.seq(), so every accent quantizes to the
// terminal's color tier (truecolor → 256 → 16) rather than emitting a fixed
// truecolor escape that a limited terminal would ignore.

// Red is the accent color (halts, the é, flagged verdicts, the prompt).
func Red(s string) string { return Color(s, RedRGB) }

// White is primary text.
func White(s string) string { return Color(s, WhiteRGB) }

// Gray is secondary/label text.
func Gray(s string) string { return Color(s, GrayRGB) }

// Dim is de-emphasized text.
func Dim(s string) string { return wrap(dim, s) }

// Bold emphasizes without changing color.
func Bold(s string) string { return wrap(bold, s) }

// BoldWhite / BoldRed are the wordmark weights.
func BoldWhite(s string) string { return boldColor(s, WhiteRGB) }
func BoldRed(s string) string   { return boldColor(s, RedRGB) }

// RGB is a 24-bit color used for gradients.
type RGB struct{ R, G, B int }

// BrandGradient is the signature coral sweep (deep red → coral → warm peach)
// used for the wordmark and rules. Cohesive with the red accent, but alive.
var BrandGradient = []RGB{{229, 72, 77}, {255, 121, 99}, {255, 179, 128}}

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
