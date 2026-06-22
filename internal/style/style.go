// Package style renders Cliche's brand palette (red / black / white) as ANSI
// truecolor escapes. It is zero-dependency and auto-disables on a non-TTY or
// when NO_COLOR is set, so piped/CI output stays plain.
package style

import "os"

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
