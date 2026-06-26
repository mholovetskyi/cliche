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

func TestSetupEndpoint(t *testing.T) {
	var got string
	srv := NewServer(nil, nil, nil)
	srv.SetSetup(func(p, k string) error { got = p + ":" + k; return nil })
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	if r, _ := http.Post(ts.URL+"/api/setup", "application/json", strings.NewReader(`{"provider":"groq","key":"gsk_x"}`)); r.StatusCode != http.StatusNoContent {
		t.Fatalf("setup = %d, want 204", r.StatusCode)
	}
	if got != "groq:gsk_x" {
		t.Fatalf("setup callback got %q", got)
	}
	// Connected now → a second setup is rejected.
	if r, _ := http.Post(ts.URL+"/api/setup", "application/json", strings.NewReader(`{"provider":"x","key":"y"}`)); r.StatusCode != http.StatusConflict {
		t.Fatalf("second setup should be 409, got %d", r.StatusCode)
	}
}

func TestSetupRejectsEmptyProvider(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	srv.SetSetup(func(p, k string) error { return nil })
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	if r, _ := http.Post(ts.URL+"/api/setup", "application/json", strings.NewReader(`{}`)); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty provider should be 400, got %d", r.StatusCode)
	}
}

func TestSessionEndpoints(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	srv.SetSessions(
		func() []SessionMeta {
			return []SessionMeta{{ID: "s1", Title: "first", Active: true}, {ID: "s2", Title: "second"}}
		},
		func() string { return "s3" },
		func(id string) []Msg { return []Msg{{Role: "user", Text: "hi " + id}} },
		func() (string, []Msg) { return "s1", []Msg{{Role: "assistant", Text: "hello"}} },
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var list []SessionMeta
	r, _ := http.Get(ts.URL + "/api/sessions")
	_ = json.NewDecoder(r.Body).Decode(&list)
	r.Body.Close()
	if len(list) != 2 || !list[0].Active {
		t.Fatalf("sessions list wrong: %+v", list)
	}

	var nw struct {
		ID string `json:"id"`
	}
	r, _ = http.Post(ts.URL+"/api/sessions/new", "application/json", nil)
	_ = json.NewDecoder(r.Body).Decode(&nw)
	r.Body.Close()
	if nw.ID != "s3" {
		t.Fatalf("new session id = %q, want s3", nw.ID)
	}

	var sel struct {
		Messages []Msg `json:"messages"`
	}
	r, _ = http.Post(ts.URL+"/api/sessions/select", "application/json", strings.NewReader(`{"id":"s2"}`))
	_ = json.NewDecoder(r.Body).Decode(&sel)
	r.Body.Close()
	if len(sel.Messages) != 1 || sel.Messages[0].Text != "hi s2" {
		t.Fatalf("select wrong: %+v", sel)
	}

	var cur struct {
		ID       string `json:"id"`
		Messages []Msg  `json:"messages"`
	}
	r, _ = http.Get(ts.URL + "/api/session")
	_ = json.NewDecoder(r.Body).Decode(&cur)
	r.Body.Close()
	if cur.ID != "s1" || len(cur.Messages) != 1 {
		t.Fatalf("current session wrong: %+v", cur)
	}
}

func TestFileEndpoints(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	srv.SetFiles(
		func() []FileNode {
			return []FileNode{{Name: "src", Path: "src", Dir: true, Children: []FileNode{{Name: "a.go", Path: "src/a.go"}}}}
		},
		func(rel string) (string, bool) {
			if rel == "src/a.go" {
				return "package main", true
			}
			return "", false
		},
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var tree []FileNode
	r, _ := http.Get(ts.URL + "/api/files")
	_ = json.NewDecoder(r.Body).Decode(&tree)
	r.Body.Close()
	if len(tree) != 1 || !tree[0].Dir || len(tree[0].Children) != 1 {
		t.Fatalf("file tree wrong: %+v", tree)
	}

	r, _ = http.Get(ts.URL + "/api/file?path=src/a.go")
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if string(b) != "package main" {
		t.Fatalf("file content = %q", b)
	}

	r, _ = http.Get(ts.URL + "/api/file?path=nope")
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("missing file should 404, got %d", r.StatusCode)
	}
	r.Body.Close()
}

func TestControlEndpoints(t *testing.T) {
	var gotMode, gotModel string
	srv := NewServer(nil, func() State { return State{Mode: "suggest"} }, nil)
	srv.SetControls(
		func(m string) bool {
			if m == "bogus" {
				return false
			}
			gotMode = m
			return true
		},
		func() []ModelInfo { return []ModelInfo{{Model: "gpt-4o", InputPerM: 2.5, OutputPerM: 10}} },
		func(m string) { gotModel = m },
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	if r, _ := http.Post(ts.URL+"/api/mode", "application/json", strings.NewReader(`{"mode":"plan"}`)); r.StatusCode != http.StatusNoContent {
		t.Fatalf("mode = %d, want 204", r.StatusCode)
	}
	if gotMode != "plan" {
		t.Fatalf("mode not applied: %q", gotMode)
	}
	if r, _ := http.Post(ts.URL+"/api/mode", "application/json", strings.NewReader(`{"mode":"bogus"}`)); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown mode should 400, got %d", r.StatusCode)
	}

	var ms []ModelInfo
	r, _ := http.Get(ts.URL + "/api/models")
	_ = json.NewDecoder(r.Body).Decode(&ms)
	r.Body.Close()
	if len(ms) != 1 || ms[0].Model != "gpt-4o" {
		t.Fatalf("models wrong: %+v", ms)
	}

	if r, _ := http.Post(ts.URL+"/api/model", "application/json", strings.NewReader(`{"model":"gpt-4o"}`)); r.StatusCode != http.StatusNoContent {
		t.Fatalf("model = %d, want 204", r.StatusCode)
	}
	if gotModel != "gpt-4o" {
		t.Fatalf("model not switched: %q", gotModel)
	}
}

func TestEditAndRulesEndpoints(t *testing.T) {
	undone := false
	srv := NewServer(nil, nil, nil)
	srv.SetEdits(
		func() []Change { return []Change{{Path: "a.txt", Before: "old", After: "new"}} },
		func() (string, bool) { undone = true; return "a.txt", true },
		func() []string { return []string{"a.txt", "b.txt"} },
	)
	srv.SetRules(func() Rules {
		return Rules{Mode: "plan", ModeDesc: "read-only", Deny: []string{"rm -rf"}, MaxTurns: 50}
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var ch []Change
	r, _ := http.Get(ts.URL + "/api/changes")
	_ = json.NewDecoder(r.Body).Decode(&ch)
	r.Body.Close()
	if len(ch) != 1 || ch[0].After != "new" {
		t.Fatalf("changes wrong: %+v", ch)
	}

	var u struct {
		Path string `json:"path"`
		Did  bool   `json:"did"`
	}
	r, _ = http.Post(ts.URL+"/api/undo", "application/json", nil)
	_ = json.NewDecoder(r.Body).Decode(&u)
	r.Body.Close()
	if !u.Did || u.Path != "a.txt" || !undone {
		t.Fatalf("undo wrong: %+v", u)
	}

	var rw struct {
		Reverted []string `json:"reverted"`
	}
	r, _ = http.Post(ts.URL+"/api/rewind", "application/json", nil)
	_ = json.NewDecoder(r.Body).Decode(&rw)
	r.Body.Close()
	if len(rw.Reverted) != 2 {
		t.Fatalf("rewind wrong: %+v", rw)
	}

	var rl Rules
	r, _ = http.Get(ts.URL + "/api/rules")
	_ = json.NewDecoder(r.Body).Decode(&rl)
	r.Body.Close()
	if rl.Mode != "plan" || len(rl.Deny) != 1 || rl.MaxTurns != 50 {
		t.Fatalf("rules wrong: %+v", rl)
	}
}

func TestStopCancelsRun(t *testing.T) {
	started := make(chan struct{})
	run := func(ctx context.Context, _ string, _ func(Event)) error {
		close(started)
		<-ctx.Done() // blocks until /api/stop cancels the context
		return ctx.Err()
	}
	srv := NewServer(run, func() State { return State{} }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	_, _ = http.Post(ts.URL+"/api/prompt", "application/json", strings.NewReader(`{"prompt":"go"}`))
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("run never started")
	}
	if r, _ := http.Post(ts.URL+"/api/stop", "application/json", nil); r.StatusCode != http.StatusNoContent {
		t.Fatalf("stop = %d, want 204", r.StatusCode)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if !srv.isRunning() {
			return // run unblocked after cancel — success
		}
	}
	t.Fatal("run did not stop after /api/stop")
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
