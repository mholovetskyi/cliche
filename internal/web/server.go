package web

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// State is the trust snapshot the UI shows in its header (spend, caps, model).
type State struct {
	Model       string  `json:"model"`
	Provider    string  `json:"provider"`
	Mode        string  `json:"mode"`
	SpentUSD    float64 `json:"spent_usd"`
	CapUSD      float64 `json:"cap_usd"`
	CtxFrac     float64 `json:"ctx_frac"`
	Running     bool    `json:"running"`
	NeedsSetup  bool    `json:"needs_setup"`  // no provider connected yet → show the welcome/setup screen
	HasPreview  bool    `json:"has_preview"`  // an index.html exists to preview (else show an empty state, not a dir listing)
	PreviewPath string  `json:"preview_path"` // subdir holding that index.html ("" = project root)
}

// Runner executes one prompt, emitting events as the agent works. It is injected
// so the server is testable with a fake and the CLI wires the real agent +
// Trust Kernel. ctx cancellation must abort the run.
type Runner func(ctx context.Context, prompt string, emit func(Event)) error

// Server is the Studio backend: SSE event stream + JSON command endpoints +
// embedded static UI. One run at a time (a single agent/session), so concurrent
// prompts are rejected rather than racing the transcript.
type Server struct {
	hub    *Hub
	run    Runner
	state  func() State
	static fs.FS  // embedded SPA assets (may be nil during early bring-up)
	token  string // when set, every request must present this bearer token (remote/cloud mode)

	mu      sync.Mutex
	running bool

	apMu      sync.Mutex
	apSeq     int
	approvals map[string]chan bool // pending approval id → answer channel

	templates  []Template
	previewDir string // project root, served read-only at /preview/ for the live preview iframe
	audit      func() AuditView
	setup      func(provider, key string) error        // first-run: connect a provider (no terminal needed)
	reconnect  func(provider, key, model string) error // live provider/model switch (Settings)

	sessions      func() []SessionMeta
	sessionNew    func() string
	sessionPick   func(id string) []Msg
	sessionCur    func() (string, []Msg)
	sessionRename func(id, title string) error
	sessionDelete func(id string) error
	files         func() []FileNode
	fileRead      func(rel string) (string, bool)

	gitStatus func() GitStatus
	gitCommit func(msg string) (string, error)
	gitBranch func(name string) error
	gitPR     func(title, body string) (string, error)
	deploy    func() (string, error)

	cronList   func() []CronJob
	cronAdd    func(spec, prompt string) error
	cronRemove func(id string) (bool, error)
	cronToggle func(id string, on bool) (bool, error)

	cancel     context.CancelFunc // cancels the in-flight run (Stop)
	setModeFn  func(mode string) bool
	modelsFn   func() []ModelInfo
	setModelFn func(model string)

	changesFn func() []Change
	undoFn    func() (string, bool)
	rewindFn  func() []string
	rulesFn   func() Rules

	tasksFn    func() []Task
	taskAdd    func(title string) []Task
	taskDone   func(id int) []Task
	taskClear  func() []Task
	imageAdd   func(data []byte, mediaType string) int
	imageClear func()
	commandsFn func() []CommandInfo

	// Hermes-style nav panels: Skills & Tools, Artifacts, Messaging.
	skillsFn     func() []SkillInfo
	toolsFn      func() []ToolInfo
	skillInstall func(url string) error
	artifactsFn  func() ArtifactsView
	recallFn     func(q string) []RecallHit
	messagingFn  func() MessagingView
	personaGet   func() PersonaView
	personaSet   func(name string) error
	limitsGet    func() Limits
	limitsSet    func(Limits) error
	devStatus    func() DevStatus
	devControl   func(action, dir string) error
	projectsGet  func() ProjectsView
	projectOpen  func(path string) error
	projectNew   func(name string) error
	appsGet      func() []AppInfo
}

// ProjectInfo is a folder under the workspace that holds its own chats + apps.
type ProjectInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Apps   int    `json:"apps"`
	Chats  int    `json:"chats"`
	Active bool   `json:"active"`
}

// ProjectsView is the project switcher's data: the workspace, the active folder,
// and the projects in it.
type ProjectsView struct {
	Workspace string        `json:"workspace"`
	Active    string        `json:"active"`
	Projects  []ProjectInfo `json:"projects"`
}

// AppInfo is a buildable folder (static index.html or a dev-server app).
type AppInfo struct {
	Name   string `json:"name"`
	Rel    string `json:"rel"`
	Kind   string `json:"kind"`
	Script string `json:"script"`
}

// SetProjects wires the project switcher: list, open (re-root the serve), create.
func (s *Server) SetProjects(get func() ProjectsView, open func(path string) error, create func(name string) error) {
	s.projectsGet, s.projectOpen, s.projectNew = get, open, create
}

// SetApps wires the apps list (buildable folders under the active project).
func (s *Server) SetApps(get func() []AppInfo) { s.appsGet = get }

// DevStatus mirrors the live dev server behind the preview (the Lovable-style
// "run the real app" experience): its state, URL, and tail of logs.
type DevStatus struct {
	State    string   `json:"state"`
	URL      string   `json:"url"`
	Dir      string   `json:"dir"`
	Detected bool     `json:"detected"`
	Script   string   `json:"script"`
	Logs     []string `json:"logs"`
}

// SetDevServer wires the dev-server controls: a status reader + a start/stop/
// restart control (with an optional app dir to target).
func (s *Server) SetDevServer(status func() DevStatus, control func(action, dir string) error) {
	s.devStatus, s.devControl = status, control
}

// Limits is the live, user-adjustable slice of the Trust Kernel: the session
// budget cap, the hard token cap, and the governor's turn limit.
type Limits struct {
	MaxUSD    float64 `json:"max_usd"`
	MaxTokens int     `json:"max_tokens"`
	MaxTurns  int     `json:"max_turns"`
}

// PersonaInfo is one selectable personality (built-in preset or custom).
type PersonaInfo struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

// PersonaView is the personality picker state: the options + the active one.
type PersonaView struct {
	Active  string        `json:"active"`
	Options []PersonaInfo `json:"options"`
}

// SkillInfo is one loaded skill, surfaced to the Skills & Tools panel.
type SkillInfo struct {
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Rel    string `json:"rel"`
	Source string `json:"source"` // "project" | "plugin"
	Body   string `json:"body,omitempty"`
}

// ToolInfo is one built-in agent tool in the read-only roster.
type ToolInfo struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// ArtifactsView is the durable state the agent accrues: project memory, the user
// profile, and the saved skills — the Artifacts panel renders these.
type ArtifactsView struct {
	Memory  string      `json:"memory"`
	Profile string      `json:"profile"`
	Skills  []SkillInfo `json:"skills"`
}

// RecallHit is one past-session match for the Artifacts recall search.
type RecallHit struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	When    string `json:"when"`
	Snippet string `json:"snippet"`
}

// MessagingView reports the remote-drive channels (the Messaging panel).
type MessagingView struct {
	Telegram TelegramStatus `json:"telegram"`
}

// TelegramStatus mirrors the owner-locked, budget-bounded Telegram bridge.
type TelegramStatus struct {
	Configured  bool    `json:"configured"`
	OwnerChat   string  `json:"owner_chat"`
	Authorized  bool    `json:"authorized"`
	Spent24hUSD float64 `json:"spent_24h_usd"`
	MaxDailyUSD float64 `json:"max_daily_usd"`
}

// SetSkillsPanel wires the Skills & Tools panel: the skill list, the tool roster,
// and install-from-URL (which reuses the slug-sanitized installer).
func (s *Server) SetSkillsPanel(list func() []SkillInfo, tools func() []ToolInfo, install func(url string) error) {
	s.skillsFn, s.toolsFn, s.skillInstall = list, tools, install
}

// SetArtifacts wires the Artifacts panel: the durable-state view + recall search.
func (s *Server) SetArtifacts(get func() ArtifactsView, recall func(q string) []RecallHit) {
	s.artifactsFn, s.recallFn = get, recall
}

// SetMessaging wires the Messaging panel's channel-status view.
func (s *Server) SetMessaging(get func() MessagingView) { s.messagingFn = get }

// SetPersona wires the personality picker: read the options + active, set active.
func (s *Server) SetPersona(get func() PersonaView, set func(name string) error) {
	s.personaGet, s.personaSet = get, set
}

// SetLimitsCtl wires the live Trust-Kernel limit controls (budget/token/turn caps).
func (s *Server) SetLimitsCtl(get func() Limits, set func(Limits) error) {
	s.limitsGet, s.limitsSet = get, set
}

// Task is one item on the session plan (/plan /tasks /done, or the agent's
// live update_plan checklist).
type Task struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Done   bool   `json:"done"`
	Status string `json:"status,omitempty"` // "pending" | "doing" | "done" (agent plan)
}

// CommandInfo is a user-defined .cliche/commands shortcut, surfaced to the palette.
type CommandInfo struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// SetTasks wires the session plan: list, add, toggle-done, clear.
func (s *Server) SetTasks(list func() []Task, add func(string) []Task, done func(int) []Task, clear func() []Task) {
	s.tasksFn, s.taskAdd, s.taskDone, s.taskClear = list, add, done, clear
}

// SetImages wires image attachment for the next prompt (add returns the new count).
func (s *Server) SetImages(add func(data []byte, mediaType string) int, clear func()) {
	s.imageAdd, s.imageClear = add, clear
}

// SetCommands wires the user's custom slash commands for the palette.
func (s *Server) SetCommands(f func() []CommandInfo) { s.commandsFn = f }

// ModelInfo is one selectable model + its price (for the model picker).
type ModelInfo struct {
	Model      string  `json:"model"`
	InputPerM  float64 `json:"input_per_m"`
	OutputPerM float64 `json:"output_per_m"`
}

// Change is one file the session edited (net before→after), for the Changes tab.
type Change struct {
	Path    string `json:"path"`
	Before  string `json:"before"`
	After   string `json:"after"`
	WasNew  bool   `json:"was_new"`
	Deleted bool   `json:"deleted"`
}

// Rules is the trust policy in force this session (the /status + /rules view).
type Rules struct {
	Mode           string   `json:"mode"`
	ModeDesc       string   `json:"mode_desc"`
	Allow          []string `json:"allow"`
	Deny           []string `json:"deny"`
	Egress         []string `json:"egress"`
	Hooks          []string `json:"hooks"`
	MaxTurns       int      `json:"max_turns"`
	MaxWallSec     int      `json:"max_wall_sec"`
	MaxFailedEdits int      `json:"max_failed_edits"`
}

// SetEdits wires the session edit journal: the net changes, undo-last, rewind-all.
func (s *Server) SetEdits(changes func() []Change, undo func() (string, bool), rewind func() []string) {
	s.changesFn, s.undoFn, s.rewindFn = changes, undo, rewind
}

// SetRules wires the read-only trust-policy view (/status + /rules).
func (s *Server) SetRules(f func() Rules) { s.rulesFn = f }

// SetControls wires the CLI-parity controls: change permission mode, list
// models, switch model — the same levers /mode and /model pull in the terminal.
func (s *Server) SetControls(setMode func(string) bool, models func() []ModelInfo, setModel func(string)) {
	s.setModeFn, s.modelsFn, s.setModelFn = setMode, models, setModel
}

// SessionMeta is a saved chat shown in the sidebar.
type SessionMeta struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Model    string `json:"model"`
	Updated  string `json:"updated"`
	Messages int    `json:"messages"`
	Active   bool   `json:"active"`
}

// Msg is one transcript message rendered in the conversation feed.
type Msg struct {
	Role string `json:"role"` // user | assistant | tool
	Text string `json:"text"`
}

// FileNode is one entry in the project file tree (the workspace).
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"` // forward-slashed, relative to the project root
	Dir      bool       `json:"dir"`
	Children []FileNode `json:"children,omitempty"`
}

// SetSessions wires multi-chat history: list saved sessions, start a new one
// (returns its id), switch to one (returns its transcript), read the current.
func (s *Server) SetSessions(list func() []SessionMeta, neww func() string, pick func(string) []Msg, cur func() (string, []Msg)) {
	s.sessions, s.sessionNew, s.sessionPick, s.sessionCur = list, neww, pick, cur
}

// SetSessionOps wires rename + delete for saved sessions (the sidebar actions).
func (s *Server) SetSessionOps(rename func(id, title string) error, del func(id string) error) {
	s.sessionRename, s.sessionDelete = rename, del
}

// SetFiles wires the workspace file tree + read-only file viewer.
func (s *Server) SetFiles(tree func() []FileNode, read func(string) (string, bool)) {
	s.files, s.fileRead = tree, read
}

// GitStatus is the working tree at a glance (the Git workspace tab).
type GitStatus struct {
	Repo   bool     `json:"repo"`
	GH     bool     `json:"gh"` // the gh CLI is available → "Open PR" works
	Branch string   `json:"branch"`
	Dirty  bool     `json:"dirty"`
	Stat   string   `json:"stat"` // "N files changed, +X -Y"
	Files  []string `json:"files"`
}

// SetGit wires the git surface: status, commit, new branch, open PR.
func (s *Server) SetGit(status func() GitStatus, commit func(string) (string, error), branch func(string) error, pr func(string, string) (string, error)) {
	s.gitStatus, s.gitCommit, s.gitBranch, s.gitPR = status, commit, branch, pr
}

// SetDeploy wires the "publish to a live URL" button (GitHub Pages via gh).
func (s *Server) SetDeploy(deploy func() (string, error)) { s.deploy = deploy }

// CronJob is a scheduled prompt as shown in the Studio "Scheduled" panel.
type CronJob struct {
	ID         string `json:"id"`
	Spec       string `json:"spec"`
	Prompt     string `json:"prompt"`
	Enabled    bool   `json:"enabled"`
	Next       string `json:"next"`        // human next-fire time
	LastStatus string `json:"last_status"` // "" if never run
}

// SetCron wires the scheduled-jobs surface (list + add/remove/enable-disable).
func (s *Server) SetCron(list func() []CronJob, add func(spec, prompt string) error, remove func(id string) (bool, error), toggle func(id string, on bool) (bool, error)) {
	s.cronList, s.cronAdd, s.cronRemove, s.cronToggle = list, add, remove, toggle
}

func (s *Server) handleCron(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		jobs := []CronJob{}
		if s.cronList != nil {
			if got := s.cronList(); got != nil {
				jobs = got
			}
		}
		_ = json.NewEncoder(w).Encode(jobs)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Action  string `json:"action"` // add | remove | toggle
		Spec    string `json:"spec"`
		Prompt  string `json:"prompt"`
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var err error
	switch body.Action {
	case "add":
		if s.cronAdd != nil {
			err = s.cronAdd(body.Spec, body.Prompt)
		}
	case "remove":
		if s.cronRemove != nil {
			_, err = s.cronRemove(body.ID)
		}
	case "toggle":
		if s.cronToggle != nil {
			_, err = s.cronToggle(body.ID, body.Enabled)
		}
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// handleSkills GET → {skills, tools} for the Skills & Tools panel.
func (s *Server) handleSkills(w http.ResponseWriter, _ *http.Request) {
	skills := []SkillInfo{}
	tools := []ToolInfo{}
	if s.skillsFn != nil {
		if got := s.skillsFn(); got != nil {
			skills = got
		}
	}
	if s.toolsFn != nil {
		if got := s.toolsFn(); got != nil {
			tools = got
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"skills": skills, "tools": tools})
}

// handleSkillInstall POST {url} → download + validate + install a SKILL.md. The
// installer slug-sanitizes the name so a remote URL can't escape .cliche/skills/.
func (s *Server) handleSkillInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if s.skillInstall == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.skillInstall(strings.TrimSpace(body.URL)); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// handleArtifacts GET → the durable agent state (memory, profile, skills).
func (s *Server) handleArtifacts(w http.ResponseWriter, _ *http.Request) {
	var v ArtifactsView
	if s.artifactsFn != nil {
		v = s.artifactsFn()
	}
	if v.Skills == nil {
		v.Skills = []SkillInfo{}
	}
	_ = json.NewEncoder(w).Encode(v)
}

// handleRecall GET ?q= → past-session matches (pure-Go cross-session search).
func (s *Server) handleRecall(w http.ResponseWriter, r *http.Request) {
	hits := []RecallHit{}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if s.recallFn != nil && q != "" {
		if got := s.recallFn(q); got != nil {
			hits = got
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"hits": hits})
}

// handleLimits GET → current Trust-Kernel caps; POST {max_usd,max_tokens,max_turns}
// → adjust them for the session (the wire refuses changes during a live run).
func (s *Server) handleLimits(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if s.limitsSet == nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var l Limits
		if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.limitsSet(l); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	var l Limits
	if s.limitsGet != nil {
		l = s.limitsGet()
	}
	_ = json.NewEncoder(w).Encode(l)
}

// handleDev GET → dev-server status; POST {action:start|stop|restart} → control it.
func (s *Server) handleDev(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if s.devControl == nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Action string `json:"action"`
			Dir    string `json:"dir"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := s.devControl(strings.TrimSpace(body.Action), strings.TrimSpace(body.Dir)); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	var st DevStatus
	if s.devStatus != nil {
		st = s.devStatus()
	}
	if st.Logs == nil {
		st.Logs = []string{}
	}
	_ = json.NewEncoder(w).Encode(st)
}

// handleProjects GET → the project switcher view; POST {action:open,path} |
// {action:create,name} re-roots the serve at that project (its own chats + apps).
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body struct {
			Action string `json:"action"`
			Path   string `json:"path"`
			Name   string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var err error
		switch body.Action {
		case "open":
			if s.projectOpen != nil {
				err = s.projectOpen(strings.TrimSpace(body.Path))
			}
		case "create":
			if s.projectNew != nil {
				err = s.projectNew(strings.TrimSpace(body.Name))
			}
		default:
			err = errBadAction
		}
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	var v ProjectsView
	if s.projectsGet != nil {
		v = s.projectsGet()
	}
	if v.Projects == nil {
		v.Projects = []ProjectInfo{}
	}
	_ = json.NewEncoder(w).Encode(v)
}

// handleApps GET → the buildable apps under the active project.
func (s *Server) handleApps(w http.ResponseWriter, _ *http.Request) {
	apps := []AppInfo{}
	if s.appsGet != nil {
		if got := s.appsGet(); got != nil {
			apps = got
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"apps": apps})
}

var errBadAction = fmt.Errorf("unknown action")

// handleMessaging GET → the remote-drive channel status (Telegram).
func (s *Server) handleMessaging(w http.ResponseWriter, _ *http.Request) {
	var v MessagingView
	if s.messagingFn != nil {
		v = s.messagingFn()
	}
	_ = json.NewEncoder(w).Encode(v)
}

// handlePersona GET → the personality options + active; POST {name} → set active.
func (s *Server) handlePersona(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if s.personaSet == nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := s.personaSet(strings.TrimSpace(body.Name)); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	var v PersonaView
	if s.personaGet != nil {
		v = s.personaGet()
	}
	if v.Options == nil {
		v.Options = []PersonaInfo{}
	}
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.deploy == nil {
		http.Error(w, "deploy not available", http.StatusNotImplemented)
		return
	}
	url, err := s.deploy()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"url": url})
}

func (s *Server) handleGit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var st GitStatus
	if s.gitStatus != nil {
		st = s.gitStatus()
	}
	_ = json.NewEncoder(w).Encode(st)
}

func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Msg string `json:"msg"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || strings.TrimSpace(body.Msg) == "" {
		http.Error(w, "a commit message is required", http.StatusBadRequest)
		return
	}
	if s.gitCommit == nil {
		http.Error(w, "git unavailable", http.StatusServiceUnavailable)
		return
	}
	res, err := s.gitCommit(body.Msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"result": res})
}

func (s *Server) handleGitBranch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		http.Error(w, "a branch name is required", http.StatusBadRequest)
		return
	}
	if s.gitBranch == nil {
		http.Error(w, "git unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.gitBranch(body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGitPR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	if s.gitPR == nil {
		http.Error(w, "PR unavailable", http.StatusServiceUnavailable)
		return
	}
	url, err := s.gitPR(body.Title, body.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
}

// Template is a one-click starting point for a non-technical user — a friendly
// name + a starter prompt that kicks off a build.
type Template struct {
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Prompt string `json:"prompt"`
}

// SetTemplates / SetPreviewDir configure the build-anything surface.
func (s *Server) SetTemplates(t []Template) { s.templates = t }
func (s *Server) SetPreviewDir(dir string)  { s.previewDir = dir }

// AuditView is the trust dashboard: the verifiable audit ledger summarized for a
// human — what the agent did, what it cost, and whether the record is intact.
type AuditView struct {
	OK           bool           `json:"ok"`       // chain intact (no tampering detected)
	Entries      int            `json:"entries"`  // receipts recorded
	Verified     int            `json:"verified"` // hash-chain-verified receipts
	BrokenAt     int            `json:"broken_at,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Turns        int            `json:"turns"`
	USD          float64        `json:"usd"`
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	Verdicts     map[string]int `json:"verdicts,omitempty"`
}

// SetAudit binds the function that reads the (signed, hash-chained) ledger.
func (s *Server) SetAudit(f func() AuditView) { s.audit = f }

// SetSetup binds the first-run connect callback (save a key / pick a provider and
// build the agent), so a non-technical user never has to touch a terminal.
func (s *Server) SetSetup(f func(provider, key string) error) { s.setup = f }

// SetReconnect binds the live provider/model switch (Settings) — rebuilds the
// agent on a new provider/model without dropping the conversation.
func (s *Server) SetReconnect(f func(provider, key, model string) error) { s.reconnect = f }

func (s *Server) handleProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.reconnect == nil {
		http.Error(w, "not available", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Provider string `json:"provider"`
		Key      string `json:"key"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.Provider == "" {
		http.Error(w, "expected {\"provider\":\"…\",\"key\":\"…\",\"model\":\"…\"}", http.StatusBadRequest)
		return
	}
	if err := s.reconnect(body.Provider, body.Key, body.Model); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.hub.Emit(Event{Kind: "state", Data: s.snapshot(s.isRunning())})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.setup == nil {
		http.Error(w, "already set up", http.StatusConflict)
		return
	}
	var body struct {
		Provider string `json:"provider"`
		Key      string `json:"key"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.Provider == "" {
		http.Error(w, "expected {\"provider\":\"…\",\"key\":\"…\"}", http.StatusBadRequest)
		return
	}
	if err := s.setup(body.Provider, body.Key); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.setup = nil // connected; the welcome screen gives way to the app
	w.WriteHeader(http.StatusNoContent)
}

func NewServer(run Runner, state func() State, static fs.FS) *Server {
	return &Server{hub: NewHub(), run: run, state: state, static: static, approvals: map[string]chan bool{}}
}

// SetRunner / SetState bind the agent callbacks after construction, so the CLI
// can build the agent with the server's own Approve as its approver (the server
// must exist first) and then wire the run loop.
func (s *Server) SetRunner(r Runner)      { s.run = r }
func (s *Server) SetState(f func() State) { s.state = f }

// approvalReq is the payload the UI renders as an "allow this?" card.
type approvalReq struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`   // e.g. "write", "run", "mcp"
	Target string `json:"target"` // the file / command / tool
}

// Approve is the agent's permission gate in the web app: it emits an approval
// request to the browser and blocks the run until the user answers (or a
// fail-safe timeout denies). This is the in-browser equivalent of the CLI's
// y/N card; the Trust Kernel's deny rules / caps still apply underneath.
func (s *Server) Approve(kind, target string) bool {
	s.apMu.Lock()
	s.apSeq++
	id := fmt.Sprintf("ap%d", s.apSeq)
	ch := make(chan bool, 1)
	s.approvals[id] = ch
	s.apMu.Unlock()
	defer func() {
		s.apMu.Lock()
		delete(s.approvals, id)
		s.apMu.Unlock()
	}()

	s.hub.Emit(Event{Kind: "approval", Data: approvalReq{ID: id, Kind: kind, Target: target}})
	select {
	case ok := <-ch:
		return ok
	case <-time.After(10 * time.Minute):
		return false // no answer → fail safe (deny), never auto-allow
	}
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID    string `json:"id"`
		Allow bool   `json:"allow"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "expected {\"id\":\"…\",\"allow\":bool}", http.StatusBadRequest)
		return
	}
	s.apMu.Lock()
	ch := s.approvals[body.ID]
	s.apMu.Unlock()
	if ch != nil {
		select {
		case ch <- body.Allow:
		default:
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Handler returns the HTTP routes. Mounted by the CLI on a localhost listener.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/prompt", s.handlePrompt)
	mux.HandleFunc("/api/approve", s.handleApprove)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/templates", s.handleTemplates)
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/setup", s.handleSetup)
	mux.HandleFunc("/api/provider", s.handleProvider)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/new", s.handleSessionNew)
	mux.HandleFunc("/api/sessions/select", s.handleSessionSelect)
	mux.HandleFunc("/api/sessions/rename", s.handleSessionRename)
	mux.HandleFunc("/api/sessions/delete", s.handleSessionDelete)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/files", s.handleFiles)
	mux.HandleFunc("/api/file", s.handleFile)
	mux.HandleFunc("/api/git", s.handleGit)
	mux.HandleFunc("/api/git/commit", s.handleGitCommit)
	mux.HandleFunc("/api/git/branch", s.handleGitBranch)
	mux.HandleFunc("/api/git/pr", s.handleGitPR)
	mux.HandleFunc("/api/deploy", s.handleDeploy)
	mux.HandleFunc("/api/cron", s.handleCron)
	mux.HandleFunc("/api/skills", s.handleSkills)
	mux.HandleFunc("/api/skills/install", s.handleSkillInstall)
	mux.HandleFunc("/api/artifacts", s.handleArtifacts)
	mux.HandleFunc("/api/recall", s.handleRecall)
	mux.HandleFunc("/api/messaging", s.handleMessaging)
	mux.HandleFunc("/api/persona", s.handlePersona)
	mux.HandleFunc("/api/limits", s.handleLimits)
	mux.HandleFunc("/api/dev", s.handleDev)
	mux.HandleFunc("/api/projects", s.handleProjects)
	mux.HandleFunc("/api/apps", s.handleApps)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/models", s.handleModels)
	mux.HandleFunc("/api/model", s.handleModel)
	mux.HandleFunc("/api/changes", s.handleChanges)
	mux.HandleFunc("/api/undo", s.handleUndo)
	mux.HandleFunc("/api/rewind", s.handleRewind)
	mux.HandleFunc("/api/rules", s.handleRules)
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/tasks/done", s.handleTaskDone)
	mux.HandleFunc("/api/tasks/clear", s.handleTaskClear)
	mux.HandleFunc("/api/image", s.handleImage)
	mux.HandleFunc("/api/image/clear", s.handleImageClear)
	mux.HandleFunc("/api/commands", s.handleCommands)
	// The live preview serves the project files (what the agent is building) so
	// an iframe can show the result. Localhost-only, the user's own files;
	// http.Dir blocks path traversal outside the root.
	if s.previewDir != "" {
		mux.Handle("/preview/", http.StripPrefix("/preview/", http.FileServer(http.Dir(s.previewDir))))
	}
	if s.static != nil {
		mux.Handle("/", http.FileServer(http.FS(s.static)))
	}
	if s.token != "" {
		return s.withAuth(mux) // remote/cloud mode: gate everything behind the token
	}
	return mux
}

func (s *Server) handleSessions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	list := []SessionMeta{}
	if s.sessions != nil {
		if got := s.sessions(); got != nil {
			list = got
		}
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleSessionNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	id := ""
	if s.sessionNew != nil {
		id = s.sessionNew()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "messages": []Msg{}})
}

func (s *Server) handleSessionSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "expected {\"id\":\"…\"}", http.StatusBadRequest)
		return
	}
	msgs := []Msg{}
	if s.sessionPick != nil {
		if got := s.sessionPick(body.ID); got != nil {
			msgs = got
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": body.ID, "messages": msgs})
}

func (s *Server) handleSessionRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "expected {\"id\":\"…\",\"title\":\"…\"}", http.StatusBadRequest)
		return
	}
	if s.sessionRename != nil {
		if err := s.sessionRename(body.ID, body.Title); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "expected {\"id\":\"…\"}", http.StatusBadRequest)
		return
	}
	if s.sessionDelete != nil {
		if err := s.sessionDelete(body.ID); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleSession(w http.ResponseWriter, _ *http.Request) {
	id, msgs := "", []Msg{}
	if s.sessionCur != nil {
		i, m := s.sessionCur()
		id = i
		if m != nil {
			msgs = m
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "messages": msgs})
}

func (s *Server) handleFiles(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tree := []FileNode{}
	if s.files != nil {
		if got := s.files(); got != nil {
			tree = got
		}
	}
	_ = json.NewEncoder(w).Encode(tree)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if s.fileRead == nil {
		http.Error(w, "no files", http.StatusNotFound)
		return
	}
	txt, ok := s.fileRead(r.URL.Query().Get("path"))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, txt)
}

func (s *Server) handleAudit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var v AuditView
	if s.audit != nil {
		v = s.audit()
	}
	_ = json.NewEncoder(w).Encode(v)
}

// exportSkip are project subdirs left out of the download — internal state and
// dependency/build noise, never the user's actual work.
var exportSkip = map[string]bool{".cliche": true, ".git": true, "node_modules": true, "dist": true}

// IsSensitiveFile reports whether a filename looks like a secret that must never
// be shown in the file viewer or bundled into an export — API keys, env files,
// credentials, private keys. The workspace is the user's own machine, but a
// secret should not be one click from being displayed or shared.
func IsSensitiveFile(name string) bool {
	n := strings.ToLower(name)
	switch n {
	case "api.txt", ".env", ".npmrc", ".netrc", "credentials", "secrets.json", "id_rsa", "id_ed25519", ".pgpass":
		return true
	}
	if strings.HasPrefix(n, ".env") {
		return true
	}
	for _, ext := range []string{".pem", ".key", ".p12", ".pfx"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}

// handleExport streams the project as a .zip so the user can take what Cliche
// built and run/keep it anywhere. Confined to the project root.
func (s *Server) handleExport(w http.ResponseWriter, _ *http.Request) {
	if s.previewDir == "" {
		http.Error(w, "no project to export", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="cliche-project.zip"`)
	zw := zip.NewWriter(w)
	defer zw.Close()

	root := s.previewDir
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if top := strings.SplitN(rel, "/", 2)[0]; exportSkip[top] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if IsSensitiveFile(filepath.Base(rel)) {
			return nil // never bundle a secret into the downloadable project
		}
		fw, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		_, _ = io.Copy(fw, f)
		return nil
	})
}

func (s *Server) handleTemplates(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	t := s.templates
	if t == nil {
		t = []Template{}
	}
	_ = json.NewEncoder(w).Encode(t)
}

// handleEvents is the SSE stream: every agent event for every connected client.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsub := s.hub.Subscribe()
	defer unsub()
	writeSSE(w, Event{Kind: "begin"})
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, e)
			flusher.Flush()
		}
	}
}

func writeSSE(w io.Writer, e Event) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
}

// handlePrompt accepts {"prompt": "..."} and runs the agent in the background,
// streaming events over /api/events. 409 if a run is already in flight.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || body.Prompt == "" {
		http.Error(w, "expected {\"prompt\": \"…\"}", http.StatusBadRequest)
		return
	}
	if s.run == nil {
		http.Error(w, "no runner configured", http.StatusServiceUnavailable)
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		http.Error(w, "a run is already in progress", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.running = true
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		s.hub.Emit(Event{Kind: "state", Data: s.snapshot(true)})
		err := s.run(ctx, body.Prompt, s.hub.Emit)
		if err != nil {
			s.hub.Emit(Event{Kind: "error", Text: err.Error()})
		}
		s.mu.Lock()
		s.running = false
		s.cancel = nil
		s.mu.Unlock()
		s.hub.Emit(Event{Kind: "end"})
		s.hub.Emit(Event{Kind: "state", Data: s.snapshot(false)})
	}()
	w.WriteHeader(http.StatusAccepted)
}

// handleStop aborts the in-flight run (the web equivalent of Ctrl-C / Esc).
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	c := s.cancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&body); err != nil || body.Mode == "" {
		http.Error(w, "expected {\"mode\":\"…\"}", http.StatusBadRequest)
		return
	}
	if s.setModeFn == nil || !s.setModeFn(body.Mode) {
		http.Error(w, "unknown mode", http.StatusBadRequest)
		return
	}
	s.hub.Emit(Event{Kind: "state", Data: s.snapshot(s.isRunning())})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		var body struct {
			Title string `json:"title"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
		_ = json.NewEncoder(w).Encode(tasksOrEmpty(s.taskAdd, body.Title))
		return
	}
	list := []Task{}
	if s.tasksFn != nil {
		if got := s.tasksFn(); got != nil {
			list = got
		}
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleTaskDone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID int `json:"id"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&body)
	list := []Task{}
	if s.taskDone != nil {
		if got := s.taskDone(body.ID); got != nil {
			list = got
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleTaskClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	list := []Task{}
	if s.taskClear != nil {
		if got := s.taskClear(); got != nil {
			list = got
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func tasksOrEmpty(add func(string) []Task, title string) []Task {
	if add == nil {
		return []Task{}
	}
	if got := add(title); got != nil {
		return got
	}
	return []Task{}
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.imageAdd == nil {
		http.Error(w, "no image support", http.StatusNotFound)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "expected a file field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 8<<20)) // 8 MB cap
	if err != nil || len(data) == 0 {
		http.Error(w, "could not read the file", http.StatusBadRequest)
		return
	}
	mt := http.DetectContentType(data)
	if !strings.HasPrefix(mt, "image/") {
		http.Error(w, "not an image", http.StatusUnsupportedMediaType)
		return
	}
	n := s.imageAdd(data, mt)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"count": n})
}

func (s *Server) handleImageClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.imageClear != nil {
		s.imageClear()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCommands(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	list := []CommandInfo{}
	if s.commandsFn != nil {
		if got := s.commandsFn(); got != nil {
			list = got
		}
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleChanges(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	list := []Change{}
	if s.changesFn != nil {
		if got := s.changesFn(); got != nil {
			list = got
		}
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	path, did := "", false
	if s.undoFn != nil {
		path, did = s.undoFn()
	}
	s.hub.Emit(Event{Kind: "state", Data: s.snapshot(s.isRunning())})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"path": path, "did": did})
}

func (s *Server) handleRewind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	reverted := []string{}
	if s.rewindFn != nil {
		if got := s.rewindFn(); got != nil {
			reverted = got
		}
	}
	s.hub.Emit(Event{Kind: "state", Data: s.snapshot(s.isRunning())})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"reverted": reverted})
}

func (s *Server) handleRules(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var rl Rules
	if s.rulesFn != nil {
		rl = s.rulesFn()
	}
	_ = json.NewEncoder(w).Encode(rl)
}

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	list := []ModelInfo{}
	if s.modelsFn != nil {
		if got := s.modelsFn(); got != nil {
			list = got
		}
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&body); err != nil || body.Model == "" {
		http.Error(w, "expected {\"model\":\"…\"}", http.StatusBadRequest)
		return
	}
	if s.setModelFn != nil {
		s.setModelFn(body.Model)
	}
	s.hub.Emit(Event{Kind: "state", Data: s.snapshot(s.isRunning())})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.snapshot(s.isRunning()))
}

func (s *Server) snapshot(running bool) State {
	st := State{}
	if s.state != nil {
		st = s.state()
	}
	st.Running = running
	return st
}

func (s *Server) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
