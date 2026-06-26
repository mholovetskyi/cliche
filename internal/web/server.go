package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
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
	if s.static != nil {
		mux.Handle("/", http.FileServer(http.FS(s.static)))
	}
	return mux
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
