package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rpcReply builds the JSON-RPC result for a given method, mirroring the stdio
// fakeServer so both transports are exercised against the same behavior.
func rpcReply(id int, method string, params json.RawMessage) (map[string]any, bool) {
	switch method {
	case "initialize":
		return map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"protocolVersion": "2024-11-05"}}, true
	case "tools/list":
		return map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
			"tools": []map[string]any{{"name": "echo", "description": "echoes input",
				"inputSchema": map[string]any{"type": "object"}}},
		}}, true
	case "tools/call":
		var p struct {
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(params, &p)
		msg, _ := p.Arguments["msg"].(string)
		return map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": "echo: " + msg}}, "isError": false,
		}}, true
	default:
		return nil, false // a notification: no reply
	}
}

// httpMCPServer is a Streamable-HTTP MCP test server. sse controls whether it
// answers with text/event-stream or plain application/json.
func httpMCPServer(t *testing.T, sse bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		reply, has := rpcReply(req.ID, req.Method, req.Params)
		if req.Method == "initialize" {
			w.Header().Set("mcp-session-id", "sess-123")
		}
		if !has {
			w.WriteHeader(http.StatusAccepted) // notification
			return
		}
		if sse {
			w.Header().Set("content-type", "text/event-stream")
			b, _ := json.Marshal(reply)
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
			return
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
}

func runHTTPRoundTrip(t *testing.T, sse bool) {
	t.Helper()
	srv := httpMCPServer(t, sse)
	defer srv.Close()

	c := StartHTTP("remote", srv.URL)
	m, err := NewManager(context.Background(), []Conn{c})
	if err != nil {
		t.Fatalf("manager init: %v", err)
	}
	if c.sessionID() != "sess-123" {
		t.Fatalf("session id not captured from initialize, got %q", c.sessionID())
	}
	tools := m.Tools()
	if len(tools) != 1 || tools[0].Name != "mcp__remote__echo" {
		t.Fatalf("expected one namespaced tool, got %+v", tools)
	}
	out, isErr, err := m.Call(context.Background(), "mcp__remote__echo", json.RawMessage(`{"msg":"hi"}`))
	if err != nil || isErr {
		t.Fatalf("call failed: err=%v isErr=%v", err, isErr)
	}
	if out != "echo: hi" {
		t.Fatalf("round-trip wrong: %q", out)
	}
}

func TestHTTPMCPJSON(t *testing.T) { runHTTPRoundTrip(t, false) }
func TestHTTPMCPSSE(t *testing.T)  { runHTTPRoundTrip(t, true) }

func TestHTTPMCPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	c := StartHTTP("remote", srv.URL)
	if err := c.Initialize(context.Background()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected an HTTP 403 error, got %v", err)
	}
}
