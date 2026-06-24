package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/mholovetskyi/cliche/internal/cli/keydec"
	"github.com/mholovetskyi/cliche/internal/cli/rawmode"
	"github.com/mholovetskyi/cliche/internal/cli/tui"
	"github.com/mholovetskyi/cliche/internal/style"
)

// tuiSink redirects the session's output into the live chat's transcript pane:
// every Write captures the bytes and repaints the frame, so streamed model text
// and the tool feed appear in the pane as they happen.
type tuiSink struct {
	v       *tui.ChatView
	repaint func()
}

func (s tuiSink) Write(p []byte) (int, error) {
	n, _ := s.v.Write(p)
	s.repaint()
	return n, nil
}

// tuiChat (/tui) runs the OPT-IN full-screen live chat: a transcript pane that
// streams the agent's output, a session sidebar, and an input line. The default
// inline REPL is untouched; this is a separate full-screen mode you exit with
// esc or :q. Falls back with a note when raw mode is unavailable.
func (s *session) tuiChat() {
	if !style.Enabled || os.Getenv("CLICHE_NO_RAW") != "" || stdinIsPiped() || !rawmode.IsTerminal(os.Stdin) {
		fmt.Fprintln(s.out, "  /tui needs an interactive terminal.")
		return
	}
	st, err := rawmode.Enable(os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(s.out, "  /tui: "+err.Error())
		return
	}
	defer st.Disable()
	s.ensureEditor()
	dec := s.editor.Decoder()

	rawmode.EnterAlt(os.Stdout)
	rawmode.EnableMouse(os.Stdout)
	defer func() {
		rawmode.DisableMouse(os.Stdout)
		rawmode.LeaveAlt(os.Stdout)
	}()

	view := &tui.ChatView{Sidebar: s.tuiSidebar()}
	cols, rows := rawmode.Size(os.Stdout)
	repaint := func() {
		if c, r := rawmode.Size(os.Stdout); c > 0 && r > 0 {
			cols, rows = c, r
		}
		frame := view.Render(cols, rows)
		var b strings.Builder
		b.WriteString("\x1b[H")
		for i, ln := range frame {
			b.WriteString("\x1b[K")
			b.WriteString(ln)
			if i < len(frame)-1 {
				b.WriteString("\r\n")
			}
		}
		io.WriteString(os.Stdout, b.String())
	}

	repaint()
	for {
		k, err := dec.ReadKey()
		if err != nil {
			return
		}
		switch k.Type {
		case keydec.KeyEsc:
			return
		case keydec.KeyEnter:
			in := strings.TrimSpace(view.Input)
			view.Input = ""
			if in == ":q" || in == ":quit" {
				return
			}
			if in != "" {
				s.tuiTurn(view, repaint, in)
			}
		case keydec.KeyBackspace:
			if r := []rune(view.Input); len(r) > 0 {
				view.Input = string(r[:len(r)-1])
			}
		case keydec.KeyRune:
			view.Input += string(k.Rune)
		case keydec.KeyPaste:
			view.Input += strings.ReplaceAll(k.Text, "\n", " ")
		}
		repaint()
	}
}

// tuiTurn runs one agent turn with output streaming into the transcript pane.
func (s *session) tuiTurn(view *tui.ChatView, repaint func(), prompt string) {
	view.Transcript = append(view.Transcript, "", style.BoldGreen("❯ "+prompt))
	repaint()

	prevOut := s.out
	s.out = tuiSink{v: view, repaint: repaint}
	s.tuiActive = true
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt) // Ctrl-C aborts the turn
	o, runErr := s.a.Run(ctx, prompt)
	stop()
	s.tuiActive = false
	s.out = prevOut

	view.FlushPartial()
	if runErr != nil {
		view.Transcript = append(view.Transcript, style.Red("  error: "+runErr.Error()))
	} else {
		view.Transcript = append(view.Transcript, style.Gray(fmt.Sprintf("  ✓ done · %d turns · $%.4f", o.Turns, o.Usage.USD)))
	}
	view.Sidebar = s.tuiSidebar()
	s.persist()
	repaint()
}

// tuiSidebar is the live session panel: trust state + the file changes so far.
func (s *session) tuiSidebar() []string {
	u, lim := s.a.Usage(), s.a.Limits()
	out := []string{
		style.Gray("mode  ") + s.modeName(),
		style.Gray("model ") + shortModel(s.a.Model()),
		style.Gray("spend ") + fmt.Sprintf("$%.4f", u.USD),
	}
	if lim.MaxUSD > 0 {
		out = append(out, style.Gray("cap   ")+fmt.Sprintf("$%.2f", lim.MaxUSD))
	}
	out = append(out, "", style.Gray("changes"))
	changes := s.journal.Changes()
	if len(changes) == 0 {
		out = append(out, style.Dim("  none yet"))
	}
	for _, c := range changes {
		tag := "~"
		switch {
		case c.Deleted:
			tag = "-"
		case c.WasNew:
			tag = "+"
		}
		out = append(out, "  "+tag+" "+c.Path)
	}
	return out
}
