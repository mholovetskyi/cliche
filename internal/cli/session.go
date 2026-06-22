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
	resumed   int         // messages restored from a resumed session (0 if fresh)
	streaming bool        // currently mid live-streamed assistant block
	stream    *mdStreamer // line-buffered markdown renderer for the streamed block
	app       *approver   // for /mode (mutates the approver's permission mode)
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
// spinner: any event stops the spinner first (so frames never race output). The
// spinner then narrates the next phase — a tool's execution after a tool_call,
// the model's thinking after a result — so a long step is never dead silence.
func (s *session) onEvent(e agent.Event) {
	if e.Kind == "delta" {
		s.stopSpin()
		if !s.streaming {
			fmt.Fprintln(s.out) // start the assistant block on its own line
			s.streaming = true
			s.stream = newMdStreamer(s.out)
		}
		s.stream.write(e.Text)
		return
	}
	s.endStream()
	s.stopSpin()
	printEvent(s.out, e)
	switch e.Kind {
	case "tool_call":
		s.startSpin(spinLabel(e)) // spin while the tool runs (fills the old silent gap)
	case "tool_result":
		s.startSpin("thinking…") // spin while the model reasons about the result
	}
}

// endStream closes a live-streamed assistant block, flushing any trailing
// partial line through the markdown streamer.
func (s *session) endStream() {
	if s.streaming {
		if s.stream != nil {
			s.stream.flush()
			s.stream = nil
		}
		s.streaming = false
	}
}

func (s *session) startSpin(label string) {
	s.spin = newSpinner(s.out, label)
	s.spin.Start()
}

// spinLabel narrates a tool call as a present-progressive phase for the spinner.
func spinLabel(e agent.Event) string {
	gerunds := map[string]string{
		"read_file": "reading", "write_file": "writing", "edit_file": "editing",
		"apply_diff": "editing", "run_command": "running", "search_files": "searching",
		"find_files": "searching", "list_files": "listing", "web_fetch": "fetching",
		"spawn_subagent": "delegating", "spawn_subagents": "delegating",
	}
	g := gerunds[e.Tool]
	if g == "" {
		return "working…"
	}
	if e.Detail != "" {
		return g + " " + style.Truncate(e.Detail, 40) + "…"
	}
	return g + "…"
}

func (s *session) stopSpin() {
	if s.spin != nil {
		s.spin.Stop()
		s.spin = nil
	}
}

func (s *session) loop() int {
	clearScreen(s.out)
	defer style.ShowCursor(s.out) // never strand a hidden cursor (panic-safe)
	_, source := secrets.Lookup(s.cfg.Provider)
	keySrc := "saved"
	if strings.HasPrefix(source, "env:") {
		keySrc = "env"
	}
	fmt.Fprintln(s.out)
	fmt.Fprintln(s.out, compactHeader(s.cfg.Provider, s.cfg.Model, s.modeName(), keySrc))
	if w := keyOverrideWarning(s.cfg.Provider); w != "" {
		fmt.Fprintln(s.out, "  "+style.Red(gl("⚠", "!"))+" "+style.White(w))
	}
	if s.resumed > 0 {
		u := s.a.Usage()
		fmt.Fprintf(s.out, "  %s\n", style.Gray(fmt.Sprintf("resumed %s · %d messages · ~$%.4f so far", s.id, s.resumed, u.USD)))
	}
	fmt.Fprintln(s.out, "  "+style.Dim(slashHint()))
	for {
		// A fresh status strip at each prompt keeps trust state (mode, model,
		// spend, context use) glanceable at the point of input.
		fmt.Fprintln(s.out, "\n  "+s.statusStrip())
		fmt.Fprint(s.out, "  "+s.prompt())
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
		start, u0 := time.Now(), s.a.Usage()
		s.startSpin("thinking…") // shimmer while we wait on the first model response
		o, runErr := s.a.Run(ctx, line)
		s.stopSpin()
		s.endStream() // close any open streamed block before the outcome line
		stop()
		s.persist() // save the transcript after every task (incl. halts)
		if runErr != nil {
			s.renderError(runErr.Error())
			continue
		}
		s.afterTask(o, time.Since(start), s.a.Usage().USD-u0.USD)
	}
}

// modeName is the current permission mode (defaults to suggest).
func (s *session) modeName() string {
	if s.app != nil && s.app.mode != "" {
		return s.app.mode
	}
	return modeSuggest
}

// statusStrip is the dim one-liner above the prompt: the trust-critical state at
// a glance — mode, model, spend, and how full the context is.
func (s *session) statusStrip() string {
	u := s.a.Usage()
	parts := s.modeName() + " · " + shortModel(s.a.Model()) + fmt.Sprintf(" · $%.4f", u.USD)
	if est, _ := s.a.ContextStats(); s.cfg.Context.LimitTokens > 0 {
		parts += fmt.Sprintf(" · ctx %d%%", pctOf(est, s.cfg.Context.LimitTokens))
	}
	return style.Gray(parts)
}

// prompt is a single chevron whose color encodes permission risk: gray when the
// agent must ask (plan/suggest), coral when it auto-edits, red when it auto-runs
// everything (full) — so the prompt itself signals how much rope is out.
func (s *session) prompt() string {
	c := style.GrayRGB
	switch s.modeName() {
	case modeAutoEdit:
		c = style.Sample(0.5)
	case modeFull:
		c = style.RedRGB
	}
	return style.Color(gl("❯", ">"), c) + " "
}

// shortModel drops a provider prefix for the status strip (openai/gpt-4o-mini →
// gpt-4o-mini, anthropic/claude-sonnet-4-6 → claude-sonnet-4-6).
func shortModel(m string) string {
	if i := strings.LastIndexByte(m, '/'); i >= 0 && i+1 < len(m) {
		return m[i+1:]
	}
	return m
}

// pctOf returns 100*part/whole clamped to [0,100].
func pctOf(part, whole int) int {
	if whole <= 0 {
		return 0
	}
	p := (part*100 + whole/2) / whole
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// renderError prints a run/provider error as a styled block with an actionable
// hint for the common cases (bad key, no credits, rate limit, wrong model).
func (s *session) renderError(msg string) {
	fmt.Fprintf(s.out, "\n  %s %s\n", style.BoldRed(gl("■", "x")), style.BoldRed("error"))
	fmt.Fprintln(s.out, "  "+style.Gray(boundMessage(msg)))
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

func (s *session) afterTask(o agent.Outcome, elapsed time.Duration, taskUSD float64) {
	u := s.a.Usage()
	renderOutcome(s.out, o, outcomeMetrics{elapsed: elapsed, tokens: u.TotalTokens(), taskUSD: taskUSD, sessionUSD: u.USD})
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
		if lim.MaxUSD > 0 {
			frac := u.USD / lim.MaxUSD
			fmt.Fprintf(s.out, "  %s%s\n", gaugePrefix(frac, 8),
				style.Gray(fmt.Sprintf("%d%% · $%.4f of $%.2f cap · %s tokens", pctFloat(frac), u.USD, lim.MaxUSD, humanTokens(u.TotalTokens()))))
		} else {
			fmt.Fprintf(s.out, "  %s\n", style.Gray(fmt.Sprintf("$%.4f · %s tokens (no cap)", u.USD, humanTokens(u.TotalTokens()))))
		}
	case "/clear":
		s.a.Reset()
		fmt.Fprintln(s.out, "  context cleared (budget preserved).")
	case "/context":
		est, compactions := s.a.ContextStats()
		if lim := s.cfg.Context.LimitTokens; lim > 0 {
			frac := float64(est) / float64(lim)
			fmt.Fprintf(s.out, "  %s%s\n", gaugePrefix(frac, 8),
				style.Gray(fmt.Sprintf("%d%% · ~%s of %s tokens · %d compaction(s)", pctOf(est, lim), humanTokens(est), humanTokens(lim), compactions)))
		} else {
			fmt.Fprintf(s.out, "  %s\n", style.Gray(fmt.Sprintf("~%s tokens · %d compaction(s)", humanTokens(est), compactions)))
		}
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
	case "/rewind":
		s.rewind()
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
		s.help()
	default:
		cmd := strings.Fields(line)[0]
		if guess := closestCommand(cmd); guess != "" {
			fmt.Fprintf(s.out, "  unknown command %s — did you mean %s?\n", style.White(cmd), style.White(guess))
		} else {
			fmt.Fprintf(s.out, "  unknown command %s (try /help)\n", style.White(cmd))
		}
	}
	return false
}

// gaugePrefix returns a Gauge plus a trailing space when styling is on, or ""
// when off (so the numeric % that follows carries the meaning).
func gaugePrefix(frac float64, width int) string {
	if g := style.Gauge(frac, width); g != "" {
		return g + " "
	}
	return ""
}

func pctFloat(frac float64) int { return pctOf(int(frac*1000), 1000) }

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

// rewind reverts every file change made this session (undo the agent).
func (s *session) rewind() {
	reverted, err := s.journal.RewindAll()
	if err != nil {
		fmt.Fprintln(s.out, "  rewind failed: "+err.Error())
		return
	}
	if len(reverted) == 0 {
		fmt.Fprintln(s.out, "  nothing to rewind.")
		return
	}
	fmt.Fprintf(s.out, "  rewound %d file(s): %s\n", len(reverted), style.Gray(strings.Join(reverted, ", ")))
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
		// <bullet> <fixed-width verb> <target>. The bullet hue encodes the action
		// category (reads coral → edits mid → commands peach); the verb column is
		// Pad'd to display cells so the target column is dead-straight every row.
		bullet := style.Color(gl("◇", "*"), style.Sample(verbHue(e.Tool)))
		verb := style.White(style.Pad(verbLabel(e.Tool), 6))
		if e.Detail != "" {
			fmt.Fprintf(out, "  %s %s %s\n", bullet, verb, style.Gray(e.Detail))
		} else {
			fmt.Fprintf(out, "  %s %s\n", bullet, verb)
		}
	case "tool_result":
		// Every result is surfaced now (silence used to read as a hang): a quiet
		// green tick for success, a loud red cross for failure, under a connector.
		if e.OK {
			if noColor {
				fmt.Fprintf(out, "      %s\n", e.Detail) // quiet: just the metric
			} else {
				fmt.Fprintf(out, "    %s %s %s\n", style.Gray("└"), style.Green("✓"), style.Gray(e.Detail))
			}
		} else {
			if noColor {
				fmt.Fprintf(out, "      FAIL %s\n", e.Detail)
			} else {
				fmt.Fprintf(out, "    %s %s %s\n", style.Gray("└"), style.Red("✗"), style.White(e.Detail))
			}
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

// verbLabel maps a tool name to a short human verb for the activity feed. A
// fixed vocabulary keeps the verb column narrow and the targets aligned. (No
// double-width emoji — those misalign the column on most terminals.)
func verbLabel(tool string) string {
	switch tool {
	case "read_file":
		return "Read"
	case "write_file":
		return "Write"
	case "edit_file", "apply_diff":
		return "Edit"
	case "run_command":
		return "Run"
	case "search_files", "find_files":
		return "Search"
	case "list_files":
		return "List"
	case "web_fetch":
		return "Fetch"
	case "spawn_subagent", "spawn_subagents":
		return "Spawn"
	default:
		return tool
	}
}

// verbHue places a tool on the brand gradient by action category, so the bullet
// color carries information: reads/searches at the coral start, edits in the
// middle, commands/spawns at the peach end (escalating "weight").
func verbHue(tool string) float64 {
	switch tool {
	case "edit_file", "write_file", "apply_diff":
		return 0.5
	case "run_command", "spawn_subagent", "spawn_subagents":
		return 1
	default:
		return 0
	}
}
