//go:build windows

// Command cliche-studio is the native desktop shell for Cliche Studio: it
// launches `cliche serve` (the local web app) and shows it in a WebView2 window
// — so the user gets a real app, not a browser tab. It lives in its OWN Go module
// (desktop/go.mod) so the WebView2 dependency never touches the zero-dependency
// CLI core; the CLI binary stays a single static, dependency-free executable.
//
// The window uses the Microsoft Edge WebView2 runtime, which ships with Windows
// 10/11. When the window closes, the embedded `cliche serve` is stopped.
package main

import (
	"bufio"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	webview "github.com/jchv/go-webview2"
)

var urlRe = regexp.MustCompile(`https?://127\.0\.0\.1:\d+`)

func main() {
	exe := clichePath()
	cmd := exec.Command(exe, "serve")
	cmd.Env = append(os.Environ(), "CLICHE_NO_BROWSER=1") // we open the window ourselves
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalln("cliche-studio:", err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatalln("cliche-studio: could not start `cliche serve`:", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill() // stop the server when the window closes
		}
	}()

	url := scanForURL(stdout)
	if url == "" {
		log.Fatalln("cliche-studio: `cliche serve` did not report a local URL")
	}

	w := webview.NewWithOptions(webview.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview.WindowOptions{
			Title:  "Cliche Studio",
			Width:  1240,
			Height: 840,
			Center: true,
		},
	})
	if w == nil {
		log.Fatalln("cliche-studio: WebView2 is unavailable — install the Microsoft Edge WebView2 runtime")
	}
	defer w.Destroy()
	w.SetSize(1240, 840, webview.HintNone)
	w.Navigate(url)
	w.Run()
}

// scanForURL reads `cliche serve`'s output until it prints the localhost URL.
func scanForURL(r io.Reader) string {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if m := urlRe.FindString(sc.Text()); m != "" {
			return m
		}
	}
	return ""
}

// clichePath finds the CLI binary: next to this shell (the installer puts both in
// the same folder), else on PATH.
func clichePath() string {
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), "cliche.exe")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	if p, err := exec.LookPath("cliche"); err == nil {
		return p
	}
	return "cliche.exe"
}
