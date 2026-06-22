package style

import (
	"strings"
	"testing"
)

func TestTierQuantization(t *testing.T) {
	old, oldTier := Enabled, tier
	Enabled = true
	defer func() { Enabled, tier = old, oldTier }()

	coral := RedRGB
	tier = TierTruecolor
	if got := coral.seq(); got != "\x1b[38;2;229;72;77m" {
		t.Errorf("truecolor seq = %q", got)
	}
	tier = TierColor256
	if got := coral.seq(); !strings.HasPrefix(got, "\x1b[38;5;") {
		t.Errorf("256 seq should be palette-indexed, got %q", got)
	}
	tier = TierANSI16
	// Coral is reddish → nearest base color is a red (31 or 91).
	if got := coral.ansi16(false); got != 31 && got != 91 {
		t.Errorf("coral should map to a red SGR code, got %d", got)
	}
	if got := coral.ansi16(true); got != 41 && got != 101 {
		t.Errorf("coral background should be a red bg code, got %d", got)
	}
	// Every tier still produces a non-empty escape so the brand never vanishes.
	for _, ti := range []Tier{TierTruecolor, TierColor256, TierANSI16} {
		tier = ti
		if Red("x") == "x" {
			t.Errorf("tier %d dropped color", ti)
		}
	}
}

func TestCube256Grayscale(t *testing.T) {
	if c := (RGB{0, 0, 0}).cube256(); c != 16 {
		t.Errorf("black cube = %d, want 16", c)
	}
	if c := (RGB{255, 255, 255}).cube256(); c != 231 {
		t.Errorf("white cube = %d, want 231", c)
	}
	mid := (RGB{128, 128, 128}).cube256()
	if mid < 232 || mid > 255 {
		t.Errorf("mid gray should land in the grayscale ramp, got %d", mid)
	}
}
