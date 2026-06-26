package style

import (
	"image/color"
	"strings"
	"testing"
)

func TestDetectCapsFromEnv(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		wantImage ImageProto
		wantSync  bool
	}{
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"}, ImageKitty, true},
		{"iterm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, ImageITerm2, true},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, ImageITerm2, true},
		{"windows-terminal", map[string]string{"WT_SESSION": "abc"}, ImageNone, true},
		{"unknown", map[string]string{}, ImageNone, false}, // conservative: safe fallback
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, k := range []string{"KITTY_WINDOW_ID", "TERM_PROGRAM", "WT_SESSION", "TERM"} {
				t.Setenv(k, "")
			}
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			got := detectCaps()
			if got.Image != c.wantImage {
				t.Errorf("image = %v, want %v", got.Image, c.wantImage)
			}
			if got.SyncOutput != c.wantSync {
				t.Errorf("sync = %v, want %v", got.SyncOutput, c.wantSync)
			}
		})
	}
}

func TestRenderImageDispatchesByProtocol(t *testing.T) {
	oldE, oldC := Enabled, caps
	defer func() { Enabled, caps = oldE, oldC }()
	Enabled = true
	img := solid(8, 8, color.RGBA{10, 20, 30, 255})

	caps = Caps{Image: ImageKitty}
	if k := RenderImage(img, 10); !strings.HasPrefix(k, "\x1b_Gf=100,a=T") {
		t.Fatalf("kitty path should emit the kitty graphics escape, got prefix %q", first(k, 24))
	}
	caps = Caps{Image: ImageITerm2}
	if it := RenderImage(img, 10); !strings.Contains(it, "\x1b]1337;File=inline=1") {
		t.Fatalf("iterm2 path should emit the iTerm2 inline-image escape")
	}
	caps = Caps{Image: ImageNone}
	if h := RenderImage(img, 10); !strings.Contains(h, "▀") {
		t.Fatalf("no-protocol path should fall back to half-block ▀")
	}
}

func first(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
