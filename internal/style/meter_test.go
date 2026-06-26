package style

import "testing"

func TestSparklineWidthAndDegrade(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()

	Enabled = true
	// One single-width bar per value.
	if w := Width(Sparkline([]float64{1, 3, 2, 8, 4})); w != 5 {
		t.Fatalf("sparkline width = %d, want 5", w)
	}
	if Sparkline(nil) != "" {
		t.Fatal("empty series renders empty")
	}
	Enabled = false
	if Sparkline([]float64{1, 2}) != "" {
		t.Fatal("sparkline must be empty when styling is off")
	}
}

func TestRampAtLevels(t *testing.T) {
	lo, mid, hi := RampAt(0), RampAt(0.5), RampAt(1)
	if lo.G <= lo.R {
		t.Fatalf("low level should read green (G>R), got %+v", lo)
	}
	if hi.R <= hi.G {
		t.Fatalf("high level should read red (R>G), got %+v", hi)
	}
	// Mid is amber: warmer than sage (more red) and its green sits between the
	// sage high and the coral low — i.e. the ramp cools from green to red.
	if mid.R <= lo.R {
		t.Fatalf("mid (amber) should be warmer than low (sage), got %+v", mid)
	}
	if !(hi.G < mid.G && mid.G < lo.G) {
		t.Fatalf("green should descend sage→amber→coral, got lo.G=%d mid.G=%d hi.G=%d", lo.G, mid.G, hi.G)
	}
	// Out-of-range clamps rather than panics.
	_ = RampAt(-1)
	_ = RampAt(2)
}
