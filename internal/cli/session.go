package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/secrets"
	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/tools"
)

// clearScreen wipes the terminal (including scrollback) and homes the cursor,
// for a clean app-like start. No-op when styling is off (pipes/CI/tests).
func clearScreen(w io.Writer) {
	if style.Enabled {
		fmt.Fprint(w, "\x1b[2J\x1b[3J\x1b[H")
	}
}

// keyOverrideWarning returns a warning when an environment variable is shadowing
// a different saved key — the usual cause of a sudden "rejected key" after a
// successful `cliche login`.
func keyOverrideWarning(provider string) string {
	key, source := secrets.Lookup(provider)
	if !strings.HasPrefix(source, "env:") {
		return ""
	}
	if saved := secrets.Saved(provider); saved != "" && saved != key {
		env := secrets.EnvVar(provider)
		return env + " in your shell is overriding your saved key (it may be stale). Unset it to use the key from `cliche login`."
	}
	return ""
}

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

	if !validMode(f.mode) {
		fmt.Fprintf(errOut, "chat: unknown --mode %q (want plan | suggest | auto-edit | full)\n", f.mode)
		return 2
	}
	mode := f.mode
	if mode == "" {
		mode = modeSuggest
	}
	reader := bufio.NewReader(os.Stdin)
	app := &approver{r: reader, out: out, mode: mode}

	// Seamless first run: if no provider key is configured yet, drop straight
	// into the setup wizard instead of erroring out.
	if cfg, _ := config.Load(f.dir); f.provider == "" && firstProviderWithKey(cfg) == "" {
		if code := runLogin(reader, out); code != 0 {
			return code
		}
	}

	a, journal, cfg, cleanup, err := buildAgent(f, app.Approve, false) // chat: mode governed by the approver (mutable via /mode)
	if err != nil {
		fmt.Fprintln(errOut, "chat: "+err.Error())
		return 1
	}
	defer cleanup()

	s := &session{a: a, r: reader, out: out, dir: f.dir, cfg: cfg, verify: f.verify, journal: journal, created: time.Now(), app: app}
	a.SetObserver(s.onEvent)

	// Resume a saved session if requested (--continue = most recent, --resume <id>).
	if id := f.resume; id != "" || f.cont {
		if id == "" {
			id = sess.Latest(f.dir)
		}
		if id == "" {
			fmt.Fprintln(errOut, "chat: no saved session to resume (try `cliche sessions`).")
		} else if rec, err := sess.Load(f.dir, id); err != nil {
			fmt.Fprintln(errOut, "chat: "+err.Error())
		} else {
			a.Restore(rec.Messages, rec.Usage)
			s.id, s.title, s.created = rec.ID, rec.Title, rec.Created
			s.resumed = len(rec.Messages)
		}
	}
	if s.id == "" {
		s.id = sess.NewID(s.created)
	}
	if f.branch {
		startBranch(out, f.dir, s.id)
	}
	return s.loop()
}

type session struct {
	a         *agent.Agent
	r         *bufio.Reader
	out       io.Writer
	dir       string
	cfg       config.Config
	verify    bool
	journal   *tools.EditJournal
	spin      *spinner // active "thinking" indicator during a model wait (main goroutine only)
	id        string   // session id for on-disk persistence
	title     string   // first prompt, used as the session title
	created   time.Time
	resumed   int       // messages restored from a resumed session (0 if fresh)
	streaming bool      // currently mid live-streamed assistant block
	app       *approver // for /mode (mutates the approver's permission mode)
}

// persist writes the session transcript to .cliche/sessions/<id>.json. Best
// effort: a disk error must not kill the live session.
func (s *session) persist() {
	if s.id == "" {
		return
	}
	_ = sess.Save(s.dir, sess.Record{
		ID:       s.id,
		Title:    s.title,
		Provider: s.cfg.Provider,
		Model:    s.a.Model(),
		Created:  s.created,
		Updated:  time.Now(),
		Usage:    s.a.Usage(),
		Messages: s.a.Transcript(),
	})
}

// onEvent renders a live activity event, coordinating with the thinking
// spinner: any event stops the spinner first (so frames never race output),
// and after tool results the model will think again, so it's restarted.
func (s *session) onEvent(e agent.Event) {
	if e.Kind == "delta" {
		s.stopSpin()
		if !s.streaming {
			fmt.Fprintln(s.out) // start the assistant block on its own line
			s.streaming = true
		}
		fmt.Fprint(s.out, e.Text)
		return
	}
	s.endStream()
	s.stopSpin()
	printEvent(s.out, e)
	if e.Kind == "tool_result" {
		s.startSpin()
	}
}

// endStream closes a live-streamed assistant block with a trailing newline.
func (s *session) endStream() {
	if s.streaming {
		fmt.Fprintln(s.out)
		s.streaming = false
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
	clearScreen(s.out)
	fmt.Fprint(s.out, banner())
	_, source := secrets.Lookup(s.cfg.Provider)
	keySrc := "saved"
	if strings.HasPrefix(source, "env:") {
		keySrc = "env"
	}
	modeLabel := ""
	if s.app != nil {
		modeLabel = "  · mode: " + s.app.mode
	}
	fmt.Fprintln(s.out, "  "+style.Gray(s.cfg.Provider+" · "+s.cfg.Model)+style.Dim("  · key: "+keySrc+modeLabel))
	if w := keyOverrideWarning(s.cfg.Provider); w != "" {
		fmt.Fprintln(s.out, "  "+style.Red(gl("⚠", "!"))+" "+style.White(w))
	}
	if s.resumed > 0 {
		u := s.a.Usage()
		fmt.Fprintf(s.out, "  %s\n", style.Gray(fmt.Sprintf("resumed session %s · %d messages · ~$%.4f spent so far", s.id, s.resumed, u.USD)))
	}
	fmt.Fprintln(s.out, "  "+style.Gray("/cost · /diff · /undo · /model · /mode · /verify · /context · /clear · /help · /exit"))
	for {
		fmt.Fprint(s.out, "\n"+style.Color(gl("❯", ">"), style.Sample(0))+style.Color(gl("❯", ">"), style.Sample(0.5))+style.Color(gl("❯", ">"), style.Sample(1))+" ")
		line, err := s.r.ReadString('\n')
		if err != nil { // EOF (Ctrl-D)
			fmt.Fprintln(s.out)
			s.persist()
			return 0
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			if s.slash(line) {
				s.persist()
				return 0
			}
			continue
		}
		if s.title == "" {
			s.title = line // first prompt becomes the session title
		}
		// Install a SIGINT handler only while a task runs, so Ctrl-C aborts the
		// current task (gracefully, structured) but leaves the session alive;
		// Ctrl-C at the idle prompt uses the default behavior (quit).
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		s.startSpin() // shimmer while we wait on the first model response
		o, runErr := s.a.Run(ctx, line)
		s.stopSpin()
		s.endStream() // close any open streamed block before the outcome line
		stop()
		s.persist() // save the transcript after every task (incl. halts)
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
	case "/model":
		s.switchModel(line)
	case "/mode":
		s.setMode(line)
	case "/commit":
		subject := strings.TrimSpace(strings.TrimPrefix(line, "/commit"))
		if subject == "" {
			subject = s.title
		}
		commitChanges(s.out, s.dir, subject, s.a.Model(), s.a.Usage().USD)
	case "/help":
		fmt.Fprintln(s.out, "  /cost — spend so far    /context — context usage   /verify — re-run tests")
		fmt.Fprintln(s.out, "  /diff — changes so far  /undo — revert last edit   /model — show/switch model")
		fmt.Fprintln(s.out, "  /mode — permission mode /commit — git commit       /recover — undo compaction")
		fmt.Fprintln(s.out, "  /clear — reset context  /exit — quit")
	default:
		fmt.Fprintf(s.out, "  unknown command (try /help)\n")
	}
	return false
}

// setMode shows or switches the permission mode for the rest of the session.
// Fully effective when the session started in the default mode; legacy
// --allow-write/--yolo flags pre-authorize at the executor and aren't undone.
func (s *session) setMode(line string) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		fmt.Fprintf(s.out, "  mode: %s\n", style.White(s.app.mode))
		fmt.Fprintln(s.out, "  "+style.Gray("plan — read-only · suggest — ask · auto-edit — auto edits, ask commands · full — auto all"))
		fmt.Fprintln(s.out, "  "+style.Gray("switch with `/mode <name>`"))
		return
	}
	m := fields[1]
	if m == "" || !validMode(m) {
		fmt.Fprintf(s.out, "  unknown mode %q (plan | suggest | auto-edit | full)\n", m)
		return
	}
	s.app.setMode(m)
	fmt.Fprintf(s.out, "  mode → %s\n", style.White(m))
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

// switchModel shows or changes the model for the rest of the session. The model
// is sent to the current provider, so on a multi-model provider (e.g.
// OpenRouter) you can hop between models mid-chat.
func (s *session) switchModel(line string) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		fmt.Fprintf(s.out, "  model: %s %s\n", style.White(s.a.Model()), style.Gray("(provider "+s.cfg.Provider+")"))
		fmt.Fprintln(s.out, "  "+style.Gray("switch with `/model <id>` · `cliche models` lists priced ids"))
		return
	}
	m := fields[1]
	s.a.SetModel(m)
	s.cfg.Model = m
	fmt.Fprintf(s.out, "  model → %s\n", style.White(m))
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
	case "delta":
		// Live-streamed text chunk (used by `run`; chat handles deltas in onEvent
		// with newline management). Print raw, no newline.
		fmt.Fprint(out, e.Text)
	case "text":
		if t := strings.TrimSpace(e.Text); t != "" {
			fmt.Fprintf(out, "\n%s\n", renderMarkdown(t))
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
	case "cache":
		fmt.Fprintf(out, "  %s\n", style.Gray(gl("⚡", "~")+" "+e.Detail))
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
