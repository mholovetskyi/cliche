package tui

import (
	"io"
	"strings"

	"github.com/mholovetskyi/cliche/internal/cli/keydec"
	"github.com/mholovetskyi/cliche/internal/cli/rawmode"
	"github.com/mholovetskyi/cliche/internal/style"
)

// box renders a titled, rounded-border panel of exactly w×h cells: a top edge
// with the title inlaid, h-2 content rows (each padded/truncated to fit), and a
// bottom edge. Content beyond h-2 rows is dropped.
func box(title string, content []string, w, h int) []string {
	if w < 4 {
		w = 4
	}
	if h < 2 {
		h = 2
	}
	inner := w - 2
	t := style.Truncate(" "+title+" ", inner)
	dashes := inner - style.Width(t)
	if dashes < 0 {
		dashes = 0
	}
	out := make([]string, 0, h)
	out = append(out, style.Gray("╭")+style.BoldWhite(t)+style.Gray(strings.Repeat("─", dashes)+"╮"))
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		out = append(out, style.Gray("│")+cell(" "+line, inner)+style.Gray("│"))
	}
	out = append(out, style.Gray("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

// joinColumns places equal-height boxes side by side, concatenating row i of
// each. Rows are padded to the tallest column.
func joinColumns(cols ...[]string) []string {
	h := 0
	for _, c := range cols {
		if len(c) > h {
			h = len(c)
		}
	}
	out := make([]string, h)
	for i := 0; i < h; i++ {
		var b strings.Builder
		for _, c := range cols {
			if i < len(c) {
				b.WriteString(c[i])
			}
		}
		out[i] = b.String()
	}
	return out
}

// renderDashboard composes the snapshot layout: a left column (trust state over
// tasks) beside a right column (file changes), filling w×h. Pure — unit-tested.
func renderDashboard(w, h int, status, tasks, changes []string) []string {
	leftW := w * 2 / 5
	if leftW < 24 {
		leftW = 24
	}
	if leftW > w-10 {
		leftW = w - 10
	}
	rightW := w - leftW
	topH := h / 2
	botH := h - topH
	left := append(box("trust", status, leftW, topH), box("tasks", tasks, leftW, botH)...)
	right := box("changes", changes, rightW, h)
	return joinColumns(left, right)
}

// Dashboard shows a full-screen, multi-pane snapshot of the live session (trust
// state, tasks, file changes) and waits for q/Esc/Enter to dismiss. It owns the
// alternate screen. A static snapshot for now — the foundation for live panes.
func Dashboard(dec *keydec.Decoder, out io.Writer, width, height int, status, tasks, changes []string) {
	if dec == nil {
		return
	}
	if width < 30 {
		width = 30
	}
	if height < 6 {
		height = 6
	}
	rawmode.EnterAlt(out)
	defer rawmode.LeaveAlt(out)

	frame := renderDashboard(width, height-1, status, tasks, changes)
	var b strings.Builder
	b.WriteString("\x1b[H")
	for _, ln := range frame {
		b.WriteString("\x1b[K" + ln + "\r\n")
	}
	b.WriteString(style.Dim("  q / esc to return"))
	io.WriteString(out, b.String())

	for {
		k, err := dec.ReadKey()
		if err != nil {
			return
		}
		switch k.Type {
		case keydec.KeyEnter, keydec.KeyEsc, keydec.KeyCtrlC:
			return
		case keydec.KeyRune:
			if k.Rune == 'q' || k.Rune == 'Q' {
				return
			}
		}
	}
}
