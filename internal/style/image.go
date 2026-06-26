package style

import (
	"image"
	"image/color"
	"strings"
)

// RenderImage turns a decoded image into colored terminal text using the
// upper-half-block technique: each character cell is "▀" with its foreground set
// to the TOP source pixel and its background to the BOTTOM one, so a single text
// row carries two pixel rows. The image is box-averaged down to fit maxCols cells
// wide (aspect preserved), and every color quantizes through the tier path — so a
// real photo shows up in truecolor on Windows Terminal today, recognizably in a
// 256-color console, and not at all under NO_COLOR. No terminal graphics protocol
// or capability probe is required; this is the universal path that works
// everywhere color does. Returns "" when styling is off or the image is empty.
func RenderImage(img image.Image, maxCols int) string {
	if !Enabled || img == nil || maxCols < 1 {
		return ""
	}
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return ""
	}
	cols := maxCols
	if cols > iw {
		cols = iw // never upscale past the source width
	}
	rows := cols * ih / iw // pixel rows, aspect-preserved
	if rows < 2 {
		rows = 2
	}
	if rows%2 == 1 {
		rows++ // even: two pixel rows per text row
	}
	px := resize(img, cols, rows)

	var sb strings.Builder
	for y := 0; y < rows; y += 2 {
		for x := 0; x < cols; x++ {
			top := rgbAt(px, x, y)
			bot := rgbAt(px, x, y+1)
			sb.WriteString(top.seq())   // foreground = upper pixel
			sb.WriteString(bot.bgSeq()) // background = lower pixel
			sb.WriteRune('▀')
		}
		sb.WriteString(reset)
		if y+2 < rows {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func rgbAt(m *image.RGBA, x, y int) RGB {
	c := m.RGBAAt(x, y)
	return RGB{R: int(c.R), G: int(c.G), B: int(c.B)}
}

// resize box-averages src down (or 1:1) to w×h. Averaging beats nearest-neighbor
// for a photo shrunk to terminal scale; stdlib-only.
func resize(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy0 := b.Min.Y + y*sh/h
		sy1 := b.Min.Y + (y+1)*sh/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < w; x++ {
			sx0 := b.Min.X + x*sw/w
			sx1 := b.Min.X + (x+1)*sw/w
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var rs, gs, bs, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, bl, _ := src.At(sx, sy).RGBA() // 16-bit per channel
					rs += uint64(r >> 8)
					gs += uint64(g >> 8)
					bs += uint64(bl >> 8)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			dst.Set(x, y, color.RGBA{R: uint8(rs / n), G: uint8(gs / n), B: uint8(bs / n), A: 255})
		}
	}
	return dst
}
