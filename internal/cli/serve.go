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
	"sync"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/pricing"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/secrets"
	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/tools"
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
	// every write/command becomes a browser "allow this?" card.
	srv := web.NewServer(nil, nil, web.StaticFS())
	previewDir := f.dir
	if previewDir == "" {
		previewDir = "."
	}
	srv.SetPreviewDir(previewDir) // serve the project files for the live preview iframe
	srv.SetTemplates(studioTemplates())
	srv.SetAudit(func() web.AuditView { return auditView(f.dir) })

	// The agent is built lazily: if a provider key is already configured we build
	// now; otherwise the server starts in SETUP mode and the browser welcome
	// screen connects a provider via /api/setup — a non-technical user never opens
	// a terminal.
	var (
		amu        sync.Mutex
		a          *agent.Agent
		acfg       config.Config
		acleanup   = func() {}
		curID      string    // current session id (persisted to .cliche/sessions)
		curTitle   string    // session title (first prompt)
		curCreated time.Time // session start
		curMode    = f.mode  // permission mode (mutable from the web, like /mode)
		running    bool      // a run is in flight → session switches are refused
		ajournal   *tools.EditJournal
		curTasks   []sess.Task      // the session plan (/plan /tasks /done)
		nextTaskID int              // monotonic id for new plan tasks
		pendingImg []provider.Image // images attached to the NEXT prompt (/image)
	)
	defer func() { acleanup() }()

	// customCmds are the user's .cliche/commands/*.md prompt shortcuts; the web
	// composer can invoke them just like the terminal (/<name> expands to its body).
	customCmds := loadCommands(f.dir)

	// webApprove is the agent's approver in the web app — it applies the live
	// permission mode exactly like the CLI's approver (plan blocks writes/commands,
	// full auto-approves, auto-edit auto-approves writes), and only suggest mode
	// falls through to a browser Allow/Not-now card. The Trust Kernel's deny rules
	// and caps still enforce underneath every mode.
	webApprove := func(action, detail string) bool {
		amu.Lock()
		m := curMode
		amu.Unlock()
		switch m {
		case modePlan:
			if action == "write" || action == "run" {
				return false
			}
		case modeFull:
			return true
		case modeAutoEdit:
			if action == "write" {
				return true
			}
		}
		return srv.Approve(action, detail)
	}

	// persist saves the live session to disk. Caller holds amu.
	persist := func() {
		if a == nil || curID == "" {
			return
		}
		title := curTitle
		if title == "" {
			title = deriveTitle(a.Transcript())
		}
		_ = sess.Save(f.dir, sess.Record{
			ID: curID, Title: title, Provider: acfg.Provider, Model: a.Model(),
			Created: curCreated, Updated: time.Now(), Usage: a.Usage(), Messages: a.Transcript(), Tasks: curTasks,
		})
	}

	curState := func() web.State {
		amu.Lock()
		cur, c, m := a, acfg, curMode
		amu.Unlock()
		if cur == nil {
			return web.State{Mode: m, NeedsSetup: true}
		}
		return webState(cur, c, m)
	}
	srv.SetState(curState)

	wire := func() error {
		na, journal, ncfg, cl, err := buildAgent(f, webApprove, true)
		if err != nil {
			return err
		}
		amu.Lock()
		a, acfg, acleanup, ajournal = na, ncfg, cl, journal
		// Resume the most recent chat (cap-honest: Restore seeds the budget from
		// the saved session), or start a fresh one.
		if id := sess.Latest(f.dir); id != "" {
			if rec, lerr := sess.Load(f.dir, id); lerr == nil {
				na.Restore(rec.Messages, rec.Usage)
				curID, curTitle, curCreated = rec.ID, rec.Title, rec.Created
				curTasks, nextTaskID = rec.Tasks, maxTaskID(rec.Tasks)
			}
		}
		if curID == "" {
			curID, curCreated = sess.NewID(time.Now()), time.Now()
		}
		amu.Unlock()
		srv.SetRunner(func(ctx context.Context, prompt string, emit func(web.Event)) error {
			// A /<name> that matches a user command expands to its body (CLI parity);
			// @file refs inline the file's contents. Both happen before the model sees
			// the prompt, exactly as in the terminal.
			if strings.HasPrefix(prompt, "/") {
				if fields := strings.Fields(prompt[1:]); len(fields) > 0 {
					if uc, ok := customCmds[fields[0]]; ok {
						prompt = uc.expand(fields[1:])
					}
				}
			}
			prompt = expandAtRefs(previewDir, prompt)
			amu.Lock()
			running = true
			if curTitle == "" {
				curTitle = titleFrom(prompt)
			}
			if len(pendingImg) > 0 {
				na.AttachImages(pendingImg)
				pendingImg = nil
			}
			amu.Unlock()
			na.SetObserver(func(e agent.Event) {
				switch e.Kind {
				case "delta", "text":
					emit(web.Event{Kind: "delta", Text: e.Text})
				case "tool_call":
					emit(web.Event{Kind: "tool_call", Text: strings.TrimSpace(e.Tool + " " + e.Detail)})
					emit(web.Event{Kind: "state", Data: curState()})
				case "tool_result":
					label := e.Tool
					if !e.OK {
						label += " — failed"
					}
					if e.Detail != "" {
						label += " · " + e.Detail
					}
					emit(web.Event{Kind: "tool_result", Text: label})
					emit(web.Event{Kind: "state", Data: curState()})
				case "halt", "budget":
					emit(web.Event{Kind: "error", Text: strings.TrimSpace(e.Text + " " + e.Detail)})
				}
			})
			_, runErr := na.Run(ctx, prompt)
			amu.Lock()
			running = false
			persist()
			amu.Unlock()
			return runErr
		})
		return nil
	}

	cfg0, _ := config.Load(f.dir)
	if f.provider != "" || firstProviderWithKey(cfg0) != "" {
		if err := wire(); err != nil {
			fmt.Fprintln(errOut, "serve: "+err.Error())
			return 1
		}
	} else {
		srv.SetSetup(func(provider, key string) error {
			if key != "" {
				if _, err := secrets.Save(provider, key); err != nil {
					return err
				}
			}
			f.provider = provider // so resolveBackend picks the chosen provider
			return wire()
		})
		fmt.Fprintln(out, "  (no provider connected yet — finish setup in the browser)")
	}

	// Multi-chat history: list / new / switch / current, all guarded by amu and
	// refused mid-run. Switching uses RestoreTranscript so the process-wide spend
	// cap can never be lowered by loading a cheaper session.
	srv.SetSessions(
		func() []web.SessionMeta {
			amu.Lock()
			cur := curID
			amu.Unlock()
			metas, _ := sess.List(f.dir)
			out := make([]web.SessionMeta, 0, len(metas))
			for _, m := range metas {
				out = append(out, web.SessionMeta{ID: m.ID, Title: firstLine(m.Title), Model: m.Model,
					Updated: m.Updated.Format(time.RFC3339), Messages: m.Messages, Active: m.ID == cur})
			}
			return out
		},
		func() string {
			amu.Lock()
			defer amu.Unlock()
			if a == nil || running {
				return curID
			}
			persist() // save the chat we're leaving
			a.Reset()
			curID, curTitle, curCreated = sess.NewID(time.Now()), "", time.Now()
			curTasks, nextTaskID = nil, 0
			return curID
		},
		func(id string) []web.Msg {
			amu.Lock()
			defer amu.Unlock()
			if a == nil || running || id == curID {
				if a != nil {
					return toMsgs(a.Transcript())
				}
				return nil
			}
			persist()
			rec, lerr := sess.Load(f.dir, id)
			if lerr != nil {
				return toMsgs(a.Transcript())
			}
			a.RestoreTranscript(rec.Messages)
			curID, curTitle, curCreated = rec.ID, rec.Title, rec.Created
			curTasks, nextTaskID = rec.Tasks, maxTaskID(rec.Tasks)
			return toMsgs(rec.Messages)
		},
		func() (string, []web.Msg) {
			amu.Lock()
			defer amu.Unlock()
			if a == nil {
				return curID, nil
			}
			return curID, toMsgs(a.Transcript())
		},
	)
	srv.SetFiles(
		func() []web.FileNode { return fileTree(previewDir) },
		func(rel string) (string, bool) { return readProjectFile(previewDir, rel) },
	)

	// CLI-parity controls: switch permission mode, list models with pricing, and
	// switch the active model — the web equivalents of /mode and /model.
	srv.SetControls(
		func(m string) bool {
			if !validMode(m) {
				return false
			}
			amu.Lock()
			curMode = m
			amu.Unlock()
			return true
		},
		func() []web.ModelInfo {
			var out []web.ModelInfo
			for _, e := range pricing.Models() {
				out = append(out, web.ModelInfo{Model: e.Model, InputPerM: e.Price.InputPerM, OutputPerM: e.Price.OutputPerM})
			}
			return out
		},
		func(m string) {
			amu.Lock()
			if a != nil {
				a.SetModel(m)
			}
			amu.Unlock()
		},
	)

	// Edit journal: the net changes the agent made, undo-last, and rewind-all —
	// the /diff, /undo, /rewind powers, now with a button.
	srv.SetEdits(
		func() []web.Change {
			amu.Lock()
			j := ajournal
			amu.Unlock()
			if j == nil {
				return nil
			}
			var out []web.Change
			for _, c := range j.Changes() {
				out = append(out, web.Change{Path: c.Path, Before: c.Before, After: c.After, WasNew: c.WasNew, Deleted: c.Deleted})
			}
			return out
		},
		func() (string, bool) {
			amu.Lock()
			j := ajournal
			amu.Unlock()
			if j == nil {
				return "", false
			}
			p, did, _ := j.Undo()
			return p, did
		},
		func() []string {
			amu.Lock()
			j := ajournal
			amu.Unlock()
			if j == nil {
				return nil
			}
			rev, _ := j.RewindAll()
			return rev
		},
	)

	// Rules/status: the trust policy in force (read-only glass box).
	srv.SetRules(func() web.Rules {
		amu.Lock()
		c, m := acfg, curMode
		amu.Unlock()
		var hooks []string
		if c.Hooks.PreToolUse != "" {
			hooks = append(hooks, "pre-tool: "+c.Hooks.PreToolUse)
		}
		if c.Hooks.Stop != "" {
			hooks = append(hooks, "stop: "+c.Hooks.Stop)
		}
		return web.Rules{
			Mode: m, ModeDesc: modeDesc(m),
			Allow: c.Permissions.Allow, Deny: c.Permissions.Deny, Egress: c.Egress.Allow, Hooks: hooks,
			MaxTurns: c.Governor.MaxTurns, MaxWallSec: c.Governor.MaxWallClockSeconds, MaxFailedEdits: c.Governor.MaxConsecutiveFailedEdits,
		}
	})

	// Plan/tasks (/plan /tasks /done), persisted with the session.
	webTasks := func() []web.Task {
		out := make([]web.Task, 0, len(curTasks))
		for _, t := range curTasks {
			out = append(out, web.Task{ID: t.ID, Title: t.Title, Done: t.Done})
		}
		return out
	}
	srv.SetTasks(
		func() []web.Task { amu.Lock(); defer amu.Unlock(); return webTasks() },
		func(title string) []web.Task {
			amu.Lock()
			defer amu.Unlock()
			if title = strings.TrimSpace(title); title != "" {
				nextTaskID++
				curTasks = append(curTasks, sess.Task{ID: nextTaskID, Title: title})
				persist()
			}
			return webTasks()
		},
		func(id int) []web.Task {
			amu.Lock()
			defer amu.Unlock()
			for i := range curTasks {
				if curTasks[i].ID == id {
					curTasks[i].Done = !curTasks[i].Done
				}
			}
			persist()
			return webTasks()
		},
		func() []web.Task {
			amu.Lock()
			defer amu.Unlock()
			curTasks, nextTaskID = nil, 0
			persist()
			return webTasks()
		},
	)

	// Image attach (/image): stash images for the next prompt.
	srv.SetImages(
		func(data []byte, mediaType string) int {
			amu.Lock()
			defer amu.Unlock()
			pendingImg = append(pendingImg, provider.Image{MediaType: mediaType, Data: data})
			return len(pendingImg)
		},
		func() {
			amu.Lock()
			pendingImg = nil
			amu.Unlock()
		},
	)

	// Custom commands (.cliche/commands) surfaced to the composer's slash palette.
	srv.SetCommands(func() []web.CommandInfo {
		var out []web.CommandInfo
		for _, c := range sortedCommands(customCmds) {
			out = append(out, web.CommandInfo{Name: c.Name, Desc: c.Desc})
		}
		return out
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

// auditView reads the project's signed, hash-chained ledger into the trust
// dashboard: the receipts, the spend, and whether the record is intact.
func auditView(dir string) web.AuditView {
	v := web.AuditView{OK: true}
	led, err := ledger.Open(config.Dir(dir))
	if err != nil {
		return v
	}
	if rep, err := led.Verify(); err == nil {
		v.OK, v.Entries, v.Verified, v.BrokenAt, v.Reason = rep.OK, rep.Entries, rep.Verified, rep.BrokenAt, rep.Reason
	}
	if sum, err := led.Summarize(); err == nil {
		v.Turns, v.USD, v.InputTokens, v.OutputTokens, v.Verdicts = sum.Turns, sum.USD, sum.InputTokens, sum.OutputTokens, sum.Verdicts
	}
	return v
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
