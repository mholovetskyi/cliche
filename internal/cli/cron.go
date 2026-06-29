package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/cron"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdCron is the scheduler surface: schedule prompts to run on a cron spec, then
// `cliche cron run` fires them — each through the FULL Trust Kernel (budget cap,
// governor, deny rules), so a scheduled agent can never run away. Jobs are stored
// per-project in .cliche/cron.json.
func cmdCron(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		return cronUsage(out)
	}
	switch args[0] {
	case "add":
		return cronAdd(args[1:], out, errOut)
	case "list", "ls":
		return cronList(args[1:], out, errOut)
	case "rm", "remove":
		return cronRemove(args[1:], out, errOut)
	case "run":
		return cronRun(args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "cron: unknown subcommand %q\n", args[0])
		return cronUsage(errOut)
	}
}

func cronUsage(w io.Writer) int {
	fmt.Fprintln(w, "Scheduled prompts — every fire runs through the Trust Kernel (can't run away).")
	fmt.Fprintln(w, "  cliche cron add \"<schedule>\" \"<prompt>\"   schedule a prompt (cron spec or @daily/@hourly/@every 30m)")
	fmt.Fprintln(w, "  cliche cron list                          list scheduled jobs + next fire")
	fmt.Fprintln(w, "  cliche cron rm <id>                       remove a job")
	fmt.Fprintln(w, "  cliche cron run                           run the scheduler (foreground; Ctrl-C to stop)")
	fmt.Fprintln(w, "  flags: --dir <project> --mode <full|auto-edit|plan> --max-usd <cap>")
	return 0
}

func cronAdd(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("cron add", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	mode := fs.String("mode", "full", "permission mode for the fire: full | auto-edit | plan")
	maxUSD := fs.Float64("max-usd", 0, "per-fire budget cap in USD (0 = the project's config cap)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) < 2 {
		fmt.Fprintln(errOut, "usage: cliche cron add [--dir .] [--mode full] [--max-usd 0] \"<schedule>\" \"<prompt>\"")
		return 2
	}
	spec, prompt := rest[0], strings.Join(rest[1:], " ")
	j, err := cron.Add(*dir, spec, prompt, *mode, *maxUSD)
	if err != nil {
		fmt.Fprintf(errOut, "cron add: %v\n", err)
		return 1
	}
	s, _ := cron.Parse(spec)
	fmt.Fprintf(out, "scheduled %s  (%s) — next fire %s\n", style.BoldWhite(j.ID), spec, nextStr(s))
	return 0
}

func cronList(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("cron list", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	jobs, err := cron.Load(*dir)
	if err != nil {
		fmt.Fprintf(errOut, "cron list: %v\n", err)
		return 1
	}
	if len(jobs) == 0 {
		fmt.Fprintln(out, "  no scheduled jobs — add one: cliche cron add \"@daily\" \"summarize today's git log\"")
		return 0
	}
	for _, j := range jobs {
		next := "?"
		if s, perr := cron.Parse(j.Spec); perr == nil {
			next = nextStr(s)
		}
		status := j.LastStatus
		if status == "" {
			status = "—"
		}
		state := ""
		if !j.Enabled {
			state = style.Gray(" (disabled)")
		}
		fmt.Fprintf(out, "  %s  %s  next %s  [%s]%s\n      %s\n",
			style.BoldWhite(j.ID), style.Gray(fmt.Sprintf("%-14s", j.Spec)), next, status, state, cronClip(j.Prompt, 70))
	}
	return 0
}

func cronRemove(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("cron rm", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(errOut, "usage: cliche cron rm [--dir .] <id>")
		return 2
	}
	ok, err := cron.Remove(*dir, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(errOut, "cron rm: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Fprintf(errOut, "cron rm: no job %q\n", fs.Arg(0))
		return 1
	}
	fmt.Fprintf(out, "removed %s\n", fs.Arg(0))
	return 0
}

// cronRun is the scheduler loop: fire the soonest-due job, one at a time (never
// overlapping), each through the Trust Kernel. Missed fires while it was down are
// NOT replayed — it computes the next fire from now, so a restart can't stampede.
func cronRun(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("cron run", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintf(out, "Cliché scheduler running for %s — every fire is bounded by the Trust Kernel. Ctrl-C to stop.\n", *dir)

	for {
		jobs, err := cron.Load(*dir)
		if err != nil {
			fmt.Fprintf(errOut, "cron: %v\n", err)
		}
		now := time.Now()
		var soonest time.Time
		var due cron.Job
		hasDue := false
		for _, j := range jobs {
			if !j.Enabled {
				continue
			}
			s, perr := cron.Parse(j.Spec)
			if perr != nil {
				continue
			}
			n := s.Next(now)
			if n.IsZero() {
				continue
			}
			if !hasDue || n.Before(soonest) {
				soonest, due, hasDue = n, j, true
			}
		}
		wait := time.Minute // no jobs → poll for newly-added ones
		if hasDue {
			if wait = time.Until(soonest); wait < 0 {
				wait = 0
			}
		}
		select {
		case <-ctx.Done():
			fmt.Fprintln(out, "scheduler stopped.")
			return 0
		case <-time.After(wait):
		}
		if !hasDue {
			continue
		}
		fmt.Fprintf(out, "\n[%s] firing %s — %s\n", time.Now().Format("15:04:05"), due.ID, cronClip(due.Prompt, 60))
		o := fireCronJob(ctx, *dir, due, out, errOut)
		cron.MarkRun(*dir, due.ID, o.Stop, time.Now())
	}
}

// fireCronJob runs one scheduled prompt headlessly through the normal agent +
// Trust Kernel. Unattended, so it auto-approves by mode (full = autonomous) — but
// the budget cap, governor, and deny rules still apply, so it cannot spiral.
func fireCronJob(ctx context.Context, dir string, j cron.Job, out, errOut io.Writer) agent.Outcome {
	f := &runFlags{dir: dir, maxUSD: -1, maxTokens: -1, maxTurns: -1}
	if j.MaxUSD > 0 {
		f.maxUSD = j.MaxUSD
	}
	switch j.Mode {
	case "auto-edit":
		f.allowWrite = true
	case "plan":
		f.mode = "plan"
	default: // "" or "full": autonomous, still Trust-Kernel-bounded
		f.yolo = true
	}
	a, _, _, cleanup, err := buildAgent(f, nil, true)
	if err != nil {
		fmt.Fprintf(errOut, "  build failed: %v\n", err)
		return agent.Outcome{Stop: "error"}
	}
	defer cleanup()
	o, runErr := a.Run(ctx, j.Prompt)
	if runErr != nil {
		fmt.Fprintf(errOut, "  run error: %v\n", runErr)
	}
	fmt.Fprintf(out, "  → %s · %d turns · $%.4f\n", o.Stop, o.Turns, o.Usage.USD)
	return o
}

func nextStr(s cron.Schedule) string { return s.Next(time.Now()).Format("Mon 2006-01-02 15:04") }

func cronClip(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
