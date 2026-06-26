package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sync"
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
}

func NewServer(run Runner, state func() State, static fs.FS) *Server {
	return &Server{hub: NewHub(), run: run, state: state, static: static}
}

// Handler returns the HTTP routes. Mounted by the CLI on a localhost listener.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/prompt", s.handlePrompt)
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
