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
	dec      *keydec.Decoder
	out      io.Writer
	history  *History
	menu     *slashMenu
	prompt   string
	promptW  int
	buf      []rune
	cursor   int
	rendered int // dropdown rows currently drawn below the input line

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
	}
}

// ReadLine reads one logical line, rendering the prompt (promptW display cells
// wide, for cursor math). Returns the line on Enter, ErrInterrupted on Ctrl-C,
// io.EOF on Ctrl-D at an empty line.
func (e *Editor) ReadLine(prompt string, promptW int) (string, error) {
	e.prompt, e.promptW = prompt, promptW
	e.buf, e.cursor, e.rendered = e.buf[:0], 0, 0
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
			e.rendered = 0
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
	e.render() // wipes any dropdown rows, leaves cursor on the input line
	io.WriteString(e.out, "\r\n")
}

// render rewrites the input line in place and (re)draws or clears the dropdown
// below it, always leaving the cursor on the input line at the edit position. A
// full rewrite each keystroke (no incremental diffing) keeps it simple and
// robust; assertions in tests check visible content, not exact escapes.
func (e *Editor) render() {
	var b strings.Builder
	b.WriteString("\r\x1b[K") // input line: column 0, erase to EOL
	b.WriteString(e.prompt)
	b.WriteString(displayLine(string(e.buf)))

	rows := e.menuRows()
	clearN := len(rows)
	if e.rendered > clearN {
		clearN = e.rendered // also wipe rows a shrinking/closing menu left behind
	}
	for i := 0; i < clearN; i++ {
		b.WriteString("\r\n\x1b[K")
		if i < len(rows) {
			b.WriteString(rows[i])
		}
	}
	if clearN > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", clearN) // back up to the input line
	}
	b.WriteString("\r")
	if col := e.promptW + style.Width(displayLine(string(e.buf[:e.cursor]))); col > 0 {
		fmt.Fprintf(&b, "\x1b[%dC", col) // park the cursor at the edit column
	}

	io.WriteString(e.out, b.String())
	e.rendered = len(rows)
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
	n := len(e.menu.filtered)
	if n > maxMenuRows {
		n = maxMenuRows
	}
	rows := make([]string, n)
	for i := 0; i < n; i++ {
		c := e.menu.filtered[i]
		name := c.Name
		if c.Args != "" {
			name += " " + c.Args
		}
		if i == e.menu.sel {
			rows[i] = "  " + style.BoldGreen("›") + " " + style.BoldGreen(style.Pad(name, 16)) + style.Gray(c.Desc)
		} else {
			rows[i] = "    " + style.White(style.Pad(name, 16)) + style.Gray(c.Desc)
		}
	}
	return rows
}
