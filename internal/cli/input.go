package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholovetskyi/cliche/internal/cli/lineedit"
	"github.com/mholovetskyi/cliche/internal/cli/rawmode"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/style"
)

// maxHistoryLines caps the persisted prompt history (.cliche/history).
const maxHistoryLines = 500

func historyFile(root string) string { return filepath.Join(config.Dir(root), "history") }

// loadHistory reads the persisted prompt history (most recent last, capped), so
// ↑ recalls prompts from previous sessions.
func loadHistory(root string) []string {
	data, err := os.ReadFile(historyFile(root))
	if err != nil {
		return nil
	}
	var out []string
	for _, ln := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(ln) != "" {
			out = append(out, ln)
		}
	}
	if len(out) > maxHistoryLines {
		out = out[len(out)-maxHistoryLines:]
	}
	return out
}

// appendHistory records one submitted single-line prompt. Multi-line prompts are
// skipped (history is one-line for ↑ recall). Best effort.
func appendHistory(root, line string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "\n") {
		return
	}
	if err := os.MkdirAll(config.Dir(root), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(historyFile(root), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line + "\n")
}

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
	io.WriteString(os.Stdout, "\x1b[?2004h")       // enable bracketed paste
	defer io.WriteString(os.Stdout, "\x1b[?2004l") // ...and turn it back off

	s.ensureEditor()
	prompt := "  " + s.barPrompt()
	line, err := s.editor.ReadLine(prompt, style.Width(prompt))
	switch err {
	case nil:
		appendHistory(s.dir, line) // persist for ↑ recall across sessions
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
	cmds := make([]lineedit.Command, 0, len(slashCommands)+len(s.customCmds))
	for _, c := range slashCommands {
		cmds = append(cmds, lineedit.Command{Name: c.name, Args: c.args, Desc: c.desc})
	}
	for _, c := range sortedCommands(s.customCmds) { // user's custom commands too
		cmds = append(cmds, lineedit.Command{Name: c.Name, Desc: c.Desc})
	}
	s.editor = lineedit.NewEditor(os.Stdin, os.Stdout, cmds, lineedit.NewHistory(loadHistory(s.dir)))
	s.editor.CycleMode = func() (string, int) {
		if s.app != nil {
			s.app.setMode(nextMode(s.modeName()))
		}
		p := "  " + s.barPrompt()
		return p, style.Width(p)
	}
}
