//go:build windows

// Command cliche-studio is the native desktop shell for Cliche Studio: it
// launches `cliche serve` (the local web app) and shows it in a WebView2 window
// — so the user gets a real app, not a browser tab. It lives in its OWN Go module
// (desktop/go.mod) so the WebView2 dependency never touches the zero-dependency
// CLI core; the CLI binary stays a single static, dependency-free executable.
//
// On launch the window shows a branded splash IMMEDIATELY (so there's no blank
// white gap while `cliche serve` boots), then swaps to the app the moment the
// server reports its local URL. First-run provider setup is handled by the web
// app itself (its Setup screen appears when no provider is configured).
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
	"time"

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

	// Show the splash instantly so the user never sees a blank window while the
	// local engine boots.
	w.SetHtml(splashHTML)

	// In the background, wait for `cliche serve` to report its URL, then swap the
	// splash for the live app (on the UI thread via Dispatch). If it never comes
	// up, show a friendly error instead of an eternal spinner.
	go func() {
		url := scanForURL(stdout, 30*time.Second)
		w.Dispatch(func() {
			if url == "" {
				w.SetHtml(errorHTML)
				return
			}
			// ?desktop=1 tells the web app to skip its in-page intro animation —
			// this splash already covered the boot.
			w.Navigate(url + "/?desktop=1")
		})
	}()

	w.Run()
}

// scanForURL reads `cliche serve`'s output until it prints the localhost URL, or
// the timeout elapses (so a hung start can't leave the splash up forever).
func scanForURL(r io.Reader, timeout time.Duration) string {
	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			if m := urlRe.FindString(sc.Text()); m != "" {
				found <- m
				return
			}
		}
		found <- "" // stdout closed without a URL (server exited early)
	}()
	select {
	case u := <-found:
		return u
	case <-time.After(timeout):
		return ""
	}
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

// splashHTML is the instant, on-brand loading screen (self-contained — no network
// or external fonts, since its origin is about:blank). It mirrors the web app's
// dark/aurora aesthetic so the hand-off to the live UI feels continuous.
const splashHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>
  *{margin:0;box-sizing:border-box}
  html,body{height:100%}
  body{background:#0a0a0c;color:#f2f2f4;overflow:hidden;height:100vh;display:grid;place-items:center;
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;-webkit-font-smoothing:antialiased}
  .aura{position:fixed;inset:-30%;z-index:0;filter:blur(80px);opacity:.5;pointer-events:none}
  .aura::before,.aura::after{content:"";position:absolute;border-radius:50%}
  .aura::before{width:50vw;height:50vw;left:48%;top:-12%;background:radial-gradient(circle,#ff6a4d,transparent 65%);animation:d1 9s ease-in-out infinite}
  .aura::after{width:44vw;height:44vw;left:-6%;top:44%;background:radial-gradient(circle,#6f5cff,transparent 65%);animation:d2 11s ease-in-out infinite}
  @keyframes d1{0%,100%{transform:translate(0,0) scale(1)}50%{transform:translate(-8vw,10vh) scale(1.2)}}
  @keyframes d2{0%,100%{transform:translate(0,0) scale(1)}50%{transform:translate(10vw,-8vh) scale(1.25)}}
  .wrap{position:relative;z-index:1;text-align:center;animation:rise .6s cubic-bezier(.2,.7,.3,1) both}
  @keyframes rise{from{opacity:0;transform:translateY(10px)}to{opacity:1;transform:none}}
  .orb{width:84px;height:84px;margin:0 auto 26px;position:relative}
  .core{position:absolute;inset:24px;border-radius:50%;background:radial-gradient(circle at 35% 30%,#ff9468,#ff6a4d);
    box-shadow:0 0 40px 6px rgba(255,106,77,.55);animation:pulse 2.4s ease-in-out infinite}
  .ring{position:absolute;inset:0;border-radius:50%;border:1.5px solid rgba(255,106,77,.3);border-top-color:#ff6a4d;animation:spin 1.1s linear infinite}
  @keyframes spin{to{transform:rotate(360deg)}}
  @keyframes pulse{0%,100%{transform:scale(1);opacity:1}50%{transform:scale(.9);opacity:.85}}
  h1{font-size:34px;font-weight:600;letter-spacing:-.02em}
  h1 span{color:#8a8a93;font-weight:400}
  .sub{margin-top:10px;color:#8a8a93;font-size:13.5px}
  .bar{margin:28px auto 0;width:200px;height:3px;border-radius:3px;background:rgba(255,255,255,.08);overflow:hidden}
  .bar::after{content:"";display:block;height:100%;width:42%;border-radius:3px;
    background:linear-gradient(90deg,transparent,#ff6a4d,#ff9468,transparent);animation:slide 1.3s ease-in-out infinite}
  @keyframes slide{0%{transform:translateX(-120%)}100%{transform:translateX(320%)}}
  @media (prefers-reduced-motion:reduce){.ring,.core,.bar::after,.aura::before,.aura::after{animation:none}}
</style></head>
<body>
  <div class="aura"></div>
  <div class="wrap">
    <div class="orb"><div class="ring"></div><div class="core"></div></div>
    <h1>Cliché <span>Studio</span></h1>
    <div class="sub">Starting your private workspace…</div>
    <div class="bar"></div>
  </div>
</body></html>`

// errorHTML shows when the local engine never comes up (e.g. `cliche serve`
// failed to start). Self-contained, same aesthetic.
const errorHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<style>
  *{margin:0;box-sizing:border-box}html,body{height:100%}
  body{background:#0a0a0c;color:#f2f2f4;height:100vh;display:grid;place-items:center;text-align:center;
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;padding:24px}
  h1{font-size:24px;font-weight:600;margin-bottom:12px}
  p{color:#a1a1ab;font-size:14px;line-height:1.6;max-width:430px}
  code{background:rgba(255,255,255,.08);padding:2px 6px;border-radius:5px;font-size:13px}
</style></head>
<body><div>
  <h1>Cliché Studio couldn’t start</h1>
  <p>The local engine didn’t come up. Make sure the <b>Microsoft Edge WebView2 runtime</b>
  is installed, then reopen Cliché Studio — or run <code>cliche serve</code> in a terminal
  to see what went wrong.</p>
</div></body></html>`
