// Package git is a thin, dependency-free wrapper over the `git` binary. Cliche
// uses it to make the agent's work reviewable and revertible: branch-per-task
// isolation and a generated commit whose body records the run's provenance
// (model, turns, cost), so the commit history and the cost ledger together form
// the "what the agent did" record. No third-party dependency — just os/exec.
package git

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Available reports whether the git binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func run(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// CurrentBranch returns the current branch name (empty on detached HEAD/error).
func CurrentBranch(dir string) string {
	out, _ := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if out == "HEAD" {
		return ""
	}
	return out
}

// HasChanges reports whether the work tree has uncommitted changes.
func HasChanges(dir string) bool {
	out, err := run(dir, "status", "--porcelain")
	return err == nil && out != ""
}

// CreateBranch creates and checks out a new branch from the current HEAD.
func CreateBranch(dir, name string) error {
	_, err := run(dir, "checkout", "-b", name)
	return err
}

// ShortStat returns a one-line summary of the working-tree diff (or "").
func ShortStat(dir string) string {
	out, _ := run(dir, "diff", "--shortstat")
	return out
}

// ChangedFiles lists the working-tree's changed paths (porcelain), capped to n
// (n <= 0 = no cap).
func ChangedFiles(dir string, n int) []string {
	out, err := run(dir, "status", "--porcelain")
	if err != nil || out == "" {
		return nil
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		// Porcelain v1 is "XY PATH"; parse status as the first token and PATH as
		// the rest. (We trim first because run() may have stripped the leading
		// status space on the first/last line.)
		line = strings.TrimSpace(line)
		if sp := strings.IndexByte(line, ' '); sp >= 0 {
			if path := strings.TrimSpace(line[sp+1:]); path != "" {
				files = append(files, path)
			}
		}
		if n > 0 && len(files) >= n {
			break
		}
	}
	return files
}

// Commit stages everything and commits with msg, returning the short hash and a
// one-line stat. Returns ("", "", nil) when there is nothing to commit.
func Commit(dir, msg string) (hash, stat string, err error) {
	if !HasChanges(dir) {
		return "", "", nil
	}
	stat = ShortStat(dir)
	if _, err = run(dir, "add", "-A"); err != nil {
		return "", "", err
	}
	if _, err = run(dir, "commit", "-m", msg); err != nil {
		return "", "", err
	}
	hash, _ = run(dir, "rev-parse", "--short", "HEAD")
	return hash, stat, nil
}
