package tui

import (
	"strings"

	"github.com/mholovetskyi/cliche/internal/style"
)

// ChatView is the model for the live full-screen chat: a transcript pane (which
// the agent's streamed output is written INTO — ChatView is an io.Writer), a
// session sidebar, and the input line. Render composes them into a frame; the
// driver (in package cli) redirects the session's output here and repaints on
// each line. The transcript keeps styled lines verbatim (Render truncates by
// visible width).
type ChatView struct {
	Transcript []string        // committed output lines (may carry ANSI)
	Sidebar    []string        // right pane: live session state
	Input      string          // current input-line text
	partial    strings.Builder // the in-progress (not-yet-newline-terminated) line
}

// Write captures streamed agent output: bytes accumulate into the current line,
// and each '\n' commits a transcript line. '\r' is dropped (we own line breaks).
func (v *ChatView) Write(p []byte) (int, error) {
	for _, r := range string(p) {
		switch r {
		case '\n':
			v.Transcript = append(v.Transcript, v.partial.String())
			v.partial.Reset()
		case '\r':
			// ignore — the streaming code uses \r for in-place tricks we don't want here
		default:
			v.partial.WriteRune(r)
		}
	}
	return len(p), nil
}

// FlushPartial commits any in-progress line (call at end of a turn).
func (v *ChatView) FlushPartial() {
	if v.partial.Len() > 0 {
		v.Transcript = append(v.Transcript, v.partial.String())
		v.partial.Reset()
	}
}

// lines returns the transcript plus the live (in-progress) line, so streaming
// text shows before its newline arrives.
func (v *ChatView) lines() []string {
	out := v.Transcript
	if v.partial.Len() > 0 {
		out = append(append([]string(nil), out...), v.partial.String())
	}
	return out
}

// Render composes the full w×h frame: a header, a body split into the transcript
// pane (left) and the sidebar (right), the input line, and a footer. Pure.
func (v *ChatView) Render(w, h int) []string {
	if h < 5 {
		h = 5
	}
	bodyH := h - 3 // header(1) + body + input(1) + footer(1)
	transW := w * 2 / 3
	if transW < 20 {
		transW = 20
	}
	sideW := w - transW

	tail := lastN(v.lines(), bodyH-2) // -2 for the box border rows
	body := joinColumns(
		box("chat", tail, transW, bodyH),
		box("session", v.Sidebar, sideW, bodyH),
	)

	frame := make([]string, 0, h)
	frame = append(frame, style.BoldWhite(cell("  cliché · live", w)))
	frame = append(frame, body...)
	frame = append(frame, cell("  "+style.BoldGreen(gl("❯"))+" "+v.Input, w))
	frame = append(frame, style.Dim(cell("  type to chat · enter send · esc / :q exit", w)))
	return frame
}

func lastN(s []string, n int) []string {
	if n < 0 {
		n = 0
	}
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// gl returns a glyph or an ASCII fallback (mirrors the cli helper; the tui
// package can't import cli). Styling is on whenever the TUI runs, so the fancy
// glyph is the normal case.
func gl(fancy string) string {
	if !style.Enabled {
		return ">"
	}
	return fancy
}
