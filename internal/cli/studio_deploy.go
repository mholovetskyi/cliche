package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/git"
)

// deployPages publishes the project — a static site with an index.html at its
// root — to GitHub Pages via the gh CLI, returning the public URL. It's the
// "ship what I built" finish line for non-technical users: one click → a live
// link. Every step is gated with a clear, human error so a missing prerequisite
// reads as guidance, not a stack trace.
func deployPages(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return "", fmt.Errorf("no index.html at the project root — Pages needs a static site to publish")
	}
	if !ghAvailable() {
		return "", fmt.Errorf("the GitHub CLI (gh) isn't installed — get it at cli.github.com, then `gh auth login`")
	}
	if _, err := runInDir(dir, 20*time.Second, "gh", "auth", "status"); err != nil {
		return "", fmt.Errorf("not signed in to GitHub — run `gh auth login` once, then deploy")
	}

	// Ensure a git repo holding a commit of the current state.
	if !git.IsRepo(dir) {
		if out, err := runInDir(dir, 20*time.Second, "git", "init"); err != nil {
			return "", fmt.Errorf("git init failed: %s", prErrLine(out, err))
		}
	}
	_, _ = runInDir(dir, 20*time.Second, "git", "add", "-A")
	_, _ = runInDir(dir, 20*time.Second, "git", "commit", "-m", "Deploy via Cliché Studio") // ok if nothing to commit
	_, _ = runInDir(dir, 10*time.Second, "git", "branch", "-M", "main")

	owner, repo := ghRepo(dir)
	if repo == "" {
		// No GitHub repo yet → create one from this dir and push it.
		name := sanitizeRepoName(filepath.Base(dir))
		if out, err := runInDir(dir, 90*time.Second, "gh", "repo", "create", name, "--public", "--source=.", "--remote=origin", "--push"); err != nil {
			return "", fmt.Errorf("couldn't create the GitHub repo: %s", prErrLine(out, err))
		}
		owner, repo = ghRepo(dir)
	} else if out, err := runInDir(dir, 90*time.Second, "git", "push", "-u", "origin", "main"); err != nil {
		return "", fmt.Errorf("git push failed: %s", prErrLine(out, err))
	}
	if owner == "" || repo == "" {
		return "", fmt.Errorf("couldn't determine the GitHub repository for this project")
	}

	// Enable Pages from main/root — idempotent; a 409 ("already enabled") is fine.
	_, _ = runInDir(dir, 40*time.Second, "gh", "api", "-X", "POST",
		fmt.Sprintf("repos/%s/%s/pages", owner, repo),
		"-f", "source[branch]=main", "-f", "source[path]=/")

	return fmt.Sprintf("https://%s.github.io/%s/", owner, repo), nil
}

// ghRepo returns the owner and repo name of the dir's GitHub remote (or empties).
func ghRepo(dir string) (owner, repo string) {
	out, err := runInDir(dir, 20*time.Second, "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	if err != nil {
		return "", ""
	}
	if full := strings.TrimSpace(out); strings.Contains(full, "/") {
		i := strings.IndexByte(full, '/')
		return full[:i], full[i+1:]
	}
	return "", ""
}

// sanitizeRepoName makes a folder name safe to use as a GitHub repo name.
func sanitizeRepoName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "cliche-site"
	}
	return b.String()
}
