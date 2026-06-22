package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/tools"
)

// noColor disables decorative glyphs (and color) for portability and dumb
// terminals — aligned with the style package's enablement.
var noColor = !style.Enabled

// gl returns the fancy glyph normally, or an ASCII fallback under NO_COLOR.
func gl(fancy, plain string) string {
	if noColor {
		return plain
	}
	return fancy
}

// cmdChat starts an interactive agentic session: type a task, the agent cooks
// (reads/edits files, runs commands) with live activity, then you ask again.
// The conversation and budget persist for the session; a fresh governor scopes
// loop breakers to each task.
func cmdChat(args []string, out, errOut io.Writer) int {
	f, fs := parseRunFlags("chat", args)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if stdinIsPiped() {
		fmt.Fprintln(errOut, "chat is interactive and needs a terminal; use `run`/`exec` for piped input.")
		return 2
	}

	reader := bufio.NewReader(os.Stdin)
	app := &approver{r: reader, out: out}

	// Seamless first run: if no provider key is configured yet, drop straight
	// into the setup wizard instead of erroring out.
	if f.provider == "" && firstProviderWithKey() == "" {
		if code := runLogin(reader, out); code != 0 {
			return code
		}
	}

	a, journal, cfg, cleanup, err := buildAgent(f, app.Approve)
	if err != nil {
		fmt.Fprintln(errOut, "chat: "+err.Error())
		return 1
	}
	defer cleanup()

	s := &session{a: a, r: reader, out: out, dir: f.dir, cfg: cfg, verify: f.verify, journal: journal}
	a.SetObserver(s.onEvent)
	return s.loop()
}

type session struct {
	a       *agent.Agent
	r       *bufio.Reader
	out     io.Writer
	dir     string
	cfg     config.Config
	verify  bool
	journal *tools.EditJournal
	spin    *spinner // active "thinking" indicator during a model wait (main goroutine only)
}

// onEvent renders a live activity event, coordinating with the thinking
// spinner: any event stops the spinner first (so frames never race output),
// and after tool results the model will think again, so it's restarted.
func (s *session) onEvent(e agent.Event) {
	s.stopSpin()
	printEvent(s.out, e)
	if e.Kind == "tool_result" {
		s.startSpin()
	}
}

func (s *session) startSpin() {
	s.spin = newSpinner(s.out, "thinking…")
	s.spin.Start()
}

func (s *session) stopSpin() {
	if s.spin != nil {
		s.spin.Stop()
		s.spin = nil
	}
}

func (s *session) loop() int {
	fmt.Fprint(s.out, banner())
	fmt.Fprintln(s.out, "  "+style.Gray(s.cfg.Provider+" · "+s.cfg.Model))
	fmt.Fprintln(s.out, "  "+style.Gray("/cost · /diff · /undo · /verify · /context · /clear · /help · /exit"))
	for {
		fmt.Fprint(s.out, "\n"+style.Color(gl("❯", ">"), style.Sample(0))+style.Color(gl("❯", ">"), style.Sample(0.5))+style.Color(gl("❯", ">"), style.Sample(1))+" ")
		line, err := s.r.ReadString('\n')
		if err != nil { // EOF (Ctrl-D)
			fmt.Fprintln(s.out)
			return 0
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			if s.slash(line) {
				return 0
			}
			continue
		}
		// Install a SIGINT handler only while a task runs, so Ctrl-C aborts the
		// current task (gracefully, structured) but leaves the session alive;
		// Ctrl-C at the idle prompt uses the default behavior (quit).
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		s.startSpin() // shimmer while we wait on the first model response
		o, runErr := s.a.Run(ctx, line)
		s.stopSpin()
		stop()
		if runErr != nil {
			s.renderError(runErr.Error())
			continue
		}
		s.afterTask(o)
	}
}

// renderError prints a run/provider error as a styled block with an actionable
// hint for the common cases (bad key, no credits, rate limit, wrong model).
func (s *session) renderError(msg string) {
	fmt.Fprintf(s.out, "\n  %s %s\n", style.BoldRed(gl("■", "x")), style.BoldRed("error"))
	fmt.Fprintln(s.out, "  "+style.Gray(msg))
	if hint := providerHint(msg); hint != "" {
		fmt.Fprintln(s.out, "  "+style.Color(gl("→", ">"), style.Sample(0))+" "+style.White(hint))
	}
}

// providerHint maps a raw provider error to a one-line, actionable suggestion.
func providerHint(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "credit") || strings.Contains(m, "afford") || strings.Contains(m, "quota") || strings.Contains(m, "billing"):
		return "your provider account is low on credits — add credits, or try a cheaper model (e.g. --model openai/gpt-4o-mini)."
	case strings.Contains(m, "rate") && strings.Contains(m, "limit"), strings.Contains(m, "429"):
		return "rate limited — wait a moment and retry; adding credits often raises the limit."
	case strings.Contains(m, "user not found"), strings.Contains(m, "401"), strings.Contains(m, "unauthor"), strings.Contains(m, "invalid api key"), strings.Contains(m, "invalid_api_key"), strings.Contains(m, "no auth"):
		return "the provider rejected the request — re-check your key with `cliche login`, or your account's balance."
	case strings.Contains(m, "model") && (strings.Contains(m, "not found") || strings.Contains(m, "invalid") || strings.Contains(m, "does not exist")):
		return "that model id may be wrong for this provider — list options with `cliche models`, or pass --model."
	}
	return ""
}

func (s *session) afterTask(o agent.Outcome) {
	switch o.Stop {
	case agent.StopCompleted:
		fmt.Fprintf(s.out, "\n%s %s%s\n", style.BoldWhite(gl("✔", "[done]")), style.BoldWhite("done"),
			style.Gray(fmt.Sprintf(" · %d turns", o.Turns)))
	case agent.StopCancelled:
		fmt.Fprintf(s.out, "\n%s\n", style.Red(gl("■", "[x]")+" interrupted"))
	case agent.StopBudget:
		fmt.Fprintf(s.out, "\n%s\n", style.Red(gl("■", "[x]")+" stopped: budget — "+o.Reason))
	default:
		fmt.Fprintf(s.out, "\n%s\n", style.Red(gl("■", "[x]")+" stopped: "+o.Stop+" — "+o.Reason))
	}
	u := s.a.Usage()
	fmt.Fprintln(s.out, "  "+style.Gray(fmt.Sprintf("session so far: %d tokens, ~$%.4f", u.TotalTokens(), u.USD)))
	if s.verify && o.Stop == agent.StopCompleted {
		v := autoVerify(s.out, s.dir, s.cfg)
		fmt.Fprintln(s.out, "  "+verdictStyled(v.Status))
	}
}

// slash handles a slash command, returning true if the session should exit.
func (s *session) slash(line string) bool {
	switch strings.Fields(line)[0] {
	case "/exit", "/quit":
		fmt.Fprintln(s.out, "bye.")
		return true
	case "/cost":
		u := s.a.Usage()
		lim := s.a.Limits()
		fmt.Fprintf(s.out, "  session: %d tokens, ~$%.4f", u.TotalTokens(), u.USD)
		if lim.MaxUSD > 0 {
			fmt.Fprintf(s.out, " of ~$%.2f cap", lim.MaxUSD)
		}
		fmt.Fprintln(s.out)
	case "/clear":
		s.a.Reset()
		fmt.Fprintln(s.out, "  context cleared (budget preserved).")
	case "/context":
		est, compactions := s.a.ContextStats()
		fmt.Fprintf(s.out, "  context: ~%d tokens, %d compaction(s) this session\n", est, compactions)
	case "/recover":
		if s.a.RecoverContext() {
			fmt.Fprintln(s.out, "  restored the pre-compaction context.")
		} else {
			fmt.Fprintln(s.out, "  nothing to recover.")
		}
	case "/verify":
		v := autoVerify(s.out, s.dir, s.cfg)
		fmt.Fprintf(s.out, "  verdict: %s\n", v.Status)
	case "/diff":
		s.showDiff()
	case "/undo":
		s.undo()
	case "/help":
		fmt.Fprintln(s.out, "  /cost — spend so far    /context — context usage   /verify — re-run tests")
		fmt.Fprintln(s.out, "  /diff — changes so far  /undo — revert last edit   /recover — undo compaction")
		fmt.Fprintln(s.out, "  /clear — reset context  /exit — quit")
	default:
		fmt.Fprintf(s.out, "  unknown command (try /help)\n")
	}
	return false
}

// showDiff prints the cumulative before→after diff of every file the agent has
// changed this session, so the user can review the whole footprint at a glance.
func (s *session) showDiff() {
	changes := s.journal.Changes()
	if len(changes) == 0 {
		fmt.Fprintln(s.out, "  no file changes this session.")
		return
	}
	for _, c := range changes {
		label := c.Path
		switch {
		case c.Deleted:
			label += "  " + style.Gray("(deleted)")
		case c.WasNew:
			label += "  " + style.Gray("(new)")
		}
		fmt.Fprintf(s.out, "\n  %s\n%s\n", style.White(label), colorizeDiff(tools.PreviewChange(c.Before, c.After)))
	}
}

// undo reverts the most recent file mutation made this session.
func (s *session) undo() {
	path, did, err := s.journal.Undo()
	switch {
	case err != nil:
		fmt.Fprintln(s.out, "  undo failed: "+err.Error())
	case !did:
		fmt.Fprintln(s.out, "  nothing to undo.")
	default:
		fmt.Fprintf(s.out, "  reverted %s\n", style.White(path))
	}
}

// printEvent renders one live activity event from the agent loop.
func printEvent(out io.Writer, e agent.Event) {
	switch e.Kind {
	case "text":
		if t := strings.TrimSpace(e.Text); t != "" {
			fmt.Fprintf(out, "\n%s\n", t)
		}
	case "tool_call":
		bullet := style.Color(gl("◆", "*"), style.Sample(0.35))
		if e.Detail != "" {
			fmt.Fprintf(out, "  %s %s  %s\n", bullet, style.White(toolGlyph(e.Tool)), style.Gray(e.Detail))
		} else {
			fmt.Fprintf(out, "  %s %s\n", bullet, style.White(toolGlyph(e.Tool)))
		}
	case "tool_result":
		if !e.OK { // only surface failures to keep the feed readable
			fmt.Fprintf(out, "    %s %s\n", style.Red(gl("✗", "x")), style.Gray(e.Detail))
		}
	case "halt":
		fmt.Fprintf(out, "  %s\n", style.Red(gl("■", "!")+" halted: "+e.Detail))
	case "budget":
		fmt.Fprintf(out, "  %s\n", style.Red(gl("■", "!")+" budget: "+e.Detail))
	case "context":
		fmt.Fprintf(out, "  %s\n", style.Gray(gl("◆", "~")+" context compacted: "+e.Detail))
	case "warn":
		fmt.Fprintf(out, "  %s\n", style.Red("! "+e.Detail))
	}
}

// toolGlyph prefixes a tool name with a small icon for the activity feed.
func toolGlyph(tool string) string {
	if noColor {
		return tool
	}
	icons := map[string]string{
		"read_file": "📖", "write_file": "✎", "edit_file": "✎",
		"run_command": "⌘", "search_files": "🔎", "find_files": "🔎",
		"list_files": "📂", "spawn_subagent": "⛬", "spawn_subagents": "⛬",
	}
	if ic, ok := icons[tool]; ok {
		return ic + " " + tool
	}
	return tool
}
