package style

import (
	"sort"
	"strings"
)

// Theme re-skins the brand palette: the accent (the Red role — the é, prompt,
// halts), the success/added color, the gray and white text tones, and the
// gradient sweep. NO_COLOR still flattens everything regardless of theme.
type Theme struct {
	Accent   RGB
	Success  RGB
	Gray     RGB
	White    RGB
	Gradient []RGB
}

var themes = map[string]Theme{
	"coral": { // the default — coral-red over a warm sweep
		Accent: RGB{229, 72, 77}, Success: RGB{120, 200, 120}, Gray: RGB{138, 138, 138}, White: RGB{237, 237, 237},
		Gradient: []RGB{{229, 72, 77}, {255, 121, 99}, {255, 179, 128}},
	},
	"mono": { // grayscale — calm, distraction-free
		Accent: RGB{205, 205, 205}, Success: RGB{170, 170, 170}, Gray: RGB{120, 120, 120}, White: RGB{236, 236, 236},
		Gradient: []RGB{{170, 170, 170}, {205, 205, 205}, {240, 240, 240}},
	},
	"ocean": { // cyan/blue
		Accent: RGB{56, 189, 248}, Success: RGB{52, 211, 153}, Gray: RGB{120, 140, 160}, White: RGB{226, 240, 250},
		Gradient: []RGB{{14, 165, 233}, {56, 189, 248}, {125, 211, 252}},
	},
	"matrix": { // phosphor green
		Accent: RGB{80, 250, 123}, Success: RGB{80, 250, 123}, Gray: RGB{95, 140, 105}, White: RGB{210, 255, 220},
		Gradient: []RGB{{0, 200, 80}, {80, 250, 123}, {180, 255, 190}},
	},
	"grape": { // violet
		Accent: RGB{167, 139, 250}, Success: RGB{134, 239, 172}, Gray: RGB{150, 140, 170}, White: RGB{235, 230, 245},
		Gradient: []RGB{{139, 92, 246}, {167, 139, 250}, {216, 180, 254}},
	},
}

// CurrentTheme is the active theme name.
var CurrentTheme = "coral"

// ApplyTheme swaps the live palette to the named theme, returning false for an
// unknown name (palette unchanged). Applied once at startup (config / env) or
// live via /theme; all rendering reads these vars, so it takes effect at once.
func ApplyTheme(name string) bool {
	t, ok := themes[name]
	if !ok {
		return false
	}
	RedRGB, GreenRGB, GrayRGB, WhiteRGB = t.Accent, t.Success, t.Gray, t.White
	BrandGradient = t.Gradient
	CurrentTheme = name
	return true
}

// ThemeNames returns the available theme names, sorted.
func ThemeNames() []string {
	out := make([]string, 0, len(themes))
	for n := range themes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ThemeSwatch renders a small colored preview of a theme (its gradient + accent
// and success dots), or "" when styling is off / the name is unknown.
func ThemeSwatch(name string) string {
	t, ok := themes[name]
	if !ok || !Enabled {
		return ""
	}
	var b strings.Builder
	for _, c := range t.Gradient {
		b.WriteString(Color("▰", c))
	}
	b.WriteString(" " + Color("●", t.Accent) + Color("●", t.Success))
	return b.String()
}
