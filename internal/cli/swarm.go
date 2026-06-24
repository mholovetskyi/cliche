package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/tools"
)

// Swarm orchestration: a deterministic multi-agent pipeline — a PLANNER
// decomposes the task into independent subtasks, EXECUTORS work them in
// parallel, and a SYNTHESIZER combines the results. Unlike model-driven
// subagents, the control flow is code, not a tool the model chooses. Every
// agent shares ONE Budget Kernel (so the whole swarm is capped), one ledger,
// and one permission-gated executor — the Trust Kernel wraps the fleet, not
// just a single agent.

const (
	plannerSystem = "You are the PLANNER in a multi-agent swarm. Break the user's task into a small set (2–5) of subtasks that are each SELF-CONTAINED and run in PARALLEL — they must NOT depend on one another's output. Do NOT produce sequential steps (e.g. 'first list the files, then read them, then analyze' is WRONG — those are dependent stages, not parallel subtasks). Prefer splitting by independent unit of work (a file, a component, a question). If the task genuinely cannot be split, return a single subtask. Reply with ONLY a JSON array of short, imperative subtask strings — no prose, no markdown fences. Do not do the work yourself."

	executorSystem = "You are one EXECUTOR in a swarm working a larger task. Do EXACTLY the single subtask you are given — use the tools to read, edit, and run code as needed — then report what you did and what you found, concisely. Stay strictly within your subtask; do not attempt the others."

	synthSystem = "You are the SYNTHESIZER in a multi-agent swarm. You are given the original task and each executor's result. Combine them into one coherent, deduplicated answer that completes the original task. Reconcile conflicts and call out any gaps honestly."
)

// swarmRunner runs one role-scoped agent turn and returns its outcome. It is the
// single seam the pipeline depends on, so the orchestration is unit-testable
// with a fake (no provider, no network).
type swarmRunner func(ctx context.Context, roleSystem, prompt string) (agent.Outcome, error)

type swarm struct {
	run    swarmRunner
	out    io.Writer
	maxSub int // cap on subtasks (0 = no cap)
}

type subtaskResult struct {
	subtask string
	output  string
	stop    string
	turns   int
	err     error
}

// execute runs the full plan → execute → synthesize pipeline and returns the
// synthesized answer plus the synthesizer's outcome (for the cost summary/exit).
func (s *swarm) execute(ctx context.Context, task string) (string, agent.Outcome, error) {
	// 1) PLAN
	fmt.Fprintf(s.out, "\n  %s %s\n", style.BoldWhite(gl("⬡", "#")+" swarm"), style.Gray("· planning"))
	planOut, err := s.run(ctx, plannerSystem, "Task:\n"+task)
	if err != nil {
		return "", planOut, err
	}
	subtasks := parsePlan(planOut.Reason)
	if s.maxSub > 0 && len(subtasks) > s.maxSub {
		fmt.Fprintf(s.out, "  %s\n", style.Gray(fmt.Sprintf("capping %d subtasks to %d", len(subtasks), s.maxSub)))
		subtasks = subtasks[:s.maxSub]
	}
	fmt.Fprintf(s.out, "  %s %s\n", style.Green("◇"), style.White(fmt.Sprintf("%d subtasks", len(subtasks))))
	for i, st := range subtasks {
		fmt.Fprintf(s.out, "    %s %s\n", style.Gray(fmt.Sprintf("%d.", i+1)), clip(st, 76))
	}

	// 2) EXECUTE (parallel — each executor shares the swarm's budget/ledger)
	fmt.Fprintf(s.out, "  %s %s\n", style.Green("◇"), style.Gray("executing in parallel"))
	results := make([]subtaskResult, len(subtasks))
	var wg sync.WaitGroup
	for i, st := range subtasks {
		wg.Add(1)
		go func(i int, st string) {
			defer wg.Done()
			o, err := s.run(ctx, executorSystem, st)
			results[i] = subtaskResult{subtask: st, output: o.Reason, stop: o.Stop, turns: o.Turns, err: err}
		}(i, st)
	}
	wg.Wait()
	for _, r := range results {
		mark := style.Green(gl("✓", "ok"))
		if r.err != nil || r.stop != agent.StopCompleted {
			mark = style.Red(gl("✗", "x"))
		}
		fmt.Fprintf(s.out, "    %s %s\n", mark, style.Gray(clip(r.subtask, 70)))
	}

	// 3) SYNTHESIZE
	fmt.Fprintf(s.out, "  %s %s\n", style.Green("◇"), style.Gray("synthesizing"))
	synthOut, err := s.run(ctx, synthSystem, synthPrompt(task, results))
	if err != nil {
		return "", synthOut, err
	}

	// Honest accounting: the cost summary should reflect the WHOLE swarm, not
	// just the synthesizer. Usage is already cumulative (every agent shares one
	// budget kernel), but turns are per-agent — sum them so "N turns" is true.
	total := planOut.Turns + synthOut.Turns
	for _, r := range results {
		total += r.turns
	}
	synthOut.Turns = total
	fmt.Fprintf(s.out, "  %s %s\n", style.Gray("◇"),
		style.Gray(fmt.Sprintf("%d agents · 1 planner + %d executors + 1 synthesizer", len(subtasks)+2, len(subtasks))))
	return synthOut.Reason, synthOut, nil
}

// synthPrompt formats the original task plus every executor result for the
// synthesizer.
func synthPrompt(task string, results []subtaskResult) string {
	var b strings.Builder
	b.WriteString("Original task:\n")
	b.WriteString(task)
	b.WriteString("\n\nExecutor results:\n")
	for i, r := range results {
		fmt.Fprintf(&b, "\n[%d] subtask: %s\n", i+1, r.subtask)
		out := strings.TrimSpace(r.output)
		if r.err != nil {
			out = "(failed: " + r.err.Error() + ")"
		} else if r.stop != agent.StopCompleted {
			out = "(halted: " + r.stop + ") " + out
		}
		if out == "" {
			out = "(no output)"
		}
		b.WriteString("result: ")
		b.WriteString(out)
		b.WriteString("\n")
	}
	b.WriteString("\nSynthesize a single final answer to the original task.")
	return b.String()
}

var planListRe = regexp.MustCompile(`^\s*(?:\d+[.)]|[-*•])\s+(.*\S)\s*$`)

// parsePlan extracts the subtask list from the planner's free text, tolerating
// JSON (a bare array or {"subtasks": [...]}) or a numbered/bulleted list. As a
// last resort the whole text becomes a single subtask, so the pipeline always
// makes progress.
func parsePlan(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var obj struct {
		Subtasks []string `json:"subtasks"`
	}
	if json.Unmarshal([]byte(text), &obj) == nil && len(obj.Subtasks) > 0 {
		return cleanPlan(obj.Subtasks)
	}
	if arr := jsonStringArray(text); len(arr) > 0 {
		return cleanPlan(arr)
	}
	if lines := listLines(text); len(lines) > 0 {
		return cleanPlan(lines)
	}
	return []string{text}
}

func jsonStringArray(text string) []string {
	i := strings.IndexByte(text, '[')
	j := strings.LastIndexByte(text, ']')
	if i < 0 || j <= i {
		return nil
	}
	var arr []string
	if json.Unmarshal([]byte(text[i:j+1]), &arr) == nil {
		return arr
	}
	return nil
}

func listLines(text string) []string {
	var out []string
	for _, ln := range strings.Split(text, "\n") {
		if m := planListRe.FindStringSubmatch(ln); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

func cleanPlan(items []string) []string {
	var out []string
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func clip(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// buildSwarmRunner wires the Trust Kernel once and returns a runner that builds
// a fresh role-scoped agent per call, all sharing the same budget cap, ledger,
// and permission-gated executor. Swarm agents do not spawn nested subagents
// (MaxSubagentDepth 0) — the swarm IS the fan-out.
func buildSwarmRunner(f *runFlags, approve tools.Approver) (swarmRunner, config.Config, error) {
	cfg, err := config.Load(f.dir)
	if err != nil {
		return nil, cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, cfg, fmt.Errorf("invalid config (.cliche/config.json): %w", err)
	}
	if os.Getenv("CLICHE_THEME") == "" && cfg.Theme != "" {
		style.ApplyTheme(cfg.Theme)
	}
	if !validMode(f.mode) {
		return nil, cfg, fmt.Errorf("unknown --mode %q (want plan | suggest | auto-edit | full)", f.mode)
	}
	b, err := resolveBackend(cfg, f)
	if err != nil {
		return nil, cfg, err
	}
	cfg.Provider, cfg.Model = b.name, b.model
	key, _ := secrets.Lookup(b.name)
	prov, err := buildProvider(b, key)
	if err != nil {
		return nil, cfg, err
	}
	bud := buildBudget(cfg, f.maxUSD, f.maxTokens) // ONE cap shared by every swarm agent
	govLimits := buildGovernorLimits(cfg, f.maxTurns)
	led, err := ledger.Open(config.Dir(f.dir))
	if err != nil {
		return nil, cfg, err
	}
	rules, err := tools.ParseRules(cfg.Permissions.Allow, cfg.Permissions.Deny)
	if err != nil {
		return nil, cfg, fmt.Errorf("invalid permission rule in .cliche/config.json: %w", err)
	}
	journal := tools.NewEditJournal(f.dir)
	pol := applyMode(tools.Policy{
		AllowWrite: f.allowWrite, AllowRun: f.allowRun, AllowWeb: f.allowWeb,
		Yolo: f.yolo, AllowOutsideRoot: f.allowOutsideRoot, Sandbox: f.sandbox,
	}, f.mode)
	exec := tools.OSExecutor{
		Root:         f.dir,
		Policy:       pol,
		Approve:      approve,
		Journal:      journal,
		Rules:        rules,
		Egress:       tools.ParseEgress(cfg.Egress.Allow),
		PreToolHook:  buildPreToolHookChain(f.dir, append([]string{cfg.Hooks.PreToolUse}, pluginHookCommands(f.dir, "pre")...)),
		PostToolHook: buildPostToolHook(f.dir, append([]string{cfg.Hooks.PostToolUse}, pluginHookCommands(f.dir, "post")...)),
	}
	wallClock := time.Duration(cfg.Governor.MaxWallClockSeconds) * time.Second

	run := func(ctx context.Context, roleSystem, prompt string) (agent.Outcome, error) {
		acfg := agent.Config{
			System:             roleSystem,
			Model:              b.model,
			MaxWallClock:       wallClock,
			ContextLimitTokens: cfg.Context.LimitTokens,
			ContextKeepRecent:  cfg.Context.KeepRecent,
			MaxSubagentDepth:   0, // the swarm is the fan-out; no nested spawning
		}
		a := agent.New(prov, bud, govLimits, led, exec, acfg)
		return a.Run(ctx, prompt)
	}
	touchProject(f.dir) // record this project in the cross-project registry
	return run, cfg, nil
}

// cmdSwarm is `cliche swarm "<task>"`: a deterministic planner→executors→
// synthesizer multi-agent run, capped as one swarm.
func cmdSwarm(args []string, out, errOut io.Writer) int {
	f, fs := parseRunFlags("swarm", args)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	task := resolvePrompt(f, fs)
	if task == "" {
		fmt.Fprintln(errOut, "swarm: a task is required, e.g. cliche swarm \"audit the codebase for race conditions\"")
		return 2
	}

	var approve tools.Approver
	if !stdinIsPiped() {
		approve = (&approver{r: bufio.NewReader(os.Stdin), out: out}).Approve
	}

	run, _, err := buildSwarmRunner(f, approve)
	if err != nil {
		fmt.Fprintln(errOut, "swarm: "+err.Error())
		return 1
	}

	start := time.Now()
	sw := &swarm{run: run, out: out, maxSub: 5}
	final, outcome, err := sw.execute(context.Background(), task)
	if err != nil {
		fmt.Fprintln(errOut, "swarm: "+err.Error())
		return 1
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, renderMarkdown(final))
	printOutcome(out, outcome, nil, time.Since(start))
	return exitCodeFor(outcome)
}
