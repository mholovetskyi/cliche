package style

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.Set(x, y, c)
		}
	}
	return m
}

func TestRenderImageHalfBlock(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()

	Enabled = true
	img := solid(8, 8, color.RGBA{200, 40, 40, 255})
	out := RenderImage(img, 4)
	if out == "" {
		t.Fatal("expected rendered output")
	}
	if !strings.Contains(out, "▀") {
		t.Fatalf("expected half-block glyphs, got %q", out)
	}
	// 4 cols wide; aspect-square 8x8 → 4 pixel rows → 2 text rows.
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 text rows for a square image at 4 cols, got %d", len(lines))
	}
	for _, ln := range lines {
		if w := Width(ln); w != 4 {
			t.Fatalf("each row should be 4 cells wide, got %d", w)
		}
	}
}

func TestRenderImageDegradesAndGuards(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()

	img := solid(4, 4, color.RGBA{10, 200, 10, 255})
	Enabled = false
	if RenderImage(img, 8) != "" {
		t.Fatal("must be empty when styling is off")
	}
	Enabled = true
	if RenderImage(nil, 8) != "" {
		t.Fatal("nil image must render empty, not panic")
	}
	if RenderImage(img, 0) != "" {
		t.Fatal("non-positive width must render empty")
	}
}
