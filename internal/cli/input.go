package cli

import (
	"io"
	"os"

	"github.com/mholovetskyi/cliche/internal/cli/lineedit"
	"github.com/mholovetskyi/cliche/internal/cli/rawmode"
	"github.com/mholovetskyi/cliche/internal/style"
)

// readLineInteractive reads one prompt line. On a real styled TTY it uses the
// raw-mode line editor (a live "/" dropdown, ↑/↓ history, readline hotkeys, and
// Shift-Tab permission-mode cycling). In every other case — pipes, NO_COLOR,
// CLICHE_NO_RAW, a non-terminal stdin, or ANY raw-mode failure — it falls back to
// the proven cooked readInput (which also keeps \ continuation and paste), so the
// REPL can never break. The returned line flows through the unchanged downstream
// path in loop() (empty-skip, slash dispatch, @file expansion).
func (s *session) readLineInteractive() (string, error) {
	if !style.Enabled || os.Getenv("CLICHE_NO_RAW") != "" || stdinIsPiped() || !rawmode.IsTerminal(os.Stdin) {
		return s.readInput()
	}
	st, err := rawmode.Enable(os.Stdin, os.Stdout)
	if err != nil {
		return s.readInput() // silent fallback — never user-visible
	}
	defer st.Disable()

	s.ensureEditor()
	prompt := "  " + s.barPrompt()
	line, err := s.editor.ReadLine(prompt, style.Width(prompt))
	switch err {
	case nil:
		return line, nil
	case lineedit.ErrInterrupted:
		return "", nil // Ctrl-C at the idle prompt → treat as an empty line
	case io.EOF:
		return "", io.EOF // Ctrl-D on an empty line → exit, exactly as today
	default:
		return "", err
	}
}

// ensureEditor lazily builds the persistent raw-mode editor (one decoder for the
// whole session, so buffered read-ahead survives between lines) and wires
// Shift-Tab to cycle the permission mode.
func (s *session) ensureEditor() {
	if s.editor != nil {
		return
	}
	cmds := make([]lineedit.Command, len(slashCommands))
	for i, c := range slashCommands {
		cmds[i] = lineedit.Command{Name: c.name, Args: c.args, Desc: c.desc}
	}
	s.editor = lineedit.NewEditor(os.Stdin, os.Stdout, cmds, lineedit.NewHistory(nil))
	s.editor.CycleMode = func() (string, int) {
		if s.app != nil {
			s.app.setMode(nextMode(s.modeName()))
		}
		p := "  " + s.barPrompt()
		return p, style.Width(p)
	}
}
