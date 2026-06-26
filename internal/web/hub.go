// Package web is the Cliche Studio backend: a zero-dependency HTTP + Server-Sent
// Events server that exposes the same agent and Trust Kernel as the CLI to a
// browser UI. It speaks SSE (server→client streaming) and plain JSON POST
// (client→server) over net/http — no WebSocket library, so the core stays
// stdlib-only and the single static binary is preserved.
package web

// Event is one thing the UI should render: a streamed token, a tool call, a
// result, a trust-state tick, or a lifecycle marker. Kept transport-agnostic and
// JSON-friendly so the CLI layer can map agent.Event onto it without this package
// importing the agent.
type Event struct {
	Kind string `json:"kind"`           // delta | tool_call | tool_result | state | begin | end | error
	Text string `json:"text,omitempty"` // token text / tool label / message
	Data any    `json:"data,omitempty"` // structured payload (e.g. the trust state)
}

// Hub fans one event stream out to every connected browser client. A subscriber
// that can't keep up drops events rather than blocking the agent — the UI is a
// view, never a backpressure source on the run.
type Hub struct {
	subscribe   chan chan Event
	unsubscribe chan chan Event
	broadcast   chan Event
	subs        map[chan Event]struct{}
}

// NewHub returns a started hub (its goroutine owns the subscriber set, so no
// lock is needed and there is no data race between subscribe/broadcast).
func NewHub() *Hub {
	h := &Hub{
		subscribe:   make(chan chan Event),
		unsubscribe: make(chan chan Event),
		broadcast:   make(chan Event, 256),
		subs:        map[chan Event]struct{}{},
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for {
		select {
		case ch := <-h.subscribe:
			h.subs[ch] = struct{}{}
		case ch := <-h.unsubscribe:
			if _, ok := h.subs[ch]; ok {
				delete(h.subs, ch)
				close(ch)
			}
		case e := <-h.broadcast:
			for ch := range h.subs {
				select {
				case ch <- e:
				default: // slow client: drop this event for it rather than stall the agent
				}
			}
		}
	}
}

// Subscribe registers a new client and returns its event channel plus an unsub
// func to call when the connection closes.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	h.subscribe <- ch
	var once bool
	return ch, func() {
		if !once {
			once = true
			h.unsubscribe <- ch
		}
	}
}

// Emit publishes an event to all subscribers (non-blocking for the caller).
func (h *Hub) Emit(e Event) { h.broadcast <- e }
