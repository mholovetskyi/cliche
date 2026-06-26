package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/web"
)

// cmdServe launches Cliche Studio: a local web server (the same agent + Trust
// Kernel, streamed to a browser over SSE) for the desktop app. Phase 0 runs
// read-only (plan mode) by default — it reads, plans, and answers, but writes
// and commands await the in-browser approval cards coming next — so it is safe
// to leave open with no interactive-approval plumbing yet.
func cmdServe(args []string, out, errOut io.Writer) int {
	f, fs := parseRunFlags("serve", args)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if f.mode == "" {
		f.mode = modeSuggest // ask before each write/command — answered by in-browser approval cards
	}
	if !validMode(f.mode) {
		fmt.Fprintf(errOut, "serve: unknown --mode %q\n", f.mode)
		return 2
	}

	// The server exists first so the agent's approver IS the server's Approve —
	// every write/command becomes a browser "allow this?" card. The Trust Kernel
	// (caps, governor, deny rules, egress) still enforces underneath.
	srv := web.NewServer(nil, nil, web.StaticFS())

	a, _, cfg, cleanup, err := buildAgent(f, srv.Approve, true)
	if err != nil {
		fmt.Fprintln(errOut, "serve: "+err.Error())
		return 1
	}
	defer cleanup()

	previewDir := f.dir
	if previewDir == "" {
		previewDir = "."
	}
	srv.SetPreviewDir(previewDir) // serve the project files for the live preview iframe
	srv.SetTemplates(studioTemplates())
	srv.SetState(func() web.State { return webState(a, cfg, f.mode) })
	srv.SetRunner(func(ctx context.Context, prompt string, emit func(web.Event)) error {
		a.SetObserver(func(e agent.Event) {
			switch e.Kind {
			case "delta", "text":
				emit(web.Event{Kind: "delta", Text: e.Text})
			case "tool_call":
				emit(web.Event{Kind: "tool_call", Text: strings.TrimSpace(e.Tool + " " + e.Detail)})
				emit(web.Event{Kind: "state", Data: webState(a, cfg, f.mode)})
			case "tool_result":
				label := e.Tool
				if !e.OK {
					label += " — failed"
				}
				if e.Detail != "" {
					label += " · " + e.Detail
				}
				emit(web.Event{Kind: "tool_result", Text: label})
				emit(web.Event{Kind: "state", Data: webState(a, cfg, f.mode)})
			case "halt", "budget":
				emit(web.Event{Kind: "error", Text: strings.TrimSpace(e.Text + " " + e.Detail)})
			}
		})
		_, runErr := a.Run(ctx, prompt)
		return runErr
	})

	ln, err := listenLocal()
	if err != nil {
		fmt.Fprintln(errOut, "serve: "+err.Error())
		return 1
	}
	url := "http://" + ln.Addr().String()
	fmt.Fprintf(out, "  Cliche Studio is running → %s  (Ctrl-C to stop)\n", url)
	if os.Getenv("CLICHE_NO_BROWSER") == "" { // the desktop shell opens its own window instead
		openBrowser(url)
	}

	httpSrv := &http.Server{Handler: srv.Handler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = httpSrv.Close()
	}()
	if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(errOut, "serve: "+err.Error())
		return 1
	}
	return 0
}

// listenLocal binds the loopback interface — Studio is a local-first app, never
// exposed to the network. It prefers a stable port, falling back to any free one.
func listenLocal() (net.Listener, error) {
	if ln, err := net.Listen("tcp", "127.0.0.1:7878"); err == nil {
		return ln, nil
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

// studioTemplates are the one-click starting points shown to a non-technical
// user on the welcome screen — each kicks off a real build.
func studioTemplates() []web.Template {
	return []web.Template{
		{Name: "Website", Desc: "A personal site or landing page", Prompt: "Build a clean, modern single-page website with a hero, an about section, and a contact area. Use plain HTML, CSS, and a little JavaScript so it runs with no build step. Put it in index.html."},
		{Name: "Automate a task", Desc: "A script to do a chore for you", Prompt: "Write a small, well-commented script that automates a routine task on my files (ask me which one if it's unclear). Make it safe and easy to run."},
		{Name: "Small tool", Desc: "A handy little app", Prompt: "Build a small, self-contained tool as a single-page web app (HTML/CSS/JS, no build step) that does one useful thing well. Suggest a couple of ideas first, then build the one I pick. Put it in index.html."},
		{Name: "Explain this project", Desc: "Understand the code here", Prompt: "Give me a clear, friendly tour of what's in this project — what it does, how it's organized, and where the important pieces are. No changes, just explain."},
	}
}

func webState(a *agent.Agent, cfg config.Config, mode string) web.State {
	u := a.Usage()
	lim := a.Limits()
	ctxFrac := 0.0
	if est, _ := a.ContextStats(); cfg.Context.LimitTokens > 0 {
		ctxFrac = float64(est) / float64(cfg.Context.LimitTokens)
	}
	return web.State{
		Model:    a.Model(),
		Provider: cfg.Provider,
		Mode:     mode,
		SpentUSD: u.USD,
		CapUSD:   lim.MaxUSD,
		CtxFrac:  ctxFrac,
	}
}
