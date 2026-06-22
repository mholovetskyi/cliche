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
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/tools"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

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
	yolo             bool
	verify           bool
	allowOutsideRoot bool
	allowMCP         bool
	dir              string
	prompt           string // -p, used by exec
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
	fs.BoolVar(&f.yolo, "yolo", false, "skip approvals (never the budget cap or governor)")
	fs.BoolVar(&f.verify, "verify", false, "after completion, re-run tests and report a verdict")
	fs.BoolVar(&f.allowOutsideRoot, "allow-outside-root", false, "permit file access outside the project root")
	fs.BoolVar(&f.allowMCP, "allow-mcp", false, "permit MCP tool calls without asking")
	fs.StringVar(&f.dir, "dir", ".", "project root")
	fs.StringVar(&f.prompt, "p", "", "prompt (headless)")
	return f, fs
}

// supportedProviders is the BYO-key backend list, in the precedence used for
// auto-detection.
var supportedProviders = []string{"anthropic", "openrouter", "openai"}

// providerKeyEnv maps a provider to the environment variable holding its key.
func providerKeyEnv(name string) string { return secrets.EnvVar(name) }

// hasProviderKey reports whether a key for a provider is configured (env var or
// the saved credentials file).
func hasProviderKey(name string) bool {
	key, _ := secrets.Lookup(name)
	return key != ""
}

// firstProviderWithKey returns the first supported provider that has a key set,
// or "" if none do.
func firstProviderWithKey() string {
	for _, p := range supportedProviders {
		if hasProviderKey(p) {
			return p
		}
	}
	return ""
}

// defaultModelFor returns a sensible default model id for a provider — used when
// the provider is chosen (or auto-detected) but no model was specified, so a
// non-Anthropic provider doesn't inherit an Anthropic-only model id. The
// non-Anthropic defaults favor a cheap, tool-capable model that works on a
// free/low-credit account out of the box; pick a stronger one with --model or
// in .cliche/config.json once you have credits.
func defaultModelFor(name string) string {
	switch name {
	case "openrouter":
		return "openai/gpt-4o-mini"
	case "openai":
		return "gpt-4o-mini"
	default:
		return "claude-sonnet-4-6"
	}
}

// resolveBackend picks the provider and model. Cliche is provider-neutral and
// BYO-key, so it must not be Anthropic-by-default in practice: when the user
// hasn't explicitly chosen a provider (no --provider) and the default
// provider's key is absent, it auto-detects from whichever supported key IS
// set. An explicit --provider is always respected (and errors if its key is
// missing). With no key at all, the error names every option.
func resolveBackend(cfg config.Config, f *runFlags) (prov, model string, err error) {
	prov = f.provider
	explicit := prov != ""
	if prov == "" {
		prov = cfg.Provider
	}
	if prov == "" {
		prov = "anthropic"
	}

	// Auto-correct a soft (non-explicit) provider to one we actually have a key
	// for, so `cliche chat` just works when only OPENROUTER_API_KEY is set.
	if !explicit && !hasProviderKey(prov) {
		if alt := firstProviderWithKey(); alt != "" {
			prov = alt
		}
	}
	if !hasProviderKey(prov) {
		return "", "", fmt.Errorf(
			"no API key configured. Cliche is BYO-key — set one up once with:\n" +
				"    cliche auth openrouter            (then paste your key, or --from-file <path>)\n" +
				"  or, for this shell only, export ANTHROPIC_API_KEY / OPENROUTER_API_KEY / OPENAI_API_KEY.\n" +
				"  (pass --provider to force a specific backend.)")
	}

	model = f.model
	if model == "" {
		model = cfg.Model
		// If the model is still the built-in default but the provider isn't
		// Anthropic, the default id won't exist there — pick the provider's own.
		if model == "" || (model == config.Default().Model && prov != "anthropic") {
			model = defaultModelFor(prov)
		}
	}
	return prov, model, nil
}

// buildProvider constructs the selected, already-resolved backend with the
// resolved key (BYO-key — from env or the saved credentials file).
func buildProvider(name, model, key, baseURLOverride string) (provider.Provider, error) {
	baseURL := func(def string) string {
		if baseURLOverride != "" {
			return baseURLOverride
		}
		return def
	}
	switch name {
	case "", "anthropic":
		return provider.NewAnthropic(key, model, 4096), nil
	case "openrouter":
		return provider.NewOpenAICompat(key, model, baseURL("https://openrouter.ai/api/v1/chat/completions"), 4096), nil
	case "openai":
		return provider.NewOpenAICompat(key, model, baseURL("https://api.openai.com/v1/chat/completions"), 4096), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (want anthropic|openrouter|openai)", name)
	}
}

// buildAgent wires the selected provider through the Trust Kernel. The approve
// callback (may be nil) handles permission prompts for actions the flags don't
// pre-authorize.
func buildAgent(f *runFlags, approve tools.Approver) (*agent.Agent, *tools.EditJournal, config.Config, func(), error) {
	noop := func() {}
	cfg, err := config.Load(f.dir)
	if err != nil {
		return nil, nil, cfg, noop, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, cfg, noop, fmt.Errorf("invalid config (.cliche/config.json): %w", err)
	}
	provName, model, err := resolveBackend(cfg, f)
	if err != nil {
		return nil, nil, cfg, noop, err
	}
	// Record the resolved backend so callers (and `verify`) reflect what actually
	// ran, not the pre-resolution defaults.
	cfg.Provider, cfg.Model = provName, model

	baseURLOverride := f.baseURL
	if baseURLOverride == "" {
		baseURLOverride = cfg.BaseURL
	}
	key, _ := secrets.Lookup(provName) // presence guaranteed by resolveBackend
	prov, err := buildProvider(provName, model, key, baseURLOverride)
	if err != nil {
		return nil, nil, cfg, noop, err
	}

	bud := buildBudget(cfg, f.maxUSD, f.maxTokens)
	govLimits := buildGovernorLimits(cfg, f.maxTurns)
	led, err := ledger.Open(config.Dir(f.dir))
	if err != nil {
		return nil, nil, cfg, noop, err
	}
	journal := tools.NewEditJournal(f.dir)
	exec := tools.OSExecutor{
		Root:    f.dir,
		Policy:  tools.Policy{AllowWrite: f.allowWrite, AllowRun: f.allowRun, Yolo: f.yolo, AllowOutsideRoot: f.allowOutsideRoot},
		Approve: approve,
		Journal: journal,
	}

	sys := "You are Cliche, a careful coding agent. Be concise and honest. Use the provided tools to read, edit, and run code. Never claim a test passes without evidence."
	wallClock := time.Duration(cfg.Governor.MaxWallClockSeconds) * time.Second
	acfg := agent.Config{
		System:             sys,
		Model:              model,
		MaxWallClock:       wallClock,
		ContextLimitTokens: cfg.Context.LimitTokens,
		ContextKeepRecent:  cfg.Context.KeepRecent,
		MaxSubagentDepth:   cfg.Subagents.MaxDepth,
	}
	a := agent.New(prov, bud, govLimits, led, exec, acfg)

	mcpSrc, cleanup, err := startMCP(cfg.MCP, f.yolo || f.allowMCP, approve)
	if err != nil {
		return nil, nil, cfg, noop, fmt.Errorf("mcp: %w", err)
	}
	if mcpSrc != nil {
		a.SetMCP(mcpSrc)
	}
	return a, journal, cfg, cleanup, nil
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

	a, journal, cfg, cleanup, err := buildAgent(f, approve)
	if err != nil {
		fmt.Fprintln(errOut, "run: "+err.Error())
		return 1
	}
	defer cleanup()
	a.SetObserver(func(e agent.Event) { printEvent(out, e) })

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintln(out, gradientWordmark()+style.Gray(fmt.Sprintf("  %s · %s — caps + governor on · Ctrl-C to stop", cfg.Provider, cfg.Model)))
	o, runErr := a.Run(ctx, prompt)
	if runErr == nil {
		printChangeSummary(out, journal)
	}
	if runErr == nil && f.verify && o.Stop == agent.StopCompleted {
		o.Verdict = autoVerify(out, f.dir, cfg).Status
	}
	printOutcome(out, o)
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

	a, _, cfg, cleanup, err := buildAgent(f, nil) // headless: no interactive approver
	if err != nil {
		writeJSON(errOut, map[string]any{"error": err.Error()})
		return 1
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	o, runErr := a.Run(ctx, prompt)
	if runErr == nil && f.verify && o.Stop == agent.StopCompleted {
		o.Verdict = autoVerify(io.Discard, f.dir, cfg).Status // keep stdout clean JSON
	}
	writeJSON(out, o)
	if runErr != nil {
		return 1
	}
	return exitCodeFor(o)
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

func printOutcome(out io.Writer, o agent.Outcome) {
	stop := style.BoldWhite(o.Stop)
	if o.Stop != agent.StopCompleted {
		stop = style.BoldRed(o.Stop)
	}
	fmt.Fprintf(out, "\n%s %s\n", style.Gray("stop:  "), stop)
	if o.Reason != "" {
		fmt.Fprintf(out, "%s %s\n", style.Gray("reason:"), o.Reason)
	}
	fmt.Fprintf(out, "%s %d\n", style.Gray("turns: "), o.Turns)
	fmt.Fprintf(out, "%s %s\n", style.Gray("usage: "), style.White(fmt.Sprintf("%d tokens, ~$%.4f (estimated)", o.Usage.TotalTokens(), o.Usage.USD)))
	if o.Verdict != "" {
		fmt.Fprintln(out, verdictStyled(o.Verdict))
	}
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
