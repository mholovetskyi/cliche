package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// screenshot renders a local file (or a localhost URL) with a headless,
// already-installed Chromium-family browser and returns a PNG so the agent — and
// a vision model — can SEE the result and iterate on it. Zero Go dependencies: it
// shells out to Edge / Chrome / Chromium. Read-only and root-confined.
func (e OSExecutor) screenshot(ctx context.Context, args map[string]string) Result {
	target := firstNonEmpty(args["target"], args["file"], args["url"], "index.html")
	url, err := e.screenshotURL(target)
	if err != nil {
		return Result{Output: "screenshot denied: " + err.Error(), Success: false}
	}
	w, h := dim(args["width"], 1366), dim(args["height"], 900)
	data, err := captureToImage(ctx, url, w, h)
	if err != nil {
		return Result{Output: "screenshot " + err.Error() + " (" + target + ")", Success: false}
	}
	return Result{
		Output: fmt.Sprintf("Captured a %d×%d screenshot of %s. Study it critically — layout, spacing, alignment, type scale, color and contrast, hierarchy, and overall polish — and fix anything that isn't world-class. Re-screenshot after changes to confirm.", w, h, target),
		// The image itself is the point — the model sees it as a vision input.
		Images:  []Image{{MediaType: "image/png", Data: data}},
		Success: true,
	}
}

// captureToImage renders url — a file://, localhost, or (for clone_site) a remote
// page — with a headless, already-installed Chromium-family browser and returns
// the PNG bytes. Zero Go dependencies: it shells out to Edge/Chrome/Chromium.
// Callers are responsible for any egress/permission gating before reaching here.
func captureToImage(ctx context.Context, url string, w, h int) ([]byte, error) {
	browser := findBrowser()
	if browser == "" {
		return nil, fmt.Errorf("unavailable: no Chrome/Edge/Chromium browser was found — set CLICHE_BROWSER or install one")
	}
	out, err := os.CreateTemp("", "cliche-shot-*.png")
	if err != nil {
		return nil, fmt.Errorf("error: %v", err)
	}
	outPath := out.Name()
	_ = out.Close()
	defer os.Remove(outPath)

	// A throwaway profile forces a fresh isolated instance — without it, a browser
	// the user already has open swallows the headless flags and no image is made.
	prof, _ := os.MkdirTemp("", "cliche-prof-*")
	if prof != "" {
		defer os.RemoveAll(prof)
	}

	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, browser,
		"--headless=new", "--disable-gpu", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check", "--disable-extensions",
		"--hide-scrollbars", "--force-color-profile=srgb",
		"--user-data-dir="+prof,
		fmt.Sprintf("--window-size=%d,%d", w, h),
		"--screenshot="+outPath,
		url,
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to launch the browser: %v", err)
	}
	// Edge/Chrome typically relaunch themselves detached, so the launched process
	// returns BEFORE the capture is on disk. Poll for the PNG to appear and finish
	// writing, rather than waiting on the (already-exited) parent process.
	var data []byte
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		fi, statErr := os.Stat(outPath)
		if statErr != nil || fi.Size() == 0 {
			continue
		}
		time.Sleep(200 * time.Millisecond) // let the write settle
		if d, _ := os.ReadFile(outPath); len(d) > 0 && int64(len(d)) >= fi.Size() {
			data = d
			break
		}
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if len(data) == 0 {
		return nil, fmt.Errorf("produced no image — the page may have failed to load")
	}
	return data, nil
}

// screenshotURL turns a target into a safe, capturable URL: a project-relative
// file (confined to root) becomes a file:// URL; only localhost http(s) URLs are
// allowed through — arbitrary remote URLs are refused.
func (e OSExecutor) screenshotURL(target string) (string, error) {
	target = strings.TrimSpace(target)
	low := strings.ToLower(target)
	switch {
	case strings.HasPrefix(low, "http://localhost"), strings.HasPrefix(low, "http://127.0.0.1"),
		strings.HasPrefix(low, "https://localhost"), strings.HasPrefix(low, "https://127.0.0.1"):
		return target, nil
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
		return "", fmt.Errorf("only local files and localhost URLs may be captured (got %q)", target)
	case strings.HasPrefix(low, "file://"):
		return "", fmt.Errorf("pass a project-relative path, not a file:// URL")
	}
	p, err := e.resolve(target) // confine to the project root
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return "file:///" + filepath.ToSlash(abs), nil
}

// findBrowser locates a Chromium-family browser without any Go dependency:
// CLICHE_BROWSER first, then OS-standard install paths, then the PATH.
func findBrowser() string {
	switch strings.ToLower(os.Getenv("CLICHE_BROWSER")) {
	case "off", "none", "0":
		return "" // explicit opt-out: skip the screenshot tools entirely
	}
	if b := os.Getenv("CLICHE_BROWSER"); b != "" {
		if _, err := os.Stat(b); err == nil {
			return b
		}
		if p, err := exec.LookPath(b); err == nil {
			return p
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		pf, pf86 := os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")
		candidates = []string{
			filepath.Join(pf86, `Microsoft\Edge\Application\msedge.exe`),
			filepath.Join(pf, `Microsoft\Edge\Application\msedge.exe`),
			filepath.Join(pf, `Google\Chrome\Application\chrome.exe`),
			filepath.Join(pf86, `Google\Chrome\Application\chrome.exe`),
		}
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "chrome", "msedge"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func dim(s string, def int) int {
	n := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err == nil && n >= 200 && n <= 3840 {
		return n
	}
	return def
}
