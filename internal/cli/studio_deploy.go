package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/git"
)

// deployTarget publishes the built project to the chosen host and returns the
// live URL. "pages" (default) uses GitHub Pages via gh; "vercel" and "netlify"
// shell out to their CLIs through npx, reading a token from the environment.
// Every missing prerequisite is a clear, human error so it reads as guidance.
func deployTarget(dir, target string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "", "pages", "github":
		return deployPages(dir)
	case "vercel":
		return deployVercel(dir)
	case "netlify":
		return deployNetlify(dir)
	default:
		return "", fmt.Errorf("unknown deploy target %q — use pages, vercel, or netlify", target)
	}
}

func deployVercel(dir string) (string, error) {
	token := strings.TrimSpace(os.Getenv("VERCEL_TOKEN"))
	if token == "" {
		return "", fmt.Errorf("set VERCEL_TOKEN (vercel.com → Account Settings → Tokens) to deploy to Vercel")
	}
	if !cmdAvailable("npx") {
		return "", fmt.Errorf("Node's npx isn't installed — install Node.js, then deploy (Cliché runs the Vercel CLI via npx)")
	}
	out, err := runInDir(dir, 6*time.Minute, "npx", "--yes", "vercel", "deploy", "--prod", "--yes", "--token", token)
	if err != nil {
		return "", fmt.Errorf("vercel deploy failed: %s", prErrLine(out, err))
	}
	if url := lastURL(out); url != "" {
		return url, nil
	}
	return "", fmt.Errorf("vercel deployed but returned no URL:\n%s", clip(out, 300))
}

func deployNetlify(dir string) (string, error) {
	token := strings.TrimSpace(os.Getenv("NETLIFY_AUTH_TOKEN"))
	if token == "" {
		return "", fmt.Errorf("set NETLIFY_AUTH_TOKEN (Netlify → User settings → Applications → personal access tokens) to deploy to Netlify")
	}
	if !cmdAvailable("npx") {
		return "", fmt.Errorf("Node's npx isn't installed — install Node.js, then deploy (Cliché runs the Netlify CLI via npx)")
	}
	// Prefer a built output directory if one exists; otherwise publish the root.
	pub := dir
	for _, d := range []string{"dist", "build", "out"} {
		if _, err := os.Stat(filepath.Join(dir, d, "index.html")); err == nil {
			pub = filepath.Join(dir, d)
			break
		}
	}
	out, err := runInDir(dir, 6*time.Minute, "npx", "--yes", "netlify-cli", "deploy", "--prod", "--dir", pub, "--json", "--auth", token)
	if err != nil {
		return "", fmt.Errorf("netlify deploy failed: %s", prErrLine(out, err))
	}
	if url := parseNetlifyURL(out); url != "" {
		return url, nil
	}
	if url := lastURL(out); url != "" {
		return url, nil
	}
	return "", fmt.Errorf("netlify deployed but returned no URL:\n%s", clip(out, 300))
}

func cmdAvailable(name string) bool { _, err := exec.LookPath(name); return err == nil }

var urlRe = regexp.MustCompile(`https?://[^\s"']+`)

// lastURL returns the last http(s) URL in s — for Vercel/Netlify CLIs that print
// the production URL last after build/inspect lines.
func lastURL(s string) string {
	m := urlRe.FindAllString(s, -1)
	if len(m) == 0 {
		return ""
	}
	return strings.TrimRight(m[len(m)-1], ".,)")
}

// parseNetlifyURL reads the live URL from `netlify deploy --json` output, which
// is a JSON object carrying url / deploy_url (the CLI may prefix it with logs).
func parseNetlifyURL(out string) string {
	i := strings.IndexByte(out, '{')
	if i < 0 {
		return ""
	}
	var r struct {
		URL       string `json:"url"`
		DeployURL string `json:"deploy_url"`
	}
	if json.Unmarshal([]byte(out[i:]), &r) != nil {
		return ""
	}
	if r.URL != "" {
		return r.URL
	}
	return r.DeployURL
}

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
