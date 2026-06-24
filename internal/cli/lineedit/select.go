package lineedit

import (
	"fmt"
	"io"
	"strings"

	"github.com/mholovetskyi/cliche/internal/cli/keydec"
	"github.com/mholovetskyi/cliche/internal/style"
)

// SelectItem is one row in a Select picker. Label is the value (returned by
// index, shown prominently); Desc is dim secondary text. Both are matched by the
// type-to-filter search.
type SelectItem struct {
	Label string
	Desc  string
}

// Select runs an interactive picker over items: typing filters (case-insensitive
// substring on Label+Desc), ↑/↓ (or Ctrl-P/N) move the highlight, Enter chooses,
// Esc/Ctrl-C cancels. It returns the chosen index into the ORIGINAL items slice
// and ok=true, or (-1, false) on cancel / empty. It reuses the editor's decoder
// and assumes the terminal is already in raw mode (the caller toggles it, the
// same way ReadLine is driven). The picker draws an inline block and erases it on
// exit, leaving the screen clean for the caller to print the outcome.
func (e *Editor) Select(header string, items []SelectItem) (int, bool) {
	var filter []rune
	sel, drawn := 0, 0

	matches := func() []int {
		f := strings.ToLower(string(filter))
		var out []int
		for i, it := range items {
			if f == "" || strings.Contains(strings.ToLower(it.Label+" "+it.Desc), f) {
				out = append(out, i)
			}
		}
		return out
	}

	render := func() {
		fl := matches()
		if sel >= len(fl) {
			sel = len(fl) - 1
		}
		if sel < 0 {
			sel = 0
		}
		var b strings.Builder
		if drawn > 0 { // climb to the top of the previously drawn block, then erase it all
			fmt.Fprintf(&b, "\x1b[%dA", drawn-1)
		}
		b.WriteString("\r\x1b[J")
		b.WriteString("  " + style.BoldWhite(header))
		b.WriteString("\r\n  " + style.Gray("search ") + style.White(string(filter)) + style.Dim("▏"))
		lines := 2

		n := len(fl)
		if n > maxMenuRows {
			n = maxMenuRows
		}
		start := 0
		if sel >= n { // scroll the window so the selection stays visible
			start = sel - n + 1
		}
		if max := len(fl) - n; start > max {
			start = max
		}
		for i := 0; i < n; i++ {
			it := items[fl[start+i]]
			b.WriteString("\r\n")
			if start+i == sel {
				b.WriteString("  " + style.BoldGreen("›") + " " + style.BoldGreen(style.Pad(it.Label, 22)) + style.Gray(it.Desc))
			} else {
				b.WriteString("    " + style.White(style.Pad(it.Label, 22)) + style.Gray(it.Desc))
			}
			lines++
		}
		if len(fl) == 0 {
			b.WriteString("\r\n  " + style.Gray("(no matches)"))
			lines++
		}
		io.WriteString(e.out, b.String())
		drawn = lines
	}

	erase := func() {
		if drawn > 0 {
			fmt.Fprintf(e.out, "\x1b[%dA\r\x1b[J", drawn-1)
			drawn = 0
		}
	}

	render()
	for {
		k, err := e.dec.ReadKey()
		if err != nil {
			erase()
			return -1, false
		}
		switch k.Type {
		case keydec.KeyEnter:
			fl := matches()
			erase()
			if len(fl) == 0 {
				return -1, false
			}
			return fl[sel], true
		case keydec.KeyEsc, keydec.KeyCtrlC:
			erase()
			return -1, false
		case keydec.KeyUp, keydec.KeyCtrlP:
			if sel > 0 {
				sel--
			}
		case keydec.KeyDown, keydec.KeyCtrlN:
			sel++ // clamped in render
		case keydec.KeyBackspace:
			if len(filter) > 0 {
				filter = filter[:len(filter)-1]
				sel = 0
			}
		case keydec.KeyRune:
			filter = append(filter, k.Rune)
			sel = 0
		case keydec.KeyPaste:
			filter = append(filter, []rune(k.Text)...)
			sel = 0
		}
		render()
	}
}
