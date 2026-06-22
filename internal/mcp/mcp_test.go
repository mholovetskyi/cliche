package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// fakeServer wires a Client to an in-process MCP server goroutine over pipes.
func fakeServer() *Client {
	c2sR, c2sW := io.Pipe() // client -> server
	s2cR, s2cW := io.Pipe() // server -> client
	go func() {
		sc := bufio.NewScanner(c2sR)
		enc := json.NewEncoder(s2cW)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var req struct {
				ID     int             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			_ = json.Unmarshal([]byte(line), &req)
			switch req.Method {
			case "initialize":
				_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
					"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "fake"},
				}})
			case "notifications/initialized":
				// notification: no response
			case "tools/list":
				_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
					"tools": []map[string]any{{
						"name": "echo", "description": "echoes input",
						"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"msg": map[string]any{"type": "string"}}},
					}},
				}})
			case "tools/call":
				var p struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				}
				_ = json.Unmarshal(req.Params, &p)
				msg, _ := p.Arguments["msg"].(string)
				_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "echo: " + msg}}, "isError": false,
				}})
			}
		}
	}()
	return NewClient("fake", c2sW, s2cR, nil)
}

func TestManagerListAndCall(t *testing.T) {
	m, err := NewManager(context.Background(), []*Client{fakeServer()})
	if err != nil {
		t.Fatal(err)
	}
	tools := m.Tools()
	if len(tools) != 1 || tools[0].Name != "mcp__fake__echo" {
		t.Fatalf("expected one namespaced tool, got %+v", tools)
	}
	out, isErr, err := m.Call(context.Background(), "mcp__fake__echo", json.RawMessage(`{"msg":"hi"}`))
	if err != nil || isErr {
		t.Fatalf("call failed: err=%v isErr=%v", err, isErr)
	}
	if out != "echo: hi" {
		t.Fatalf("output round-trip wrong: %q", out)
	}
}

func TestManagerUnknownTool(t *testing.T) {
	m := &Manager{route: map[string]routed{}}
	if _, isErr, err := m.Call(context.Background(), "mcp__x__y", nil); err == nil || !isErr {
		t.Fatal("unknown tool must return an error")
	}
}

func TestContextCancelUnblocks(t *testing.T) {
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()
	go func() {
		sc := bufio.NewScanner(c2sR)
		enc := json.NewEncoder(s2cW)
		for sc.Scan() {
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(sc.Bytes(), &req) != nil || req.ID == 0 {
				continue
			}
			if req.Method == "initialize" {
				_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}})
			}
			// Deliberately never answer tools/list — simulate a stalled server.
		}
	}()
	c := NewClient("silent", c2sW, s2cR, nil)
	if err := c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	if _, err := c.ListTools(ctx); err == nil {
		t.Fatal("a stalled server must not hang; expected a cancellation error")
	}
	// After a cancelled call the client refuses further calls (no concurrent read).
	if _, err := c.ListTools(context.Background()); err == nil {
		t.Fatal("expected the client to be marked unavailable after cancellation")
	}
}

func TestCallToolErrorResponse(t *testing.T) {
	// A server that returns a JSON-RPC error must surface it.
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()
	go func() {
		sc := bufio.NewScanner(c2sR)
		enc := json.NewEncoder(s2cW)
		for sc.Scan() {
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(sc.Bytes(), &req) != nil {
				continue
			}
			if req.ID == 0 {
				continue // a notification (e.g. notifications/initialized): no reply
			}
			if req.Method == "initialize" {
				_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}})
				continue
			}
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32603, "message": "boom"}})
		}
	}()
	c := NewClient("bad", c2sW, s2cR, nil)
	if err := c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListTools(context.Background()); err == nil {
		t.Fatal("expected the server error to surface")
	}
}
