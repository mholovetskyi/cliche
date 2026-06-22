package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/tools"
)

// runFlags are shared by `run` and `exec`.
type runFlags struct {
	model      string
	maxUSD     float64
	maxTokens  int
	maxTurns   int
	allowWrite bool
	allowRun   bool
	yolo       bool
	dir        string
	prompt     string // -p, used by exec
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
	fs.StringVar(&f.dir, "dir", ".", "project root")
	fs.StringVar(&f.prompt, "p", "", "prompt (headless)")
	return f, fs
}

// buildAgent wires a real (Anthropic) provider through the Trust Kernel.
func buildAgent(f *runFlags) (*agent.Agent, error) {
	cfg, err := config.Load(f.dir)
	if err != nil {
		return nil, err
	}
	model := cfg.Model
	if f.model != "" {
		model = f.model
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set — Cliche is BYO-key. Export your key and retry")
	}

	bud := buildBudget(cfg, f.maxUSD, f.maxTokens)
	gov := buildGovernor(cfg, f.maxTurns)
	led, err := ledger.Open(config.Dir(f.dir))
	if err != nil {
		return nil, err
	}
	exec := tools.OSExecutor{Policy: tools.Policy{AllowWrite: f.allowWrite, AllowRun: f.allowRun, Yolo: f.yolo}}

	maxTok := 4096
	prov := provider.NewAnthropic(apiKey, model, maxTok)

	sys := "You are Cliche, a careful coding agent. Be concise and honest. Never claim a test passes without evidence."
	wallClock := time.Duration(cfg.Governor.MaxWallClockSeconds) * time.Second
	return agent.New(prov, bud, gov, led, exec, agent.Config{System: sys, Model: model, MaxWallClock: wallClock}), nil
}

// cmdRun is the human-facing run command.
func cmdRun(args []string, out, errOut io.Writer) int {
	f, fs := parseRunFlags("run", args)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		prompt = f.prompt
	}
	if prompt == "" {
		fmt.Fprintln(errOut, "run: a prompt is required, e.g. cliche run \"fix the build\"")
		return 2
	}

	a, err := buildAgent(f)
	if err != nil {
		fmt.Fprintln(errOut, "run: "+err.Error())
		return 1
	}

	fmt.Fprintln(out, "cliche: trust kernel armed (caps + governor on). Running…")
	o, runErr := a.Run(context.Background(), prompt)
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
	prompt := f.prompt
	if prompt == "" {
		prompt = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
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

	a, err := buildAgent(f)
	if err != nil {
		writeJSON(errOut, map[string]any{"error": err.Error()})
		return 1
	}

	o, runErr := a.Run(context.Background(), prompt)
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
	switch o.Stop {
	case agent.StopCompleted:
		return 0
	case agent.StopError:
		return 1 // provider/runtime error
	case agent.StopBudget:
		return 3 // budget cap reached
	default:
		return 4 // a governor breaker tripped
	}
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
