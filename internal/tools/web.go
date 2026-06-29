package tools

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// web_fetch lets the agent pull current documentation/pages into context — a
// baseline coding-CLI tool. It is network egress, so it is permission-gated
// (--allow-web / approval), constrained by an optional host allowlist (Egress),
// and bounded. Pure stdlib (net/http).

const maxFetchBytes = 200_000 // cap fetched/extracted text fed back to the model

var (
	scriptStyleRe = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)
	tagRe         = regexp.MustCompile(`(?s)<[^>]+>`)
	wsRe          = regexp.MustCompile(`[ \t]+`)
	blankLinesRe  = regexp.MustCompile(`\n{3,}`)
)

// permitWeb gates a network fetch: --allow-web / --yolo pre-authorize, else the
// approver is asked. Plan mode does NOT block reads like fetching.
func (e OSExecutor) permitWeb(url string) bool {
	if e.Policy.Yolo || e.Policy.AllowWeb {
		return true
	}
	if e.Approve != nil {
		return e.Approve("fetch", url)
	}
	return false
}

func (e OSExecutor) webFetch(ctx context.Context, args map[string]string) Result {
	target := strings.TrimSpace(args["url"])
	if target == "" {
		return Result{Output: "fetch error: no url specified", Success: false}
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return Result{Output: "fetch error: url must start with http:// or https://", Success: false}
	}
	u, err := url.Parse(target)
	if err != nil {
		return Result{Output: "fetch error: invalid url: " + err.Error(), Success: false}
	}
	// Sandbox denies network by default: without an egress allowlist there is no
	// host the agent may reach.
	if e.Policy.Sandbox && !e.Egress.Configured() {
		return Result{Output: "blocked: sandbox mode disables network (configure an egress allowlist to permit specific hosts)", Success: false}
	}
	// Egress allowlist is a hard boundary: it is checked before the --allow-web /
	// --yolo gate, so a configured allowlist constrains even an unattended run.
	if !e.Egress.Allowed(u.Hostname()) {
		return Result{Output: "blocked by egress allowlist: " + u.Hostname() + " is not allowed", Success: false}
	}
	if !e.permitWeb(target) {
		return Result{Output: "permission denied: web fetch (" + target + ")", Success: false}
	}
	text, err := fetchURL(ctx, target, e.Egress)
	if err != nil {
		return Result{Output: "fetch error: " + err.Error(), Success: false}
	}
	return Result{Output: text, Success: true}
}

// cloneSite is the open-lovable-style "recreate a website" tool: it fetches a URL
// AND screenshots the rendered page, handing the model BOTH the original's content
// and a picture of it so it can rebuild the site as a clean, modern app — then
// drives the agentic loop by telling it to screenshot its OWN result and compare.
// Network egress, gated exactly like web_fetch (sandbox + allowlist + approval);
// the screenshot only runs after the same checks pass.
func (e OSExecutor) cloneSite(ctx context.Context, args map[string]string) Result {
	target := strings.TrimSpace(args["url"])
	if target == "" {
		return Result{Output: "clone_site error: no url specified", Success: false}
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return Result{Output: "clone_site error: url must start with http:// or https://", Success: false}
	}
	u, err := url.Parse(target)
	if err != nil {
		return Result{Output: "clone_site error: invalid url: " + err.Error(), Success: false}
	}
	if e.Policy.Sandbox && !e.Egress.Configured() {
		return Result{Output: "blocked: sandbox mode disables network (configure an egress allowlist to permit the site's host)", Success: false}
	}
	if !e.Egress.Allowed(u.Hostname()) {
		return Result{Output: "blocked by egress allowlist: " + u.Hostname() + " is not allowed", Success: false}
	}
	if !e.permitWeb(target) {
		return Result{Output: "permission denied: clone_site (" + target + ")", Success: false}
	}
	text, ferr := fetchURL(ctx, target, e.Egress)
	if ferr != nil {
		return Result{Output: "clone_site fetch error: " + ferr.Error(), Success: false}
	}
	var images []Image
	visual := "You also have a SCREENSHOT of the original above — match its layout, type, spacing, and color."
	if data, cerr := captureToImage(ctx, target, 1440, 1500); cerr == nil && len(data) > 0 {
		images = []Image{{MediaType: "image/png", Data: data}}
	} else {
		visual = "(Couldn't screenshot the original — work from the content below; install a Chromium browser for a visual reference.)"
	}
	out := "Cloned " + target + ". Recreate it as a polished, responsive web app that faithfully matches the original's structure, sections, copy, and visual style — then raise the polish. " + visual +
		"\n\nAfter you build it, run the screenshot tool on YOUR result and compare it to the original; iterate until it matches and looks world-class. Use placeholder assets where you can't fetch the originals.\n\n--- ORIGINAL PAGE CONTENT (" + u.Hostname() + ") ---\n" + text
	return Result{Output: out, Images: images, Success: true}
}

// fetchURL GETs url and returns its text. The egress allowlist is re-checked on
// EVERY redirect hop, not just the initial URL — otherwise a 302 from an
// allowlisted host could send the agent to an arbitrary or internal host (SSRF),
// defeating the boundary. A configured allowlist therefore confines the entire
// redirect chain; an unconfigured one (allow-all) leaves redirects unrestricted,
// matching the initial-request behavior.
func fetchURL(ctx context.Context, url string, egress Egress) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if !egress.Allowed(req.URL.Hostname()) {
				return fmt.Errorf("redirect to disallowed host %q blocked by egress allowlist", req.URL.Hostname())
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("user-agent", "cliche/web_fetch")
	req.Header.Set("accept", "text/html,text/plain,application/json,*/*")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes*4)) // read more, then extract+cap
	if err != nil {
		return "", err
	}
	body := string(raw)
	if strings.Contains(strings.ToLower(resp.Header.Get("content-type")), "html") {
		body = htmlToText(body)
	}
	if len(body) > maxFetchBytes {
		body = body[:maxFetchBytes] + "\n… [truncated]"
	}
	return strings.TrimSpace(body), nil
}

// htmlToText is a small, dependency-free reader-mode extractor: drop scripts and
// styles, strip tags, unescape entities, and collapse whitespace.
func htmlToText(s string) string {
	s = scriptStyleRe.ReplaceAllString(s, " ")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = wsRe.ReplaceAllString(s, " ")
	s = blankLinesRe.ReplaceAllString(s, "\n\n")
	return s
}
