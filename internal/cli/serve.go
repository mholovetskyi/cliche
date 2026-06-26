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
		f.mode = modePlan // P0: read-only until the browser approval UI lands
	}
	if !validMode(f.mode) {
		fmt.Fprintf(errOut, "serve: unknown --mode %q\n", f.mode)
		return 2
	}
	denyWrites := func(kind, target string) bool { return false } // no terminal to prompt; plan mode is read-only anyway

	a, _, cfg, cleanup, err := buildAgent(f, denyWrites, true)
	if err != nil {
		fmt.Fprintln(errOut, "serve: "+err.Error())
		return 1
	}
	defer cleanup()

	srv := web.NewServer(
		func(ctx context.Context, prompt string, emit func(web.Event)) error {
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
		},
		func() web.State { return webState(a, cfg, f.mode) },
		web.StaticFS(),
	)

	ln, err := listenLocal()
	if err != nil {
		fmt.Fprintln(errOut, "serve: "+err.Error())
		return 1
	}
	url := "http://" + ln.Addr().String()
	fmt.Fprintf(out, "  Cliche Studio is running → %s  (Ctrl-C to stop)\n", url)
	openBrowser(url)

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
