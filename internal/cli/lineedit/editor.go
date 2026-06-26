// Package lineedit is an I/O-injected raw-mode line editor: it reads decoded key
// events from any io.Reader and writes render bytes to any io.Writer, so all of
// its behavior — editing, the live "/" dropdown, ↑/↓ history, readline hotkeys —
// is exercised by unit tests with synthetic byte streams (no TTY needed). The
// platform raw-mode toggle and the cooked-mode fallback live in package cli; this
// package is pure logic. It assumes the terminal is in raw mode (OPOST off), so
// every line break it emits is an explicit "\r\n".
package lineedit

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mholovetskyi/cliche/internal/cli/keydec"
	"github.com/mholovetskyi/cliche/internal/style"
)

// ErrInterrupted is returned by ReadLine when the user presses Ctrl-C.
var ErrInterrupted = errors.New("interrupted")

// maxMenuRows caps the dropdown height so it can't flood a short terminal.
const maxMenuRows = 8

// Editor holds the line state and renders in place. The decoder persists across
// ReadLine calls so buffered read-ahead (e.g. a fast paste) isn't lost.
type Editor struct {
	dec           *keydec.Decoder
	out           io.Writer
	history       *History
	menu          *slashMenu
	prompt        string
	promptW       int
	buf           []rune
	cursor        int
	rendered      int  // dropdown rows currently drawn below the input line
	cols          int  // terminal width in cells (for wrap-aware redraw); 0 → 80
	prevCursorRow int  // cursor's physical row within the block at the last render
	ghostOff      bool // suppress the inline autosuggestion (during commit's final redraw)

	// Footer, if set, is a hint line drawn just below the input whenever the "/"
	// dropdown is closed (the dropdown takes its place when open). It must include
	// its own leading indent/styling — render writes it verbatim.
	Footer string

	// CycleMode, if set, is invoked on Shift-Tab to cycle the permission mode; it
	// returns the new prompt (and its display width) so the chevron updates live.
	CycleMode func() (prompt string, promptW int)
}

// NewEditor builds an editor reading keys from in and rendering to out, with the
// given command table (for the dropdown) and history.
func NewEditor(in io.Reader, out io.Writer, cmds []Command, hist *History) *Editor {
	if hist == nil {
		hist = NewHistory(nil)
	}
	return &Editor{
		dec:     keydec.NewDecoder(in),
		out:     out,
		history: hist,
		menu:    newSlashMenu(cmds),
		cols:    80, // sensible default until SetWidth is called
	}
}

// Decoder exposes the editor's key decoder so a full-screen mode (tui.Browse)
// can read from the SAME buffered source — avoiding a second reader over stdin
// that would strand bytes between the two.
func (e *Editor) Decoder() *keydec.Decoder { return e.dec }

// SetWidth sets the terminal width (in cells) used for wrap-aware redraw. Called
// before each ReadLine so a window resize between prompts is picked up.
func (e *Editor) SetWidth(cols int) {
	if cols > 0 {
		e.cols = cols
	}
}

// ReadLine reads one logical line, rendering the prompt (promptW display cells
// wide, for cursor math). Returns the line on Enter, ErrInterrupted on Ctrl-C,
// io.EOF on Ctrl-D at an empty line.
func (e *Editor) ReadLine(prompt string, promptW int) (string, error) {
	e.prompt, e.promptW = prompt, promptW
	e.buf, e.cursor, e.rendered, e.prevCursorRow = e.buf[:0], 0, 0, 0
	e.menu.reset()
	e.menu.update("")
	e.render()
	for {
		k, err := e.dec.ReadKey()
		if err != nil {
			return "", err // io.EOF at a clean end
		}
		switch k.Type {
		case keydec.KeyEnter:
			// While the menu is open, Enter COMPLETES to the selection — unless the
			// buffer already equals it (an exact command), in which case it submits.
			if e.menu.open {
				if c, ok := e.menu.completion(); ok && string(e.buf) != c {
					e.setBuf(c)
					break
				}
			}
			// Backslash continuation: a trailing "\" becomes a newline and editing
			// continues (composes a multi-line prompt), instead of submitting.
			if n := len(e.buf); n > 0 && e.buf[n-1] == '\\' {
				e.buf[n-1] = '\n'
				e.cursor = len(e.buf)
				break
			}
			line := string(e.buf)
			e.commit()
			e.history.Add(line)
			return line, nil
		case keydec.KeyCtrlC:
			e.commit()
			return "", ErrInterrupted
		case keydec.KeyCtrlD:
			if len(e.buf) == 0 {
				e.commit()
				return "", io.EOF
			}
			e.deleteRight()
		case keydec.KeyTab:
			if c, ok := e.menu.completion(); ok {
				e.setBuf(c)
			}
		case keydec.KeyShiftTab:
			if e.CycleMode != nil {
				e.prompt, e.promptW = e.CycleMode()
			}
		case keydec.KeyRune:
			e.insertRune(k.Rune)
		case keydec.KeyPaste:
			for _, r := range k.Text { // a multi-line paste keeps its newlines in the buffer
				e.insertRune(r)
			}
		case keydec.KeyBackspace:
			e.deleteLeft()
		case keydec.KeyDelete:
			e.deleteRight()
		case keydec.KeyLeft, keydec.KeyCtrlB:
			if e.cursor > 0 {
				e.cursor--
			}
		case keydec.KeyRight, keydec.KeyCtrlF:
			if e.cursor < len(e.buf) {
				e.cursor++
			} else if g := e.ghost(); g != "" {
				e.setBuf(string(e.buf) + g) // accept the inline autosuggestion
			}
		case keydec.KeyHome, keydec.KeyCtrlA:
			e.cursor = 0
		case keydec.KeyEnd, keydec.KeyCtrlE:
			e.cursor = len(e.buf)
		case keydec.KeyCtrlU:
			e.killToStart()
		case keydec.KeyCtrlK:
			e.killToEnd()
		case keydec.KeyCtrlW:
			e.deleteWordLeft()
		case keydec.KeyCtrlL:
			io.WriteString(e.out, "\x1b[2J\x1b[H")
			e.rendered, e.prevCursorRow = 0, 0 // screen cleared: don't climb into it on the next redraw
		case keydec.KeyUp, keydec.KeyCtrlP:
			if e.menu.open {
				e.menu.up()
			} else {
				e.setBuf(e.history.Prev(string(e.buf)))
			}
		case keydec.KeyDown, keydec.KeyCtrlN:
			if e.menu.open {
				e.menu.down()
			} else {
				e.setBuf(e.history.Next())
			}
		}
		e.menu.update(string(e.buf))
		e.render()
	}
}

// ghost returns the inline autosuggestion: the dim completion of the current
// buffer from history, shown only when the cursor is at the end of a non-empty
// buffer and the "/" dropdown is closed. Right / Ctrl-F accepts it.
func (e *Editor) ghost() string {
	if e.ghostOff || e.menu.open || e.cursor != len(e.buf) || len(e.buf) == 0 {
		return ""
	}
	return e.history.Suggest(string(e.buf))
}

// setBuf replaces the whole buffer (history recall / completion) and parks the
// cursor at the end.
func (e *Editor) setBuf(s string) {
	e.buf = []rune(s)
	e.cursor = len(e.buf)
}

// ---- pure buffer mutators (cursor is a rune offset) ----

func (e *Editor) insertRune(r rune) {
	e.buf = append(e.buf, 0)
	copy(e.buf[e.cursor+1:], e.buf[e.cursor:])
	e.buf[e.cursor] = r
	e.cursor++
}

func (e *Editor) deleteLeft() {
	if e.cursor == 0 {
		return
	}
	e.buf = append(e.buf[:e.cursor-1], e.buf[e.cursor:]...)
	e.cursor--
}

func (e *Editor) deleteRight() {
	if e.cursor >= len(e.buf) {
		return
	}
	e.buf = append(e.buf[:e.cursor], e.buf[e.cursor+1:]...)
}

func (e *Editor) killToStart() {
	e.buf = append([]rune(nil), e.buf[e.cursor:]...)
	e.cursor = 0
}

func (e *Editor) killToEnd() { e.buf = e.buf[:e.cursor] }

func (e *Editor) deleteWordLeft() {
	i := e.cursor
	for i > 0 && e.buf[i-1] == ' ' {
		i--
	}
	for i > 0 && e.buf[i-1] != ' ' {
		i--
	}
	e.buf = append(e.buf[:i], e.buf[e.cursor:]...)
	e.cursor = i
}

// ---- rendering ----

// commit clears the dropdown and moves to a fresh line, so the submitted line
// stays and subsequent output starts clean.
func (e *Editor) commit() {
	e.menu.reset()
	f := e.Footer
	e.Footer = ""         // the committed line keeps no footer
	e.cursor = len(e.buf) // park at the end so the trailing newline lands BELOW the whole (possibly wrapped) line
	e.ghostOff = true     // the committed line shows only what was typed — no trailing suggestion
	e.render()            // wipes any dropdown/footer rows, leaves the cursor at the line end
	e.ghostOff = false
	e.Footer = f
	io.WriteString(e.out, "\r\n")
}

// render rewrites the input line in place and (re)draws or clears the dropdown
// below it, always leaving the cursor on the input line at the edit position. A
// full rewrite each keystroke (no incremental diffing) keeps it simple and
// robust; assertions in tests check visible content, not exact escapes.
func (e *Editor) render() {
	cols := e.cols
	if cols < 1 {
		cols = 80
	}
	var b strings.Builder

	// Return to the TOP-LEFT of the block drawn last time, then erase all of it in
	// one shot. \x1b[J (erase to end of screen) clears the input line even when it
	// wrapped across several physical rows, plus any dropdown rows below — the old
	// \r\x1b[K cleared only ONE row and corrupted a wrapped line on every redraw.
	if e.prevCursorRow > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", e.prevCursorRow)
	}
	b.WriteString("\r\x1b[J")

	// Rewrite the input line; the terminal wraps it across cols on its own. A
	// dim ghost (inline autosuggestion) trails the buffer — the cursor parks
	// BEFORE it, so it reads as a preview the user accepts with Right / Ctrl-F.
	ghost := e.ghost()
	b.WriteString(e.prompt)
	b.WriteString(displayLine(string(e.buf)))
	if ghost != "" {
		b.WriteString(style.Gray(displayLine(ghost)))
	}

	// Below the (wrapped) input: the "/" dropdown, or — when it's closed — the
	// footer hint line, so the box always has a bottom edge while idle.
	rows := e.menuRows()
	if len(rows) == 0 && e.Footer != "" {
		rows = []string{e.Footer}
	}
	for _, r := range rows {
		b.WriteString("\r\n")
		b.WriteString(r)
	}

	// Park the cursor at the edit position, addressed as (row, col) within the
	// block. Writing left the cursor on the bottom row; move up to the cursor row.
	// The ghost adds to the line's display width (so wrapping/row math is right),
	// but the cursor parks at the buffer end, before the ghost.
	inputCells := e.promptW + style.Width(displayLine(string(e.buf))) + style.Width(displayLine(ghost))
	cursorCells := e.promptW + style.Width(displayLine(string(e.buf[:e.cursor])))
	totalRows := physicalRows(inputCells, cols) + len(rows)
	cursorRow := cursorCells / cols
	cursorCol := cursorCells % cols
	if up := (totalRows - 1) - cursorRow; up > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", up)
	}
	b.WriteString("\r")
	if cursorCol > 0 {
		fmt.Fprintf(&b, "\x1b[%dC", cursorCol)
	}

	io.WriteString(e.out, b.String())
	e.rendered = len(rows)
	e.prevCursorRow = cursorRow
}

// physicalRows returns how many terminal rows `cells` display columns occupy at
// width `cols` (always at least 1).
func physicalRows(cells, cols int) int {
	if cols < 1 {
		cols = 1
	}
	if cells <= 0 {
		return 1
	}
	return (cells + cols - 1) / cols
}

// emphasizeMatch styles a command label so the runes the fuzzy query matched
// (indices into the command name) pop in bold accent, the rest in the row's base
// color (bright on the selected row). Returns s unchanged when styling is off.
func emphasizeMatch(s string, pos []int, selected bool) string {
	if !style.Enabled {
		return s
	}
	hit := make(map[int]bool, len(pos))
	for _, p := range pos {
		hit[p] = true
	}
	base := style.White
	if selected {
		base = style.BoldGreen
	}
	var b strings.Builder
	for i, r := range []rune(s) {
		if hit[i] {
			b.WriteString(style.BoldRed(string(r))) // matched rune: bold coral accent
		} else {
			b.WriteString(base(string(r)))
		}
	}
	return b.String()
}

// displayLine renders a (possibly multi-line, from a paste) buffer on a single
// visual line: each newline becomes a ↵ marker so the in-place render stays
// one-line. The buffer itself keeps real newlines (returned verbatim on submit).
func displayLine(s string) string {
	return strings.ReplaceAll(s, "\n", "↵")
}

// menuRows builds the (capped) styled dropdown rows, the selected row marked.
func (e *Editor) menuRows() []string {
	if !e.menu.open || len(e.menu.filtered) == 0 {
		return nil
	}
	total := len(e.menu.filtered)
	n := total
	if n > maxMenuRows {
		n = maxMenuRows
	}
	// Scroll the visible window so the selected row is always shown: without this,
	// arrowing past row maxMenuRows hid the highlight AND let Tab/Enter complete a
	// command scrolled off-screen.
	start := 0
	if e.menu.sel >= n {
		start = e.menu.sel - n + 1
	}
	if max := total - n; start > max {
		start = max
	}
	rows := make([]string, n)
	for i := 0; i < n; i++ {
		c := e.menu.filtered[start+i]
		name := c.Name
		if c.Args != "" {
			name += " " + c.Args
		}
		selected := start+i == e.menu.sel
		var pos []int
		if start+i < len(e.menu.matchPos) {
			pos = e.menu.matchPos[start+i]
		}
		labeled := style.Pad(emphasizeMatch(name, pos, selected), 16)
		if selected {
			rows[i] = "  " + style.BoldGreen("›") + " " + labeled + style.Gray(c.Desc)
		} else {
			rows[i] = "    " + labeled + style.Gray(c.Desc)
		}
	}
	return rows
}
