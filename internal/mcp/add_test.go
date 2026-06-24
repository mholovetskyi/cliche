package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeConn struct {
	name  string
	tools []Tool
}

func (f *fakeConn) Name() string                              { return f.name }
func (f *fakeConn) Initialize(context.Context) error          { return nil }
func (f *fakeConn) ListTools(context.Context) ([]Tool, error) { return f.tools, nil }
func (f *fakeConn) CallTool(_ context.Context, name string, _ json.RawMessage) (string, bool, error) {
	return "called " + name, false, nil
}
func (f *fakeConn) Close() error { return nil }

func TestManagerAddAttachesLive(t *testing.T) {
	m, err := NewManager(context.Background(), []Conn{&fakeConn{name: "a", tools: []Tool{{Name: "t1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tools()) != 1 {
		t.Fatalf("start: want 1 tool, got %d", len(m.Tools()))
	}

	// Hot-attach a second server without restarting the first.
	if err := m.Add(context.Background(), &fakeConn{name: "b", tools: []Tool{{Name: "t2"}}}); err != nil {
		t.Fatal(err)
	}
	if len(m.Tools()) != 2 {
		t.Fatalf("after Add: want 2 tools, got %d", len(m.Tools()))
	}
	// The newly attached tool is namespaced and routable.
	out, _, err := m.Call(context.Background(), "mcp__b__t2", nil)
	if err != nil || out != "called t2" {
		t.Fatalf("call mcp__b__t2 = %q, %v", out, err)
	}
	// And the original still routes.
	if out, _, _ := m.Call(context.Background(), "mcp__a__t1", nil); out != "called t1" {
		t.Fatalf("original server should still route, got %q", out)
	}
}
