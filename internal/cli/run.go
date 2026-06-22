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
	fs.StringVar(&f.provider, "provider", "", "anthropic | openrouter | openai")
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

// buildProvider selects the model backend (BYO-key, provider-neutral).
func buildProvider(cfg config.Config, f *runFlags, model string) (provider.Provider, error) {
	name := f.provider
	if name == "" {
		name = cfg.Provider
	}
	baseURL := func(def string) string {
		if f.baseURL != "" {
			return f.baseURL
		}
		if cfg.BaseURL != "" {
			return cfg.BaseURL
		}
		return def
	}
	switch name {
	case "", "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set — Cliche is BYO-key. Export your key and retry")
		}
		return provider.NewAnthropic(key, model, 4096), nil
	case "openrouter":
		key := os.Getenv("OPENROUTER_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY is not set")
		}
		return provider.NewOpenAICompat(key, model, baseURL("https://openrouter.ai/api/v1/chat/completions"), 4096), nil
	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is not set")
		}
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
	model := cfg.Model
	if f.model != "" {
		model = f.model
	}

	prov, err := buildProvider(cfg, f, model)
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
	fmt.Fprintln(out, wordmark()+style.Gray("  trust kernel armed — caps + governor on · Ctrl-C to stop"))
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
