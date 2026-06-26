package style

import "strings"

// SemanticRamp runs sage (healthy) → amber (warning) → coral (alarm). Meters that
// represent CONSUMPTION — spend vs cap, context used, governor pressure — color
// by their fill LEVEL through this ramp, so a near-full bar reads alarm-red, not
// merely "long". It is the visual embodiment of the Trust Kernel: you can see the
// run heat up. Quantizes through the tier path and flattens to nothing under
// NO_COLOR (the caller always pairs a meter with a numeric value).
var SemanticRamp = []RGB{{120, 200, 120}, {235, 185, 80}, {229, 72, 77}}

// RampAt samples SemanticRamp at frac in [0,1]; frac is the fill level. NaN/<0
// clamp to 0, >1 clamps to 1.
func RampAt(frac float64) RGB {
	if frac != frac || frac < 0 { // NaN or negative
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return colorAt(SemanticRamp, frac)
}

// eighthBlocks indexes the left-growing partial block glyphs by eighths of a
// cell: index 0 is empty, 8 is a full block. They render single-width on every
// target terminal (Windows Terminal, conhost, xterm, …), so a bar stays exactly
// `width` cells while gaining 8× the resolution of a whole-cell bar.
var eighthBlocks = []rune(" ▏▎▍▌▋▊▉█")

// sparkBars are the eight bar heights for a one-line chart.
var sparkBars = []rune("▁▂▃▄▅▆▇█")

// Sparkline renders a value series as a compact inline chart, normalized across
// the series (min → shortest bar, max → tallest), tinted left→right with the
// brand gradient. A flat/empty series renders as a baseline. Returns "" when
// styling is off so the caller can fall back to a numeric summary.
func Sparkline(values []float64) string {
	if !Enabled || len(values) == 0 {
		return ""
	}
	lo, hi := values[0], values[0]
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	span := hi - lo
	denom := float64(len(values) - 1)
	if denom < 1 {
		denom = 1
	}
	var b strings.Builder
	for i, v := range values {
		frac := 0.0
		if span > 0 {
			frac = (v - lo) / span
		}
		idx := int(frac*float64(len(sparkBars)-1) + 0.5)
		b.WriteString(Color(string(sparkBars[idx]), Sample(float64(i)/denom)))
	}
	return b.String()
}
