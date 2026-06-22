package style

import (
	"fmt"
	"os"
	"strings"
)

// Tier is the color fidelity of the terminal. Every color quantizes to the best
// escape the tier supports, so the brand palette survives — recognizably — from
// truecolor down to a 16-color console, instead of vanishing or garbling.
type Tier int

const (
	TierNone      Tier = iota // no color
	TierANSI16                // 16 SGR colors
	TierColor256              // 256-color cube
	TierTruecolor             // 24-bit
)

// tier is detected once at init. Tests may override it.
var tier = detectTier()

// detectTier is optimistic: it stays at truecolor (matching modern terminals,
// including Windows Terminal) unless there is positive evidence of a more
// limited terminal, so the common case is never downgraded. CLICHE_FORCE_COLOR
// pins truecolor for piping into a capable pager.
func detectTier() Tier {
	if os.Getenv("CLICHE_FORCE_COLOR") != "" {
		return TierTruecolor
	}
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return TierTruecolor
	}
	term := os.Getenv("TERM")
	switch {
	case strings.Contains(term, "256color"):
		return TierColor256
	case strings.HasSuffix(term, "-16color"), term == "ansi", term == "linux":
		return TierANSI16
	default:
		return TierTruecolor
	}
}

// seq returns the best foreground escape for the active tier.
func (c RGB) seq() string {
	switch tier {
	case TierColor256:
		return fmt.Sprintf("\x1b[38;5;%dm", c.cube256())
	case TierANSI16:
		return fmt.Sprintf("\x1b[%dm", c.ansi16(false))
	default:
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
	}
}

// bgSeq returns the best background escape for the active tier.
func (c RGB) bgSeq() string {
	switch tier {
	case TierColor256:
		return fmt.Sprintf("\x1b[48;5;%dm", c.cube256())
	case TierANSI16:
		return fmt.Sprintf("\x1b[%dm", c.ansi16(true))
	default:
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c.R, c.G, c.B)
	}
}

// cube256 maps an RGB to the xterm 256-color palette (6×6×6 cube + grayscale).
func (c RGB) cube256() int {
	if c.R == c.G && c.G == c.B {
		switch {
		case c.R < 8:
			return 16
		case c.R > 248:
			return 231
		default:
			return 232 + (c.R-8)*24/247
		}
	}
	q := func(v int) int { return (v*5 + 127) / 255 }
	return 16 + 36*q(c.R) + 6*q(c.G) + q(c.B)
}

// ansi16Table approximates the 16 base SGR colors for nearest-color matching.
var ansi16Table = []struct {
	rgb  RGB
	code int
}{
	{RGB{0, 0, 0}, 30}, {RGB{205, 49, 49}, 31}, {RGB{13, 188, 121}, 32}, {RGB{229, 229, 16}, 33},
	{RGB{36, 114, 200}, 34}, {RGB{188, 63, 188}, 35}, {RGB{17, 168, 205}, 36}, {RGB{229, 229, 229}, 37},
	{RGB{102, 102, 102}, 90}, {RGB{241, 76, 76}, 91}, {RGB{35, 209, 139}, 92}, {RGB{245, 245, 67}, 93},
	{RGB{59, 142, 234}, 94}, {RGB{214, 112, 214}, 95}, {RGB{41, 184, 219}, 96}, {RGB{255, 255, 255}, 97},
}

// ansi16 returns the nearest 16-color SGR code (+10 for background).
func (c RGB) ansi16(bg bool) int {
	best, bestD := 37, 1<<31-1
	for _, e := range ansi16Table {
		dr, dg, db := c.R-e.rgb.R, c.G-e.rgb.G, c.B-e.rgb.B
		if d := dr*dr + dg*dg + db*db; d < bestD {
			best, bestD = e.code, d
		}
	}
	if bg {
		best += 10
	}
	return best
}
