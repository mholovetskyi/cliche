package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/git"
	"github.com/mholovetskyi/cliche/internal/ledger"
	sess "github.com/mholovetskyi/cliche/internal/session"
)

// reportData is the assembled, render-ready summary of a run.
type reportData struct {
	Title    string
	Provider string
	Model    string
	Summary  ledger.Summary
	Stat     string
	Files    []string
}

// cmdReport exports the audit ledger (enriched with the latest session and git
// changes) as a shareable Markdown verdict — the "what the agent did, what it
// cost, did it pass" artifact. It can be printed, written to a file, or posted
// straight onto a GitHub PR via gh.
func cmdReport(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dir := fs.String("dir", ".", "project root")
	sessionID := fs.String("session", "", "session id to title the report (default: latest)")
	title := fs.String("title", "", "override the report title")
	outFile := fs.String("out", "", "write the report to this file instead of stdout")
	pr := fs.String("pr", "", "post the report as a comment on this GitHub PR number (uses gh)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	led, err := ledger.Open(config.Dir(*dir))
	if err != nil {
		fmt.Fprintln(errOut, "report: "+err.Error())
		return 1
	}
	sum, err := led.Summarize()
	if err != nil {
		fmt.Fprintln(errOut, "report: "+err.Error())
		return 1
	}

	data := reportData{Summary: sum, Title: *title}
	// Enrich with the latest (or named) session for the title/provider/model.
	id := *sessionID
	if id == "" {
		id = sess.Latest(*dir)
	}
	if id != "" {
		if rec, err := sess.Load(*dir, id); err == nil {
			if data.Title == "" {
				data.Title = rec.Title
			}
			data.Provider, data.Model = rec.Provider, rec.Model
		}
	}
	if git.Available() && git.IsRepo(*dir) {
		data.Stat = git.ShortStat(*dir)
		data.Files = git.ChangedFiles(*dir, 20)
	}

	md := renderReport(data)

	if *pr != "" {
		if err := postPRComment(context.Background(), *dir, *pr, md); err != nil {
			fmt.Fprintln(errOut, "report: posting to PR: "+err.Error())
			return 1
		}
		fmt.Fprintf(out, "posted report to PR #%s\n", *pr)
		return 0
	}
	if *outFile != "" {
		if err := os.WriteFile(*outFile, []byte(md+"\n"), 0o644); err != nil {
			fmt.Fprintln(errOut, "report: "+err.Error())
			return 1
		}
		fmt.Fprintf(out, "wrote %s\n", *outFile)
		return 0
	}
	fmt.Fprintln(out, md)
	return 0
}

// renderReport assembles the Markdown. Pure (no IO) so it is unit-testable.
func renderReport(d reportData) string {
	var b strings.Builder
	b.WriteString("## 🤖 Cliche run report\n\n")
	if d.Title != "" {
		b.WriteString("**Task:** " + d.Title + "\n\n")
	}
	if d.Provider != "" || d.Model != "" {
		b.WriteString("**Model:** " + strings.TrimPrefix(d.Provider+" · "+d.Model, " · ") + "\n\n")
	}
	b.WriteString("**Verdict:** " + verdictLine(d.Summary.Verdicts) + "\n\n")

	b.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| Turns | %d |\n", d.Summary.Turns)
	fmt.Fprintf(&b, "| Tokens | %d in / %d out (%d total) |\n",
		d.Summary.InputTokens, d.Summary.OutputTokens, d.Summary.InputTokens+d.Summary.OutputTokens)
	fmt.Fprintf(&b, "| Est. cost | ~$%.4f |\n", d.Summary.USD)
	if d.Stat != "" {
		fmt.Fprintf(&b, "| Diff | %s |\n", d.Stat)
	}

	if len(d.Files) > 0 {
		b.WriteString("\n**Files changed:**\n")
		for _, f := range d.Files {
			b.WriteString("- `" + f + "`\n")
		}
	}
	b.WriteString("\n_Produced under a hard spend cap with a loop circuit-breaker and a reward-hack verifier — the Cliche Trust Kernel._")
	return b.String()
}

// verdictLine renders the verdict counts into a human line with an icon.
func verdictLine(v map[string]int) string {
	if len(v) == 0 {
		return "— (not verified)"
	}
	if n := v["flagged"]; n > 0 {
		return fmt.Sprintf("⚠️ flagged (%d)", n)
	}
	if n := v["verified"]; n > 0 {
		return fmt.Sprintf("✅ verified (%d)", n)
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v[k]))
	}
	return strings.Join(parts, ", ")
}

// postPRComment posts body as a comment on a GitHub PR using the gh CLI.
func postPRComment(ctx context.Context, dir, pr, body string) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("the GitHub CLI (gh) is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "comment", pr, "--body-file", "-")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(body)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
