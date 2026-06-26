package web

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestApproveBlocksUntilAnswered(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Drain the approval id off the SSE stream so we know what to answer.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	idCh := make(chan string, 1)
	go func() {
		tmp := make([]byte, 1024)
		var acc strings.Builder
		for {
			n, err := resp.Body.Read(tmp)
			if n > 0 {
				acc.Write(tmp[:n])
				if i := strings.Index(acc.String(), `"kind":"approval"`); i >= 0 {
					var line string
					for _, l := range strings.Split(acc.String(), "\n") {
						if strings.Contains(l, `"approval"`) {
							line = strings.TrimPrefix(l, "data: ")
						}
					}
					var ev struct {
						Data approvalReq `json:"data"`
					}
					if json.Unmarshal([]byte(line), &ev) == nil && ev.Data.ID != "" {
						idCh <- ev.Data.ID
						return
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Approve() blocks the "agent" goroutine until the browser answers.
	decision := make(chan bool, 1)
	go func() { decision <- srv.Approve("run", "go test ./...") }()

	select {
	case id := <-idCh:
		_, _ = http.Post(ts.URL+"/api/approve", "application/json",
			strings.NewReader(`{"id":"`+id+`","allow":true}`))
	case <-time.After(3 * time.Second):
		t.Fatal("approval request never reached the SSE stream")
	}

	select {
	case ok := <-decision:
		if !ok {
			t.Fatal("Approve should return true after an allow")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Approve did not unblock after /api/approve")
	}
}

func TestTemplatesAndPreview(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi from preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(nil, nil, nil)
	srv.SetTemplates([]Template{{Name: "Website", Prompt: "build a site"}})
	srv.SetPreviewDir(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	r, err := http.Get(ts.URL + "/api/templates")
	if err != nil {
		t.Fatal(err)
	}
	var tpl []Template
	_ = json.NewDecoder(r.Body).Decode(&tpl)
	r.Body.Close()
	if len(tpl) != 1 || tpl[0].Name != "Website" {
		t.Fatalf("templates endpoint wrong: %+v", tpl)
	}

	pr, err := http.Get(ts.URL + "/preview/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(pr.Body)
	pr.Body.Close()
	if !strings.Contains(string(b), "hi from preview") {
		t.Fatalf("preview should serve the project index.html, got %q", b)
	}
}

func TestExportZipsProjectAndSkipsNoise(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>my site</h1>"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".cliche"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".cliche", "ledger"), []byte("secret-ish"), 0o644)

	srv := NewServer(nil, nil, nil)
	srv.SetPreviewDir(dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	r, err := http.Get(ts.URL + "/api/export")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("export is not a valid zip: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["index.html"] {
		t.Fatalf("export should contain the project files, got %v", names)
	}
	for n := range names {
		if strings.HasPrefix(n, ".cliche") {
			t.Fatalf("export must not include .cliche internals: %q", n)
		}
	}
}

func TestAuditEndpoint(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	srv.SetAudit(func() AuditView { return AuditView{OK: true, Entries: 7, Verified: 7, USD: 0.12, Turns: 3} })
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	r, _ := http.Get(ts.URL + "/api/audit")
	var v AuditView
	_ = json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	if !v.OK || v.Entries != 7 || v.USD != 0.12 {
		t.Fatalf("audit view wrong: %+v", v)
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
