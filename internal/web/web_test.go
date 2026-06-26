package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHubFanOut(t *testing.T) {
	h := NewHub()
	a, unA := h.Subscribe()
	b, unB := h.Subscribe()
	defer unB()

	h.Emit(Event{Kind: "delta", Text: "hi"})
	for _, ch := range []<-chan Event{a, b} {
		select {
		case e := <-ch:
			if e.Text != "hi" {
				t.Fatalf("got %q", e.Text)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive the event")
		}
	}

	// Unsubscribe closes the channel.
	unA()
	select {
	case _, ok := <-a:
		if ok {
			if _, ok2 := <-a; ok2 {
				t.Fatal("channel should close after unsubscribe")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("unsubscribed channel should close")
	}
}

func TestPromptRunsAndStreams(t *testing.T) {
	run := func(ctx context.Context, prompt string, emit func(Event)) error {
		emit(Event{Kind: "delta", Text: "building " + prompt})
		return nil
	}
	srv := NewServer(run, func() State { return State{Model: "mock", CapUSD: 5} }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req) // returns once headers (the "begin" frame) flush → subscribed
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	found := make(chan struct{}, 1)
	go func() {
		var acc strings.Builder
		tmp := make([]byte, 1024)
		for {
			n, err := resp.Body.Read(tmp)
			if n > 0 {
				acc.Write(tmp[:n])
				if strings.Contains(acc.String(), "building a site") {
					found <- struct{}{}
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	pr, err := http.Post(ts.URL+"/api/prompt", "application/json", strings.NewReader(`{"prompt":"a site"}`))
	if err != nil {
		t.Fatal(err)
	}
	if pr.StatusCode != http.StatusAccepted {
		t.Fatalf("prompt status = %d, want 202", pr.StatusCode)
	}

	select {
	case <-found:
		// streamed through SSE — success
	case <-time.After(3 * time.Second):
		t.Fatal("did not observe the streamed delta over SSE")
	}
}

func TestPromptRejectsBadBody(t *testing.T) {
	srv := NewServer(func(context.Context, string, func(Event)) error { return nil }, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	r, _ := http.Post(ts.URL+"/api/prompt", "application/json", strings.NewReader(`{}`))
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty prompt should be 400, got %d", r.StatusCode)
	}
}

func TestStateEndpoint(t *testing.T) {
	srv := NewServer(nil, func() State { return State{Model: "claude", SpentUSD: 0.5, CapUSD: 5} }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	r, err := http.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var st State
	if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Model != "claude" || st.SpentUSD != 0.5 {
		t.Fatalf("state round-trip wrong: %+v", st)
	}
}
