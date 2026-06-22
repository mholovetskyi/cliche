package tools

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// web_fetch lets the agent pull current documentation/pages into context — a
// baseline coding-CLI tool. It is network egress, so it is permission-gated
// (--allow-web / approval) and bounded; an egress allowlist (roadmap) will
// further constrain which hosts it can reach. Pure stdlib (net/http).

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
	url := strings.TrimSpace(args["url"])
	if url == "" {
		return Result{Output: "fetch error: no url specified", Success: false}
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return Result{Output: "fetch error: url must start with http:// or https://", Success: false}
	}
	if !e.permitWeb(url) {
		return Result{Output: "permission denied: web fetch (" + url + ")", Success: false}
	}
	text, err := fetchURL(ctx, url)
	if err != nil {
		return Result{Output: "fetch error: " + err.Error(), Success: false}
	}
	return Result{Output: text, Success: true}
}

func fetchURL(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("user-agent", "cliche/web_fetch")
	req.Header.Set("accept", "text/html,text/plain,application/json,*/*")
	resp, err := http.DefaultClient.Do(req)
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
