package style

import (
	"os"
	"strings"
)

// ImageProto is the best inline-image transport the terminal supports.
type ImageProto int

const (
	ImageNone   ImageProto = iota // no protocol → half-block truecolor raster (works everywhere)
	ImageKitty                    // kitty graphics protocol
	ImageITerm2                   // iTerm2 / WezTerm inline-image protocol
)

// Caps is what the terminal can do, detected once from environment signals. It is
// CONSERVATIVE: a capability is claimed only on positive evidence, so an unknown
// terminal falls back to the universally-safe path (truecolor half-blocks, no
// premium escape, append-only output). This is deliberately env-only — a runtime
// DA1/CPR probe would refine it but requires a terminal round-trip that can hang
// a dumb terminal; the common terminals are identified safely without one.
type Caps struct {
	Truecolor  bool
	SyncOutput bool       // supports synchronized output (?2026) → flicker-free in-place regions
	Image      ImageProto // best inline-image transport
	TermName   string     // friendly name, for `/status` etc.
}

// caps is detected at init; tests may override it.
var caps = detectCaps()

// Capabilities returns the detected terminal capabilities.
func Capabilities() Caps { return caps }

func detectCaps() Caps {
	c := Caps{Truecolor: tier == TierTruecolor}
	term := strings.ToLower(os.Getenv("TERM"))
	prog := os.Getenv("TERM_PROGRAM")
	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty"):
		c.Image, c.SyncOutput, c.TermName = ImageKitty, true, "kitty"
	case prog == "WezTerm":
		c.Image, c.SyncOutput, c.TermName = ImageITerm2, true, "WezTerm"
	case prog == "iTerm.app":
		c.Image, c.SyncOutput, c.TermName = ImageITerm2, true, "iTerm2"
	case os.Getenv("WT_SESSION") != "":
		// Windows Terminal: synchronized output, but no inline-image protocol
		// (half-block is the path) — the primary target, fully supported.
		c.SyncOutput, c.TermName = true, "Windows Terminal"
	case prog == "vscode":
		c.SyncOutput, c.TermName = true, "VS Code"
	case prog != "":
		c.TermName = prog
	}
	return c
}
