package cli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/git"
)

// ghAvailable reports whether the GitHub CLI is installed (so Studio can offer
// "Open PR").
func ghAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// openPR pushes the current branch and opens a GitHub pull request via the gh
// CLI. Bounded by timeouts so a network stall can't hang the server.
func openPR(dir, title, body string) (string, error) {
	if !ghAvailable() {
		return "", fmt.Errorf("the GitHub CLI (gh) isn't installed — get it at cli.github.com, then `gh auth login`")
	}
	if !git.IsRepo(dir) {
		return "", fmt.Errorf("not a git repository")
	}
	branch := git.CurrentBranch(dir)
	if branch == "" {
		return "", fmt.Errorf("no current branch")
	}
	// Push the branch so the remote has it (best-effort — gh reports the real error if this matters).
	_, _ = runInDir(dir, 35*time.Second, "git", "push", "-u", "origin", branch)
	args := []string{"pr", "create"}
	if strings.TrimSpace(title) != "" {
		args = append(args, "--title", title, "--body", body)
	} else {
		args = append(args, "--fill")
	}
	out, err := runInDir(dir, 50*time.Second, "gh", args...)
	if err != nil {
		return "", fmt.Errorf("%s", prErrLine(out, err))
	}
	return prURL(out), nil
}

func runInDir(dir string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// prURL returns the first http(s) line gh printed (the created PR URL).
func prURL(out string) string {
	for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "http://") || strings.HasPrefix(ln, "https://") {
			return ln
		}
	}
	return strings.TrimSpace(out)
}

func prErrLine(out string, err error) string {
	for _, ln := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(ln); s != "" {
			return s
		}
	}
	return err.Error()
}
