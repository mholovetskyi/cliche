//go:build windows

// Command cliche-studio is the native desktop shell for Cliche Studio: it
// launches `cliche serve` (the local web app) and shows it in a WebView2 window
// — so the user gets a real app, not a browser tab. It lives in its OWN Go module
// (desktop/go.mod) so the WebView2 dependency never touches the zero-dependency
// CLI core; the CLI binary stays a single static, dependency-free executable.
//
// On launch the window shows a branded splash IMMEDIATELY (the Cliché logo over a
// dark aurora — no blank white gap while `cliche serve` boots), then swaps to the
// live app the moment the server reports its local URL. First-run provider setup
// is handled by the web app itself (its Setup screen appears when no provider is
// configured). If the WebView2 runtime is missing we can't draw HTML at all, so
// we fall back to a native message box pointing the user at the runtime download.
//
// The window uses the Microsoft Edge WebView2 runtime, which ships with Windows
// 10/11. When the window closes (or we exit for any reason), the embedded
// `cliche serve` is stopped.
package main

import (
	"bufio"
	_ "embed"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
	"unsafe"

	webview "github.com/jchv/go-webview2"
)

//go:embed logo.svg
var logoSVG string

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
	stopServer := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill() // stop the server whenever we leave
		}
	}
	defer stopServer()

	w := webview.NewWithOptions(webview.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview.WindowOptions{
			Title:  "Cliché Studio",
			Width:  1240,
			Height: 840,
			Center: true,
		},
	})
	if w == nil {
		// No webview means no HTML surface for an error screen — a downloaded
		// double-click app would otherwise just vanish. Show a native dialog.
		stopServer()
		messageBox("Cliché Studio",
			"Cliché Studio needs the Microsoft Edge WebView2 runtime, which isn't installed on this PC.\n\n"+
				"Install it (free, from Microsoft) at:\nhttps://developer.microsoft.com/microsoft-edge/webview2/\n\n"+
				"Then reopen Cliché Studio.")
		return
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

// messageBox shows a native Win32 modal (used only when there's no WebView2
// surface to render an HTML error into).
func messageBox(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	const mbIconError = 0x10
	proc.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), mbIconError)
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
// or external fonts, since its origin is about:blank). It shows the Cliché logo
// (the globe slowly rotating) over the same dark aurora as the web app, so the
// hand-off to the live UI feels continuous.
var splashHTML = buildSplash()

func buildSplash() string {
	return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>
  *{margin:0;box-sizing:border-box}
  html,body{height:100%;background:#0a0a0c}
  body{color:#f2f2f4;overflow:hidden;height:100vh;display:grid;place-items:center;
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;-webkit-font-smoothing:antialiased}
  .aura{position:fixed;inset:-30%;z-index:0;filter:blur(80px);opacity:.5;pointer-events:none}
  .aura::before,.aura::after{content:"";position:absolute;border-radius:50%}
  .aura::before{width:50vw;height:50vw;left:48%;top:-12%;background:radial-gradient(circle,#ff6a4d,transparent 65%);animation:d1 9s ease-in-out infinite}
  .aura::after{width:44vw;height:44vw;left:-6%;top:44%;background:radial-gradient(circle,#6f5cff,transparent 65%);animation:d2 11s ease-in-out infinite}
  @keyframes d1{0%,100%{transform:translate(0,0) scale(1)}50%{transform:translate(-8vw,10vh) scale(1.2)}}
  @keyframes d2{0%,100%{transform:translate(0,0) scale(1)}50%{transform:translate(10vw,-8vh) scale(1.25)}}
  .wrap{position:relative;z-index:1;text-align:center;animation:rise .6s cubic-bezier(.2,.7,.3,1) both}
  @keyframes rise{from{opacity:0;transform:translateY(10px)}to{opacity:1;transform:none}}
  .logo{width:132px;height:132px;margin:0 auto;color:#f2f2f4}
  .logo svg{width:132px;height:132px;display:block}
  .logo svg>g{transform-box:view-box;transform-origin:256px 256px;animation:spin 24s linear infinite}
  @keyframes spin{to{transform:rotate(360deg)}}
  h1{margin-top:18px;font-size:30px;font-weight:600;letter-spacing:-.02em}
  h1 span{color:#8a8a93;font-weight:400}
  .sub{margin-top:10px;color:#8a8a93;font-size:13.5px}
  .bar{margin:26px auto 0;width:200px;height:3px;border-radius:3px;background:rgba(255,255,255,.08);overflow:hidden}
  .bar::after{content:"";display:block;height:100%;width:42%;border-radius:3px;
    background:linear-gradient(90deg,transparent,#ff6a4d,#ff9468,transparent);animation:slide 1.3s ease-in-out infinite}
  @keyframes slide{0%{transform:translateX(-120%)}100%{transform:translateX(320%)}}
  @media (prefers-reduced-motion:reduce){.wrap,.logo svg>g,.bar::after,.aura::before,.aura::after{animation:none}}
</style></head>
<body>
  <div class="aura"></div>
  <div class="wrap">
    <div class="logo">` + logoSVG + `</div>
    <h1>Cliché <span>Studio</span></h1>
    <div class="sub">Starting your private workspace…</div>
    <div class="bar"></div>
  </div>
</body></html>`
}

// errorHTML shows when the local engine never comes up (e.g. `cliche serve`
// failed to start). Self-contained, same aesthetic.
const errorHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<style>
  *{margin:0;box-sizing:border-box}html,body{height:100%;background:#0a0a0c}
  body{color:#f2f2f4;height:100vh;display:grid;place-items:center;text-align:center;
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
