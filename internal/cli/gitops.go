package cli

import (
	"fmt"
	"io"

	"github.com/mholovetskyi/cliche/internal/git"
	"github.com/mholovetskyi/cliche/internal/style"
)

// commitMessage builds a commit message whose body records the run's provenance,
// so the commit history doubles as part of the audit trail.
func commitMessage(subject, model string, usd float64) string {
	if subject == "" {
		subject = "changes"
	}
	if len(subject) > 64 {
		subject = subject[:63] + "…"
	}
	return fmt.Sprintf("cliche: %s\n\nProduced by cliche · %s · ~$%.4f", subject, model, usd)
}

// startBranch creates and checks out a fresh cliche/<id> branch when --branch is
// set and the dir is a git repo. Best effort — a failure is reported, not fatal.
func startBranch(out io.Writer, dir, id string) {
	if !git.Available() || !git.IsRepo(dir) {
		fmt.Fprintln(out, "  "+style.Gray("--branch: not a git repository — staying put"))
		return
	}
	name := "cliche/" + id
	if err := git.CreateBranch(dir, name); err != nil {
		fmt.Fprintln(out, "  "+style.Gray("--branch: "+err.Error()))
		return
	}
	fmt.Fprintln(out, "  "+style.Gray("branch: "+name))
}

// commitChanges stages + commits the working tree, printing the result.
func commitChanges(out io.Writer, dir, subject, model string, usd float64) {
	if !git.Available() || !git.IsRepo(dir) {
		return
	}
	hash, stat, err := git.Commit(dir, commitMessage(subject, model, usd))
	switch {
	case err != nil:
		fmt.Fprintln(out, "  commit failed: "+err.Error())
	case hash == "":
		// nothing to commit
	default:
		fmt.Fprintf(out, "  %s committed %s  %s\n", style.Red(gl("✔", "+")), style.White(hash), style.Gray(stat))
	}
}
