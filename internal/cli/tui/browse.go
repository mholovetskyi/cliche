package tui

import (
	"io"
	"strings"

	"github.com/mholovetskyi/cliche/internal/cli/keydec"
	"github.com/mholovetskyi/cliche/internal/cli/rawmode"
	"github.com/mholovetskyi/cliche/internal/style"
)

// Item is one row of a Browse list: a Label (shown in the left pane) and a
// Preview (lines shown in the right pane when it's selected).
type Item struct {
	Label   string
	Preview []string
}

// cell truncates s to a visible width of w and pads it back out to w, so styled
// (ANSI) text composes into fixed-width columns correctly.
func cell(s string, w int) string {
	if w < 0 {
		w = 0
	}
	return style.Pad(style.Truncate(s, w), w)
}

// renderFrame builds exactly `height` screen lines for the two-pane browser:
// a header row, then a split body (left = scrollable list, right = the selected
// item's preview), then a footer of keys. Pure — unit-tested.
func renderFrame(l *List, items []Item, width, height int, header string) []string {
	leftW := width / 3
	if leftW < 16 {
		leftW = 16
	}
	if leftW > 40 {
		leftW = 40
	}
	if leftW > width-4 {
		leftW = width - 4
	}
	rightW := width - leftW - 3 // " │ "
	if rightW < 0 {
		rightW = 0
	}
	contentRows := height - 2
	if contentRows < 1 {
		contentRows = 1
	}
	l.SetHeight(contentRows)

	var preview []string
	if l.Sel >= 0 && l.Sel < len(items) {
		preview = items[l.Sel].Preview
	}

	lines := make([]string, 0, height)
	lines = append(lines, style.BoldWhite(cell(header, width)))

	start, _ := l.Window()
	for i := 0; i < contentRows; i++ {
		idx := start + i
		left := ""
		if idx < len(items) {
			lab := style.Truncate(items[idx].Label, leftW-2)
			if idx == l.Sel {
				left = style.BoldGreen("▸ " + lab)
			} else {
				left = "  " + style.White(lab)
			}
		}
		right := ""
		if i < len(preview) {
			right = style.Gray(style.Truncate(preview[i], rightW))
		}
		lines = append(lines, cell(left, leftW)+style.Gray(" │ ")+right)
	}
	lines = append(lines, style.Dim(cell("↑↓/wheel move · click select · enter open · q quit", width)))
	return lines
}

// Browse runs the full-screen two-pane browser over a decoder already reading a
// raw-mode terminal: a scrollable, selectable list on the left and the selected
// item's preview on the right. ↑/↓ (j/k) and the mouse wheel scroll, a click
// selects a row, Enter opens the selection, q/Esc cancels. It owns the alternate
// screen + mouse reporting (restored on return). Returns the chosen index + true,
// or (-1, false) on cancel.
func Browse(dec *keydec.Decoder, out io.Writer, width, height int, header string, items []Item) (int, bool) {
	if len(items) == 0 || dec == nil {
		return -1, false
	}
	if width < 8 {
		width = 8
	}
	if height < 4 {
		height = 4
	}
	rawmode.EnterAlt(out)
	rawmode.EnableMouse(out)
	defer func() {
		rawmode.DisableMouse(out)
		rawmode.LeaveAlt(out)
	}()

	l := NewList(len(items))
	draw := func() {
		frame := renderFrame(l, items, width, height, header)
		var b strings.Builder
		b.WriteString("\x1b[H") // cursor home
		for i, ln := range frame {
			b.WriteString("\x1b[K")
			b.WriteString(ln)
			if i < len(frame)-1 {
				b.WriteString("\r\n")
			}
		}
		io.WriteString(out, b.String())
	}

	draw()
	for {
		k, err := dec.ReadKey()
		if err != nil {
			return -1, false
		}
		switch k.Type {
		case keydec.KeyEnter:
			return l.Sel, true
		case keydec.KeyEsc, keydec.KeyCtrlC:
			return -1, false
		case keydec.KeyUp, keydec.KeyCtrlP:
			l.Up()
		case keydec.KeyDown, keydec.KeyCtrlN:
			l.Down()
		case keydec.KeyRune:
			switch k.Rune {
			case 'q', 'Q':
				return -1, false
			case 'k':
				l.Up()
			case 'j':
				l.Down()
			}
		case keydec.KeyMouse:
			switch k.MouseButton {
			case keydec.MouseWheelUp:
				l.Up()
			case keydec.MouseWheelDown:
				l.Down()
			case keydec.MouseLeft:
				if k.MousePress {
					l.ClickRow(k.MouseY - 2) // header occupies screen row 1; list starts at row 2
				}
			}
		}
		draw()
	}
}
