package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/cron"
	"github.com/mholovetskyi/cliche/internal/devserver"
	"github.com/mholovetskyi/cliche/internal/git"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/memory"
	"github.com/mholovetskyi/cliche/internal/persona"
	"github.com/mholovetskyi/cliche/internal/pricing"
	"github.com/mholovetskyi/cliche/internal/profile"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/secrets"
	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/tools"
	"github.com/mholovetskyi/cliche/internal/web"
	"github.com/mholovetskyi/cliche/internal/workspace"
)

// cmdServe launches Cliche Studio: a local web server (the same agent + Trust
// Kernel, streamed to a browser over SSE) for the desktop app. Phase 0 runs
// read-only (plan mode) by default — it reads, plans, and answers, but writes
// and commands await the in-browser approval cards coming next — so it is safe
// to leave open with no interactive-approval plumbing yet.
func cmdServe(args []string, out, errOut io.Writer) int {
	f, fs := parseRunFlags("serve", args)
	var listenAddr, authToken string
	fs.StringVar(&listenAddr, "listen", "", "bind address for remote/cloud access, e.g. :7878 or 0.0.0.0:7878 (default: loopback only)")
	fs.StringVar(&authToken, "token", os.Getenv("CLICHE_SERVE_TOKEN"), "require this bearer token (mandatory for a non-loopback --listen)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	// Unless the user explicitly pointed Studio at a folder (--dir), build in a
	// dedicated workspace under the home directory — never the directory Studio
	// happened to be launched from (which would litter, e.g., Cliché's own repo).
	explicitDir := false
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "dir" {
			explicitDir = true
		}
	})
	if !explicitDir {
		if ws, err := studioWorkspace(); err == nil && ws != "" {
			f.dir = ws
			fmt.Fprintf(out, "Studio workspace: %s\n", ws)
			fmt.Fprintln(out, "(projects build here, not in the launch folder — pass --dir . to work in the current folder)")
		}
	}
	if f.mode == "" {
		f.mode = modeSuggest // ask before each write/command — answered by in-browser approval cards
	}
	f.pro = true // Studio is the "build amazing things" surface — hold the product bar by default
	if !validMode(f.mode) {
		fmt.Fprintf(errOut, "serve: unknown --mode %q\n", f.mode)
		return 2
	}

	// The server exists first so the agent's approver IS the server's Approve —
	// every write/command becomes a browser "allow this?" card.
	srv := web.NewServer(nil, nil, web.StaticFS())
	// Normalize the root to an absolute, cleaned path so project switching (which
	// compares paths to confine them to the workspace) is consistent everywhere.
	if f.dir != "" {
		if abs, err := filepath.Abs(f.dir); err == nil {
			f.dir = abs
		}
	}
	previewDir := f.dir
	if previewDir == "" {
		previewDir = "."
	}
	// wsRoot is the workspace (the parent of every Project); f.dir is the ACTIVE
	// project (or the workspace itself). Captured before any project switch so the
	// Projects list always enumerates from the workspace.
	wsRoot := previewDir
	srv.SetPreviewDir(previewDir) // serve the project files for the live preview iframe

	// dev runs the built app's dev server (npm run dev) for a live, hot-reloading
	// preview — the Lovable experience. User-initiated (like Deploy); its whole
	// process tree is killed when the server shuts down.
	dev := devserver.New()
	defer dev.Stop()
	srv.SetDevServer(
		func() web.DevStatus {
			st := dev.Status(previewDir)
			return web.DevStatus{State: st.State, URL: st.URL, Dir: st.Dir, Detected: st.Detected, Script: st.Script, Logs: st.Logs}
		},
		func(action, dir string) error {
			// dir (optional) targets a specific app under the active project; it is
			// confined to the project tree, so a request can't run code elsewhere.
			root := previewDir
			if d := strings.TrimSpace(dir); d != "" {
				abs := d
				if !filepath.IsAbs(abs) {
					abs = filepath.Join(previewDir, d)
				}
				if rel, rerr := filepath.Rel(previewDir, abs); rerr != nil || strings.HasPrefix(rel, "..") {
					return fmt.Errorf("app must be inside the current project")
				}
				root = abs
			}
			switch action {
			case "start":
				return dev.Start(root)
			case "stop":
				dev.Stop()
				return nil
			case "restart":
				return dev.Restart(root)
			default:
				return fmt.Errorf("unknown dev action %q", action)
			}
		},
	)
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
		bl := a.Limits()
		_ = sess.Save(f.dir, sess.Record{
			ID: curID, Title: title, Provider: acfg.Provider, Model: a.Model(),
			Created: curCreated, Updated: time.Now(), Usage: a.Usage(), Messages: a.Transcript(), Tasks: curTasks,
			Limits: &sess.Limits{MaxUSD: bl.MaxUSD, MaxTokens: bl.MaxTokens, MaxTurns: a.GovernorLimits().MaxTurns},
		})
	}

	// applyLimits restores a session's saved Trust-Kernel caps onto the live agent,
	// falling back to the config defaults when a session has none (so switching to a
	// chat you never dialed doesn't inherit the previous chat's limits). Caller holds amu.
	applyLimits := func(cur *agent.Agent, rl *sess.Limits) {
		if cur == nil {
			return
		}
		l := sess.Limits{MaxUSD: acfg.Budget.MaxUSD, MaxTokens: acfg.Budget.MaxTokens, MaxTurns: acfg.Governor.MaxTurns}
		if rl != nil {
			l = *rl
		}
		cur.SetLimits(budget.Limits{MaxUSD: l.MaxUSD, MaxTokens: l.MaxTokens})
		g := cur.GovernorLimits()
		g.MaxTurns = l.MaxTurns
		cur.SetGovernorLimits(g)
	}

	curState := func() web.State {
		amu.Lock()
		cur, c, m := a, acfg, curMode
		amu.Unlock()
		entry, hasPreview := findPreviewEntry(previewDir)
		if cur == nil {
			return web.State{Mode: m, NeedsSetup: true, HasPreview: hasPreview, PreviewPath: entry}
		}
		st := webState(cur, c, m)
		st.HasPreview, st.PreviewPath = hasPreview, entry
		return st
	}
	srv.SetState(curState)

	// installRunner binds the run loop to a specific agent — shared by the initial
	// wire and a live provider/model switch so the two can never drift.
	installRunner := func(na *agent.Agent) {
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
					ev := web.Event{Kind: "tool_result", Text: label}
					if len(e.Images) > 0 {
						ev.Data = map[string]any{"images": e.Images}
					}
					emit(ev)
					emit(web.Event{Kind: "state", Data: curState()})
					if e.IsEdit && e.OK {
						// Is this edit inside the RUNNING dev app (so it hot-reloads itself),
						// or a separate build (e.g. a static page) the preview should switch to?
						devScoped := false
						st := dev.Status(previewDir)
						if st.State == "running" && st.Dir != "" && e.Path != "" {
							editAbs := e.Path
							if !filepath.IsAbs(editAbs) {
								editAbs = filepath.Join(previewDir, e.Path)
							}
							if rel, rerr := filepath.Rel(st.Dir, editAbs); rerr == nil && !strings.HasPrefix(rel, "..") {
								devScoped = true
							}
						}
						emit(web.Event{Kind: "reload", Data: map[string]any{"dev_scoped": devScoped}})
					}
				case "plan":
					// The agent's live progress checklist becomes the session plan,
					// pushed to the UI so the user watches it tick off in real time.
					amu.Lock()
					curTasks = curTasks[:0]
					tasks := make([]web.Task, 0, len(e.Plan))
					for i, s := range e.Plan {
						curTasks = append(curTasks, sess.Task{ID: i + 1, Title: s.Title, Done: s.Status == "done", Status: s.Status})
						tasks = append(tasks, web.Task{ID: i + 1, Title: s.Title, Done: s.Status == "done", Status: s.Status})
					}
					nextTaskID = len(curTasks) + 1
					amu.Unlock()
					emit(web.Event{Kind: "tasks", Data: tasks})
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
	}

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
				applyLimits(na, rec.Limits)
			}
		}
		if curID == "" {
			curID, curCreated = sess.NewID(time.Now()), time.Now()
		}
		amu.Unlock()
		installRunner(na)
		return nil
	}

	// reconnect rebuilds the agent on a different provider/model from the browser,
	// WITHOUT losing the current conversation. The new agent is built first; only
	// on success is it swapped in, so a bad key leaves the running agent untouched.
	reconnect := func(prov, key, model string) error {
		amu.Lock()
		busy := running
		amu.Unlock()
		if busy {
			return fmt.Errorf("a run is in progress — stop it first, then switch")
		}
		if key != "" {
			if _, err := secrets.Save(prov, key); err != nil {
				return err
			}
		}
		amu.Lock()
		prevProvider, prevModel := f.provider, f.model
		f.provider, f.model = prov, strings.TrimSpace(model)
		var keep []provider.Message
		if a != nil {
			keep = a.Transcript()
		}
		amu.Unlock()
		na, journal, ncfg, cl, err := buildAgent(f, webApprove, true)
		if err != nil {
			amu.Lock()
			f.provider, f.model = prevProvider, prevModel // restore the flags on failure
			amu.Unlock()
			return err
		}
		amu.Lock()
		old := acleanup
		a, acfg, acleanup, ajournal = na, ncfg, cl, journal
		na.RestoreTranscript(keep) // carry the conversation across the switch (budget stays honest)
		amu.Unlock()
		if old != nil {
			old()
		}
		installRunner(na)
		return nil
	}

	// switchProject re-roots the WHOLE serve at a folder under the workspace: the
	// agent's file/preview/dev scope, the chat history (each project keeps its own
	// .cliche/sessions), and the files/apps views all move there. Refused mid-run;
	// rebuilds the agent like a provider switch (via wire), then loads that
	// project's own chats. dir must be the workspace itself or a folder inside it.
	switchProject := func(dir string) error {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		if abs != wsRoot {
			rel, rerr := filepath.Rel(wsRoot, abs)
			if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("a project must be inside the workspace")
			}
		}
		if fi, serr := os.Stat(abs); serr != nil || !fi.IsDir() {
			return fmt.Errorf("no such project folder")
		}
		amu.Lock()
		if running {
			amu.Unlock()
			return fmt.Errorf("stop the current run before switching projects")
		}
		if abs == f.dir {
			amu.Unlock()
			return nil // already active
		}
		persist() // save the chat we're leaving
		oldCleanup := acleanup
		f.dir, previewDir = abs, abs
		curID, curTitle, curCreated, curTasks, nextTaskID = "", "", time.Time{}, nil, 0
		amu.Unlock()

		dev.Stop() // the new project gets its own dev server
		if oldCleanup != nil {
			oldCleanup()
		}
		srv.SetPreviewDir(abs)
		return wire() // rebuild the agent at the new root + load this project's chats
	}

	// createProject makes a new folder under the workspace and returns its path.
	createProject := func(name string) (string, error) {
		clean := sanitizeProjectName(name)
		if clean == "" {
			return "", fmt.Errorf("invalid project name")
		}
		dir := filepath.Join(wsRoot, clean)
		if _, err := os.Stat(dir); err == nil {
			return dir, nil // already exists → just open it
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		return dir, nil
	}

	srv.SetProjects(
		func() web.ProjectsView {
			amu.Lock()
			active := f.dir
			amu.Unlock()
			pv := web.ProjectsView{Workspace: wsRoot, Active: active, Projects: []web.ProjectInfo{}}
			for _, p := range workspace.Projects(wsRoot) {
				pv.Projects = append(pv.Projects, web.ProjectInfo{Name: p.Name, Path: p.Path, Apps: p.Apps, Chats: p.Chats, Active: p.Path == active})
			}
			return pv
		},
		switchProject, // open an existing project folder
		func(name string) error { // create a new one, then open it
			dir, err := createProject(name)
			if err != nil {
				return err
			}
			return switchProject(dir)
		},
	)
	srv.SetApps(func() []web.AppInfo {
		amu.Lock()
		root := f.dir
		amu.Unlock()
		out := []web.AppInfo{}
		for _, a := range workspace.Apps(root) {
			out = append(out, web.AppInfo{Name: a.Name, Rel: a.Rel, Kind: a.Kind, Script: a.Script})
		}
		return out
	})

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

	// Live provider/model switch from the browser (Settings → switch provider).
	srv.SetReconnect(reconnect)

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
			applyLimits(a, nil) // a fresh chat starts at the config defaults
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
			applyLimits(a, rec.Limits)
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
	srv.SetSessionOps(
		// rename: retitle a saved session (the active one updates live too).
		func(id, title string) error {
			amu.Lock()
			defer amu.Unlock()
			title = firstLine(strings.TrimSpace(title))
			if id == curID {
				curTitle = title
				persist() // writes the new title for the active chat
				return nil
			}
			rec, err := sess.Load(f.dir, id)
			if err != nil {
				return err
			}
			rec.Title = title
			return sess.Save(f.dir, rec)
		},
		// delete: remove a saved session; deleting the active one starts a fresh chat.
		func(id string) error {
			amu.Lock()
			defer amu.Unlock()
			if running {
				return fmt.Errorf("can't delete while a run is in progress")
			}
			if id == curID {
				if a != nil {
					a.Reset()
					applyLimits(a, nil) // back to config defaults for the fresh chat
				}
				curID, curTitle, curCreated = sess.NewID(time.Now()), "", time.Now()
				curTasks, nextTaskID = nil, 0
			}
			return sess.Delete(f.dir, id)
		},
	)
	srv.SetFiles(
		func() []web.FileNode { return fileTree(previewDir) },
		func(rel string) (string, bool) { return readProjectFile(previewDir, rel) },
	)

	// Git surface: status, commit, branch, and (gh) open-a-PR — the "ship what I
	// built" finish line, backed by internal/git.
	srv.SetGit(
		func() web.GitStatus {
			return web.GitStatus{
				Repo: git.IsRepo(f.dir), GH: ghAvailable(),
				Branch: git.CurrentBranch(f.dir), Dirty: git.HasChanges(f.dir),
				Stat: git.ShortStat(f.dir), Files: git.ChangedFiles(f.dir, 50),
			}
		},
		func(msg string) (string, error) {
			hash, stat, err := git.Commit(f.dir, msg)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(hash + "  " + stat), nil
		},
		func(name string) error { return git.CreateBranch(f.dir, name) },
		func(title, body string) (string, error) { return openPR(f.dir, title, body) },
	)
	srv.SetDeploy(func(target string) (string, error) { return deployTarget(previewDir, target) })

	// Scheduled jobs surface (the "Scheduled" panel) — manage .cliche/cron.json from
	// the web; `cliche cron run` fires them, each bounded by the Trust Kernel.
	srv.SetCron(
		func() []web.CronJob {
			jobs, _ := cron.Load(f.dir)
			out := make([]web.CronJob, 0, len(jobs))
			for _, j := range jobs {
				next := ""
				if sc, perr := cron.Parse(j.Spec); perr == nil {
					next = sc.Next(time.Now()).Format("Mon 15:04")
				}
				out = append(out, web.CronJob{ID: j.ID, Spec: j.Spec, Prompt: j.Prompt, Enabled: j.Enabled, Next: next, LastStatus: j.LastStatus})
			}
			return out
		},
		func(spec, prompt, notify string) error {
			_, err := cron.Add(f.dir, spec, prompt, "full", notify, 0)
			return err
		},
		func(id string) (bool, error) { return cron.Remove(f.dir, id) },
		func(id string, on bool) (bool, error) { return cron.SetEnabled(f.dir, id, on) },
	)

	// Hermes-style nav panels — Skills & Tools, Artifacts, Messaging — all backed
	// by existing zero-dep machinery (loadSkills, the tool roster, memory/profile,
	// pure-Go session search, the Telegram env + cron spend tracker).
	skillInfos := func(withBody bool) []web.SkillInfo {
		out := []web.SkillInfo{}
		for _, sk := range loadSkills(f.dir) {
			src := "project"
			if strings.Contains(filepath.ToSlash(sk.Rel), "/plugins/") {
				src = "plugin"
			}
			si := web.SkillInfo{Name: sk.Name, Desc: sk.Desc, Rel: sk.Rel, Source: src}
			if withBody {
				si.Body = sk.Body
			}
			out = append(out, si)
		}
		return out
	}
	srv.SetSkillsPanel(
		func() []web.SkillInfo { return skillInfos(true) },
		func() []web.ToolInfo {
			out := []web.ToolInfo{}
			for _, ts := range agent.DefaultToolSpecs() {
				out = append(out, web.ToolInfo{Name: ts.Name, Desc: ts.Description})
			}
			return out
		},
		func(url string) error { _, _, err := installSkillFromURL(url, f.dir); return err },
	)
	srv.SetArtifacts(
		func() web.ArtifactsView {
			return web.ArtifactsView{Memory: memory.Load(f.dir), Profile: profile.Load(), Skills: skillInfos(false)}
		},
		func(q string) []web.RecallHit {
			out := []web.RecallHit{}
			for _, h := range sess.Search(f.dir, q, 8) {
				out = append(out, web.RecallHit{ID: h.ID, Title: h.Title, When: h.Updated.Format("Jan 2"), Snippet: h.Snippet})
			}
			return out
		},
	)
	srv.SetMessaging(func() web.MessagingView {
		token := strings.TrimSpace(os.Getenv("CLICHE_TELEGRAM_TOKEN"))
		chat := strings.TrimSpace(os.Getenv("CLICHE_TELEGRAM_CHAT"))
		return web.MessagingView{Telegram: web.TelegramStatus{
			Configured:  token != "",
			OwnerChat:   chat,
			Authorized:  token != "" && chat != "",
			Spent24hUSD: cron.SpentLast24h(f.dir),
			MaxDailyUSD: 10,
		}}
	})
	srv.SetPersona(
		func() web.PersonaView {
			opts := []web.PersonaInfo{}
			for _, p := range persona.Presets() {
				opts = append(opts, web.PersonaInfo{Name: p.Name, Title: p.Title, Desc: p.Desc})
			}
			if persona.HasCustom() {
				opts = append(opts, web.PersonaInfo{Name: "custom", Title: "Custom", Desc: "your PERSONA.md"})
			}
			active := persona.Active()
			if active == "" {
				active = "default"
			}
			return web.PersonaView{Active: active, Options: opts}
		},
		func(name string) error { return persona.SetActive(name) },
	)

	// Live Trust-Kernel limits — the user can raise/lower the session budget cap,
	// the hard token cap, and the governor's turn limit from Settings. Applied to
	// the live agent (so it persists across session switches in this serve); refused
	// mid-run so a turn can't race the caps it's being measured against.
	srv.SetLimitsCtl(
		func() web.Limits {
			amu.Lock()
			cur := a
			amu.Unlock()
			if cur == nil {
				return web.Limits{}
			}
			bl := cur.Limits()
			return web.Limits{MaxUSD: bl.MaxUSD, MaxTokens: bl.MaxTokens, MaxTurns: cur.GovernorLimits().MaxTurns}
		},
		func(l web.Limits) error {
			if l.MaxUSD < 0 || l.MaxTokens < 0 || l.MaxTurns < 0 {
				return fmt.Errorf("limits can't be negative (use 0 for unlimited)")
			}
			amu.Lock()
			defer amu.Unlock()
			if a == nil {
				return fmt.Errorf("not connected yet")
			}
			if running {
				return fmt.Errorf("stop the current run before changing limits")
			}
			a.SetLimits(budget.Limits{MaxUSD: l.MaxUSD, MaxTokens: l.MaxTokens})
			g := a.GovernorLimits()
			g.MaxTurns = l.MaxTurns
			a.SetGovernorLimits(g)
			persist() // save the dialed caps to the current session so reopening it restores them
			return nil
		},
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
			out = append(out, web.Task{ID: t.ID, Title: t.Title, Done: t.Done, Status: t.Status})
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

	// Remote/cloud mode: binding beyond loopback exposes an agent that runs shell
	// commands, so a token is mandatory. If the operator didn't supply one, mint a
	// strong one rather than ever exposing it unauthenticated.
	networked := listenAddr != "" && !isLoopback(listenAddr)
	if networked && authToken == "" {
		authToken = genToken()
		fmt.Fprintln(out, "  ⚠ Exposing Cliché to the network — minted a required access token.")
	}
	if authToken != "" {
		srv.SetAuth(authToken)
	}

	var ln net.Listener
	var err error
	if listenAddr == "" {
		ln, err = listenLocal() // local-first default: loopback, stable port → :0 fallback
	} else {
		ln, err = net.Listen("tcp", listenAddr)
	}
	if err != nil {
		fmt.Fprintln(errOut, "serve: "+err.Error())
		return 1
	}

	url := "http://" + ln.Addr().String()
	switch {
	case authToken != "":
		fmt.Fprintf(out, "  Cliché Studio is running → %s/?token=%s\n", url, authToken)
		if networked {
			fmt.Fprintln(out, "  Network access is ON — anyone with this token and a route to this host can drive the agent.")
		}
	default:
		fmt.Fprintf(out, "  Cliche Studio is running → %s  (Ctrl-C to stop)\n", url)
	}
	// Only auto-open a browser for the local default; networked/cloud runs are headless.
	if os.Getenv("CLICHE_NO_BROWSER") == "" && listenAddr == "" {
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

// listenLocal binds the loopback interface — Studio's local-first default, never
// exposed to the network. It prefers a stable port, falling back to any free one.
func listenLocal() (net.Listener, error) {
	if ln, err := net.Listen("tcp", "127.0.0.1:7878"); err == nil {
		return ln, nil
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

// isLoopback reports whether a bind address stays on the local machine. A bare
// ":7878" (all interfaces) and "0.0.0.0" are NOT loopback — they need a token.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false // ":7878" → every interface
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// genToken mints a URL-safe random bearer token (~192 bits).
func genToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "cliche-" + base64.RawURLEncoding.EncodeToString([]byte(time.Now().String()))
	}
	return base64.RawURLEncoding.EncodeToString(b)
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
		{Name: "Website", Desc: "A polished marketing / landing site", Prompt: "Build a polished, modern marketing website — hero, features, social proof, and a contact/CTA section. Treat it like a real product launch: a coherent design system (type scale, spacing, color tokens), responsive from mobile to desktop, accessible, with tasteful motion and thoughtful copy (no lorem ipsum). It must be viewable by opening index.html at the project root — if you use a build step, output the built site to the project root."},
		{Name: "Web app", Desc: "A real, interactive product", Prompt: "Build a genuinely useful, interactive web app with a REAL modern stack — scaffold Vite + React + TypeScript + Tailwind (add shadcn/ui if it fits). First propose 2–3 concrete ideas, then build the one I pick to a production bar: a real component structure, a cohesive design system with tokens, real empty/loading/error states, input validation, and keyboard-accessible, responsive UI. Studio runs your dev server (npm run dev) for a live, hot-reloading preview — build it as a proper app, don't cram everything into one index.html. Run npm install and the dev server yourself to confirm it boots."},
		{Name: "Clone a site", Desc: "Recreate any website from its URL", Prompt: "Clone this website and make it even better — here's the URL: https://\n\nUse the clone_site tool to fetch and screenshot the original, then recreate its layout, sections, copy, and visual style as a clean, responsive app at the project root (index.html). After building, screenshot your result, compare it to the original, and iterate until it matches and looks world-class."},
		{Name: "Automate a task", Desc: "A robust little tool/script", Prompt: "Build a small but robust tool that automates a routine task on my files (ask which one if it's unclear). Make it well-structured and well-named, validate inputs, handle errors and edge cases, include a short README and a couple of tests, and run it to confirm it works."},
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

// studioWorkspace returns the default project directory for `cliche serve` when
// no --dir was given: a dedicated folder under the user's home (honoring
// $CLICHE_WORKSPACE), created on demand. This keeps Studio from building into
// whatever directory it was launched from — e.g. Cliché's own source tree.
func studioWorkspace() (string, error) {
	if ws := strings.TrimSpace(os.Getenv("CLICHE_WORKSPACE")); ws != "" {
		return ws, os.MkdirAll(ws, 0o755)
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("could not locate home directory")
	}
	ws := filepath.Join(home, "Cliche Projects")
	return ws, os.MkdirAll(ws, 0o755)
}

// sanitizeProjectName makes a safe folder name from user input: letters, digits,
// spaces, dot, dash, underscore only; trimmed and collapsed; never empty or "..",
// so a created project can't escape the workspace.
func sanitizeProjectName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.Trim(out, ".")
	if len(out) > 60 {
		out = out[:60]
	}
	return strings.TrimSpace(out)
}

// findPreviewEntry locates an app to show in the live preview: the project root's
// index.html if present, else the most-recently-built index.html one level down
// (so a project built into a subfolder still previews). Returns the relative dir
// holding it ("" = root) and whether one was found — when none, the UI shows a
// clean empty state instead of a raw directory listing.
func findPreviewEntry(dir string) (string, bool) {
	if dir == "" {
		dir = "."
	}
	if fi, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !fi.IsDir() {
		return "", true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	best := ""
	var bestMod time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if name := e.Name(); strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		p := filepath.Join(dir, e.Name(), "index.html")
		if fi, statErr := os.Stat(p); statErr == nil && !fi.IsDir() {
			if best == "" || fi.ModTime().After(bestMod) {
				best, bestMod = e.Name(), fi.ModTime()
			}
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}
