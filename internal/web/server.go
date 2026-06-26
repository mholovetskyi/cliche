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
	Model    string  `json:"model"`
	Provider string  `json:"provider"`
	Mode     string  `json:"mode"`
	SpentUSD float64 `json:"spent_usd"`
	CapUSD   float64 `json:"cap_usd"`
	CtxFrac  float64 `json:"ctx_frac"`
	Running  bool    `json:"running"`
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
	static fs.FS // embedded SPA assets (may be nil during early bring-up)

	mu      sync.Mutex
	running bool

	apMu      sync.Mutex
	apSeq     int
	approvals map[string]chan bool // pending approval id → answer channel

	templates  []Template
	previewDir string // project root, served read-only at /preview/ for the live preview iframe
	audit      func() AuditView
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
	// The live preview serves the project files (what the agent is building) so
	// an iframe can show the result. Localhost-only, the user's own files;
	// http.Dir blocks path traversal outside the root.
	if s.previewDir != "" {
		mux.Handle("/preview/", http.StripPrefix("/preview/", http.FileServer(http.Dir(s.previewDir))))
	}
	if s.static != nil {
		mux.Handle("/", http.FileServer(http.FS(s.static)))
	}
	return mux
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
	s.running = true
	s.mu.Unlock()

	go func() {
		s.hub.Emit(Event{Kind: "state", Data: s.snapshot(true)})
		err := s.run(context.Background(), body.Prompt, s.hub.Emit)
		if err != nil {
			s.hub.Emit(Event{Kind: "error", Text: err.Error()})
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		s.hub.Emit(Event{Kind: "end"})
		s.hub.Emit(Event{Kind: "state", Data: s.snapshot(false)})
	}()
	w.WriteHeader(http.StatusAccepted)
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
