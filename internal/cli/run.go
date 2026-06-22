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
	"github.com/mholovetskyi/cliche/internal/tools"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

// runFlags are shared by `run` and `exec`.
type runFlags struct {
	model            string
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

// buildAgent wires a real (Anthropic) provider through the Trust Kernel. The
// approve callback (may be nil) handles permission prompts for actions the
// flags don't pre-authorize.
func buildAgent(f *runFlags, approve tools.Approver) (*agent.Agent, config.Config, func(), error) {
	noop := func() {}
	cfg, err := config.Load(f.dir)
	if err != nil {
		return nil, cfg, noop, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, cfg, noop, fmt.Errorf("invalid config (.cliche/config.json): %w", err)
	}
	model := cfg.Model
	if f.model != "" {
		model = f.model
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, cfg, noop, fmt.Errorf("ANTHROPIC_API_KEY is not set — Cliche is BYO-key. Export your key and retry")
	}

	bud := buildBudget(cfg, f.maxUSD, f.maxTokens)
	govLimits := buildGovernorLimits(cfg, f.maxTurns)
	led, err := ledger.Open(config.Dir(f.dir))
	if err != nil {
		return nil, cfg, noop, err
	}
	exec := tools.OSExecutor{
		Root:    f.dir,
		Policy:  tools.Policy{AllowWrite: f.allowWrite, AllowRun: f.allowRun, Yolo: f.yolo, AllowOutsideRoot: f.allowOutsideRoot},
		Approve: approve,
	}

	prov := provider.NewAnthropic(apiKey, model, 4096)
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
		return nil, cfg, noop, fmt.Errorf("mcp: %w", err)
	}
	if mcpSrc != nil {
		a.SetMCP(mcpSrc)
	}
	return a, cfg, cleanup, nil
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

	a, cfg, cleanup, err := buildAgent(f, approve)
	if err != nil {
		fmt.Fprintln(errOut, "run: "+err.Error())
		return 1
	}
	defer cleanup()
	a.SetObserver(func(e agent.Event) { printEvent(out, e) })

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintln(out, "cliche: trust kernel armed (caps + governor on). Running… (Ctrl-C to stop)")
	o, runErr := a.Run(ctx, prompt)
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

	a, cfg, cleanup, err := buildAgent(f, nil) // headless: no interactive approver
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

func printOutcome(out io.Writer, o agent.Outcome) {
	fmt.Fprintf(out, "\nstop:   %s\n", o.Stop)
	if o.Reason != "" {
		fmt.Fprintf(out, "reason: %s\n", o.Reason)
	}
	fmt.Fprintf(out, "turns:  %d\n", o.Turns)
	fmt.Fprintf(out, "usage:  %d tokens, ~$%.4f (estimated)\n", o.Usage.TotalTokens(), o.Usage.USD)
	if o.Verdict != "" {
		fmt.Fprintf(out, "verify: %s\n", o.Verdict)
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
