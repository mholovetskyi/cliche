package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/memory"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/repomap"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/tools"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

// repoMapBudget bounds the project map injected into the system prompt (and the
// default for `cliche map`). It's a few KB so it informs the agent without
// dominating the context window; the cached system block amortizes the cost.
const repoMapBudget = 6000

// runFlags are shared by `run` and `exec`.
type runFlags struct {
	model            string
	provider         string
	baseURL          string
	maxUSD           float64
	maxTokens        int
	maxTurns         int
	allowWrite       bool
	allowRun         bool
	allowWeb         bool
	sandbox          bool
	yolo             bool
	verify           bool
	allowOutsideRoot bool
	allowMCP         bool
	dir              string
	prompt           string // -p, used by exec
	resume           string // chat: resume this session id
	cont             bool   // chat: --continue the most recent session
	mode             string // permission mode: plan | suggest | auto-edit | full
	branch           bool   // work on a fresh git branch
	commit           bool   // commit changes after a successful run
	pro              bool   // product mode: world-class standard, stronger model, more iterations
}

func parseRunFlags(name string, args []string) (*runFlags, *flag.FlagSet) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	f := &runFlags{}
	fs.StringVar(&f.model, "model", "", "model id")
	fs.StringVar(&f.provider, "provider", "", "anthropic | openrouter | openai (default: auto-detect from your API keys)")
	fs.StringVar(&f.baseURL, "base-url", "", "override the provider API endpoint")
	fs.Float64Var(&f.maxUSD, "max-usd", -1, "estimated dollar cap")
	fs.IntVar(&f.maxTokens, "max-tokens", -1, "hard token cap")
	fs.IntVar(&f.maxTurns, "max-turns", -1, "governor turn limit")
	fs.BoolVar(&f.allowWrite, "allow-write", false, "permit file writes")
	fs.BoolVar(&f.allowRun, "allow-run", false, "permit shell commands")
	fs.BoolVar(&f.allowWeb, "allow-web", false, "permit web_fetch network access")
	fs.BoolVar(&f.sandbox, "sandbox", false, "strict posture: confine to root, deny network unless allowlisted, scrub secrets from shell commands")
	fs.BoolVar(&f.yolo, "yolo", false, "skip approvals (never the budget cap or governor)")
	fs.BoolVar(&f.verify, "verify", false, "after completion, re-run tests and report a verdict")
	fs.BoolVar(&f.allowOutsideRoot, "allow-outside-root", false, "permit file access outside the project root")
	fs.BoolVar(&f.allowMCP, "allow-mcp", false, "permit MCP tool calls without asking")
	fs.StringVar(&f.dir, "dir", ".", "project root")
	fs.StringVar(&f.prompt, "p", "", "prompt (headless)")
	fs.StringVar(&f.resume, "resume", "", "chat: resume a saved session by id")
	fs.BoolVar(&f.cont, "continue", false, "chat: resume the most recent session")
	fs.StringVar(&f.mode, "mode", "", "permission mode: plan | suggest | auto-edit | full")
	fs.BoolVar(&f.branch, "branch", false, "work on a fresh git branch (cliche/<id>)")
	fs.BoolVar(&f.commit, "commit", false, "commit the agent's changes after a successful run")
	fs.BoolVar(&f.pro, "pro", false, "product mode: hold a world-class engineering bar, upgrade a weak model, and allow more iterations")
	return f, fs
}

// hasProviderKey reports whether a key for a provider is configured (env var or
// the saved credentials file).
func hasProviderKey(name string) bool {
	key, _ := secrets.Lookup(name)
	return key != ""
}

// firstProviderWithKey returns the first provider (built-in precedence, then any
// config-defined) that has a key configured, or "" if none do.
func firstProviderWithKey(cfg config.Config) string {
	for _, p := range allProviderNames(cfg) {
		if hasProviderKey(p) {
			return p
		}
	}
	return ""
}

// maxOutputTokens caps a single completion. 8192 (up from a prior 4096) gives a
// coding agent room to write a sizable file in one tool call without the
// response — and thus the tool-call JSON — being truncated mid-stream. Supported
// by all current mainstream models.
const maxOutputTokens = 8192

// backend is a fully-resolved model target.
type backend struct {
	name, model, baseURL string
	native               bool              // Anthropic Messages API vs OpenAI-compatible
	headers              map[string]string // extra request headers (gateway auth)
}

// resolveBackend picks the provider, model, and endpoint. Cliche is
// provider-neutral and BYO-key, so it is not Anthropic-by-default in practice:
// when the user hasn't chosen a provider (no --provider) and the default
// provider's key is absent, it auto-detects from whichever configured key IS
// set. An explicit --provider is always respected (and errors if its key is
// missing). With no key at all, the error points at `cliche login`.
func resolveBackend(cfg config.Config, f *runFlags) (backend, error) {
	name := f.provider
	explicit := name != ""
	if name == "" {
		name = cfg.Provider
	}
	if name == "" {
		name = "anthropic"
	}
	info, known := lookupProvider(cfg, name)
	// Local servers (Ollama, LM Studio, …) need no key. Everything else is
	// BYO-key: auto-detect from a configured key when the default lacks one.
	if !info.local {
		if !explicit && !hasProviderKey(name) {
			if alt := firstProviderWithKey(cfg); alt != "" {
				name = alt
				info, known = lookupProvider(cfg, name)
			}
		}
		if !hasProviderKey(name) {
			return backend{}, fmt.Errorf(
				"no API key configured for %q. Cliche is BYO-key — set one up once with `cliche login`\n"+
					"  (or `cliche auth %s`), or export %s for this shell.",
				name, name, secrets.EnvVar(name))
		}
	}
	baseURL := info.baseURL
	if f.baseURL != "" {
		baseURL = f.baseURL
	} else if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	if !known && baseURL == "" {
		return backend{}, fmt.Errorf("unknown provider %q — pass --base-url (any OpenAI-compatible endpoint) or define it under `providers` in .cliche/config.json", name)
	}
	if !info.native && baseURL == "" {
		return backend{}, fmt.Errorf("provider %q has no base URL — pass --base-url or set it in config", name)
	}

	model := f.model
	if model == "" {
		model = cfg.Model
		// The built-in config default is an Anthropic id; for any other provider
		// fall back to that provider's own default model.
		if model == "" || (model == config.Default().Model && name != "anthropic") {
			model = info.defaultModel
		}
	}
	if model == "" {
		return backend{}, fmt.Errorf("no model for provider %q — pass --model or set default_model in config", name)
	}
	return backend{name: name, model: model, baseURL: baseURL, native: info.native, headers: info.headers}, nil
}

// buildProvider constructs the resolved backend with its key (from env or the
// saved credentials file). Anthropic uses the native Messages API; every other
// provider is OpenAI-compatible at its base URL.
func buildProvider(b backend, key string) (provider.Provider, error) {
	if b.native {
		return provider.NewAnthropic(key, b.model, maxOutputTokens), nil
	}
	oc := provider.NewOpenAICompat(key, b.model, b.baseURL, maxOutputTokens)
	oc.SetHeaders(b.headers) // gateway/proxy auth headers, if any
	return oc, nil
}

// buildAgent wires the provider through the Trust Kernel. staticMode bakes the
// --mode preset into the executor Policy (for headless run/exec); when false
// (interactive chat) the mode is governed by the approver instead, so it can be
// switched mid-session with /mode.
func buildAgent(f *runFlags, approve tools.Approver, staticMode bool) (*agent.Agent, *tools.EditJournal, config.Config, func(), error) {
	noop := func() {}
	cfg, err := config.Load(f.dir)
	if err != nil {
		return nil, nil, cfg, noop, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, cfg, noop, fmt.Errorf("invalid config (.cliche/config.json): %w", err)
	}
	if os.Getenv("CLICHE_THEME") == "" && cfg.Theme != "" {
		style.ApplyTheme(cfg.Theme) // config palette (an explicit env theme wins)
	}
	b, err := resolveBackend(cfg, f)
	if err != nil {
		return nil, nil, cfg, noop, err
	}
	// Product mode: upgrade a weak/budget model to the provider's strong default
	// (unless the user explicitly pinned --model), and give the build more room to
	// iterate. The bump goes in BEFORE applyOrgPolicy so org governance can still
	// tighten it back down — never the other way around.
	if f.pro {
		if f.model == "" {
			if qm, bumped := qualityModel(b.name, b.model); bumped {
				b.model = qm
			}
		}
		if cfg.Governor.MaxTurns < 120 {
			cfg.Governor.MaxTurns = 120
		}
		if cfg.Governor.MaxWallClockSeconds < 3600 {
			cfg.Governor.MaxWallClockSeconds = 3600
		}
	}
	// Record the resolved backend so callers (and `verify`) reflect what actually
	// ran, not the pre-resolution defaults.
	cfg.Provider, cfg.Model = b.name, b.model

	key, _ := secrets.Lookup(b.name) // presence guaranteed by resolveBackend
	prov, err := buildProvider(b, key)
	if err != nil {
		return nil, nil, cfg, noop, err
	}

	// Fold CLI cap flags + apply org policy (tighten-only, fail-closed). Shared
	// with the swarm builder so neither path can bypass org governance. Flags are
	// folded into cfg here, so buildBudget/Governor take -1 (no re-override).
	cfg, err = applyOrgPolicy(cfg, f)
	if err != nil {
		return nil, nil, cfg, noop, err
	}
	bud := buildBudget(cfg, -1, -1)
	govLimits := buildGovernorLimits(cfg, -1)
	led, err := ledger.Open(config.Dir(f.dir))
	if err != nil {
		return nil, nil, cfg, noop, err
	}
	if !validMode(f.mode) {
		return nil, nil, cfg, noop, fmt.Errorf("unknown --mode %q (want plan | suggest | auto-edit | full)", f.mode)
	}
	rules, err := tools.ParseRules(cfg.Permissions.Allow, cfg.Permissions.Deny)
	if err != nil {
		return nil, nil, cfg, noop, fmt.Errorf("invalid permission rule in .cliche/config.json: %w", err)
	}
	journal := tools.NewEditJournal(f.dir)
	pol := tools.Policy{AllowWrite: f.allowWrite, AllowRun: f.allowRun, AllowWeb: f.allowWeb, Yolo: f.yolo, AllowOutsideRoot: f.allowOutsideRoot, Sandbox: f.sandbox}
	if staticMode {
		pol = applyMode(pol, f.mode) // headless: bake the mode into the policy
	}
	exec := tools.OSExecutor{
		Root:    f.dir,
		Policy:  pol,
		Approve: approve,
		Journal: journal,
		Rules:   rules,
		Egress:  tools.ParseEgress(cfg.Egress.Allow),
		// The project's pre-tool hook plus every plugin's, composed (any deny blocks).
		PreToolHook: buildPreToolHookChain(f.dir, append([]string{cfg.Hooks.PreToolUse}, pluginHookCommands(f.dir, "pre")...)),
		// Observe-only post-tool hooks (project + plugins).
		PostToolHook: buildPostToolHook(f.dir, append([]string{cfg.Hooks.PostToolUse}, pluginHookCommands(f.dir, "post")...)),
	}

	sys := "You are Cliche, a careful, autonomous coding agent. Work in small, verifiable steps.\n" +
		"- Read a file before you edit it. With edit_file, copy old_string verbatim from the file (exact text and indentation) and include enough surrounding lines that it matches exactly one place; make sure new_string leaves the file syntactically complete.\n" +
		"- After changing code, build it or run the tests to confirm — never claim a bug is fixed, a test passes, or a build is green without having run the command and seen the output.\n" +
		"- If a tool fails, read the error and adapt; for a rejected edit, re-read the file and copy the current text exactly.\n" +
		"- Do the work yourself with direct tool calls. Only delegate to a subagent for a genuinely large, ISOLATED investigation — never for a quick edit or a small fix, and never let a subagent edit a file you are also editing.\n" +
		"- Be concise and honest: if you are blocked or unsure, say so plainly instead of guessing." +
		baseStandard +
		proStandard(f.pro) +
		modeSystemNote(f.mode)
	// Inject a bounded repo map so the agent starts knowing the project layout
	// (it lands in the cached system block, so the token cost is largely one-time).
	if m, err := repomap.Build(f.dir, repoMapBudget); err == nil && m != "" {
		sys += "\n\nProject map (directories, files, and key Go symbols):\n" + m
	}
	sys += skillsSystemNote(f.dir)               // make .cliche/skills discoverable to the agent
	sys += learnSkillNote()                      // the learning loop: capture reusable workflows as skills
	sys += memory.SystemNote(memory.Load(f.dir)) // durable facts from earlier sessions
	wallClock := time.Duration(cfg.Governor.MaxWallClockSeconds) * time.Second
	acfg := agent.Config{
		System:             sys,
		Model:              b.model,
		MaxWallClock:       wallClock,
		ContextLimitTokens: cfg.Context.LimitTokens,
		ContextKeepRecent:  cfg.Context.KeepRecent,
		MaxSubagentDepth:   cfg.Subagents.MaxDepth,
		SubagentModel:      cfg.Subagents.Model,
	}
	a := agent.New(prov, bud, govLimits, led, exec, acfg)

	// Project MCP servers, plus any from installed plugins, plus connected OAuth
	// connectors (global — connect once, available everywhere).
	mcpServers := append(append([]config.MCPServer(nil), cfg.MCP...), pluginMCP(f.dir)...)
	mcpServers = append(mcpServers, connectorMCP()...)
	mcpSrc, cleanup, err := startMCP(mcpServers, f.yolo || f.allowMCP, approve)
	if err != nil {
		return nil, nil, cfg, noop, fmt.Errorf("mcp: %w", err)
	}
	if mcpSrc != nil {
		a.SetMCP(mcpSrc)
	}
	touchProject(f.dir) // record this project in the cross-project registry
	// Seal the audit ledger when the run/session finishes (signs the chain head).
	sealedCleanup := func() {
		sealLedgerDir(f.dir)
		cleanup()
	}
	return a, journal, cfg, sealedCleanup, nil
}

// cmdRun is the human-facing run command.
func cmdRun(args []string, out, errOut io.Writer) int {
	f, fs := parseRunFlags("run", args)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	prompt := resolvePrompt(f, fs)
	if prompt == "" {
		fmt.Fprintln(errOut, "run: a prompt is required, e.g. cliche run \"fix the build\"")
		return 2
	}

	// In a real terminal, prompt for permission on actions the flags don't
	// pre-authorize. When stdin is piped (CI/headless), there's no approver.
	var approve tools.Approver
	if !stdinIsPiped() {
		approve = (&approver{r: bufio.NewReader(os.Stdin), out: out}).Approve
	}

	a, journal, cfg, cleanup, err := buildAgent(f, approve, true) // run: static mode
	if err != nil {
		fmt.Fprintln(errOut, "run: "+err.Error())
		return 1
	}
	defer cleanup()
	// Streamed deltas are printed raw (no trailing newline); track an open block
	// so the next feed line (or the post-run summary) starts on a fresh line.
	streamOpen := false
	a.SetObserver(func(e agent.Event) {
		if e.Kind != "delta" && streamOpen {
			fmt.Fprintln(out)
			streamOpen = false
		}
		if e.Kind == "delta" {
			streamOpen = true
		}
		printEvent(out, e)
	})
	closeStream := func() {
		if streamOpen {
			fmt.Fprintln(out)
			streamOpen = false
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintln(out, gradientWordmark()+style.Gray(fmt.Sprintf("  %s · %s — caps + governor on · Ctrl-C to stop", cfg.Provider, cfg.Model)))
	if f.branch {
		startBranch(out, f.dir, "run-"+time.Now().UTC().Format("20060102-150405"))
	}
	runStart := time.Now()
	o, runErr := a.Run(ctx, prompt)
	closeStream()
	if runErr == nil {
		printChangeSummary(out, journal)
	}
	var findings []verifier.Finding
	if runErr == nil && f.verify && o.Stop == agent.StopCompleted {
		v := autoVerify(out, f.dir, cfg)
		o.Verdict, findings = v.Status, v.Findings
	}
	if runErr == nil && f.commit && o.Stop == agent.StopCompleted {
		commitChanges(out, f.dir, prompt, cfg.Model, o.Usage.USD)
	}
	if runErr == nil {
		runStopHook(out, f.dir, cfg.Hooks.Stop, o.Stop, o.Verdict)
		for _, c := range pluginHookCommands(f.dir, "stop") { // plugin stop hooks too
			runStopHook(out, f.dir, c, o.Stop, o.Verdict)
		}
	}
	printOutcome(out, o, findings, time.Since(runStart))
	if runErr != nil {
		return 1
	}
	return exitCodeFor(o)
}

// cmdExec is the headless command: JSON output, clean exit codes, fails loudly.
func cmdExec(args []string, out, errOut io.Writer) int {
	f, fs := parseRunFlags("exec", args)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	prompt := resolvePrompt(f, fs)
	if prompt == "" && stdinIsPiped() {
		// Read prompt from stdin (supports: git diff | cliche exec ...). Guarded
		// so an interactive TTY with no input does not hang waiting on EOF.
		if data, _ := io.ReadAll(os.Stdin); len(data) > 0 {
			prompt = strings.TrimSpace(string(data))
		}
	}
	if prompt == "" {
		writeJSON(errOut, map[string]any{"error": "no prompt provided (use -p, args, or stdin)"})
		return 2
	}

	a, _, cfg, cleanup, err := buildAgent(f, nil, true) // headless: static mode, no approver
	if err != nil {
		writeJSON(errOut, map[string]any{"error": err.Error()})
		return 1
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	o, runErr := a.Run(ctx, prompt)
	res := execResult{Outcome: o}
	if runErr == nil && f.verify && o.Stop == agent.StopCompleted {
		v := autoVerify(io.Discard, f.dir, cfg) // keep stdout clean JSON
		res.Outcome.Verdict = v.Status
		res.VerdictFindings = v.Findings
	}
	writeJSON(out, res)
	if runErr != nil {
		return 1
	}
	return exitCodeFor(res.Outcome)
}

// execResult is exec's JSON payload: the structured outcome plus any verifier
// findings, so CI can branch on the specific reward-hack rule that fired — not
// just the verdict status. VerdictFindings is omitted when there are none.
type execResult struct {
	agent.Outcome
	VerdictFindings []verifier.Finding `json:"verdict_findings,omitempty"`
}

// printChangeSummary lists the files a run created/modified/deleted, so a
// one-shot `run` ends with a visible record of what touched the working tree.
func printChangeSummary(out io.Writer, j *tools.EditJournal) {
	changes := j.Changes()
	if len(changes) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s\n", style.Gray(fmt.Sprintf("changed %d file(s):", len(changes))))
	for _, c := range changes {
		tag := "~"
		switch {
		case c.Deleted:
			tag = "-"
		case c.WasNew:
			tag = "+"
		}
		fmt.Fprintf(out, "  %s %s\n", style.Red(tag), style.White(c.Path))
	}
}

func printOutcome(out io.Writer, o agent.Outcome, findings []verifier.Finding, elapsed time.Duration) {
	// One-shot run: the final assistant text is already streamed above, so the
	// raw reason isn't repeated; this is the unified outcome summary.
	renderOutcome(out, o, outcomeMetrics{elapsed: elapsed, tokens: o.Usage.TotalTokens(), taskUSD: o.Usage.USD, sessionUSD: -1, findings: findings})
}

// resolvePrompt resolves the prompt with a consistent precedence for both run
// and exec: an explicit -p wins, then positional args.
func resolvePrompt(f *runFlags, fs *flag.FlagSet) string {
	if strings.TrimSpace(f.prompt) != "" {
		return strings.TrimSpace(f.prompt)
	}
	return strings.TrimSpace(strings.Join(fs.Args(), " "))
}

// stdinIsPiped reports whether stdin is a pipe or redirected file (not a TTY).
func stdinIsPiped() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

// exitCodeFor maps an outcome to a process exit code so CI can branch on it.
func exitCodeFor(o agent.Outcome) int {
	if o.Stop == agent.StopCompleted && o.Verdict == verifier.StatusFlagged {
		return 5 // completed, but auto-verify flagged the result
	}
	switch o.Stop {
	case agent.StopCompleted:
		return 0
	case agent.StopError:
		return 1 // provider/runtime error
	case agent.StopBudget:
		return 3 // budget cap reached
	case agent.StopCancelled:
		return 130 // interrupted (SIGINT convention)
	default:
		return 4 // a governor breaker tripped
	}
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
