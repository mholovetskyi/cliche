package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// maxResponseBytes caps a single JSON-RPC HTTP response body, so a hostile or
// buggy server can't exhaust memory with an unbounded reply.
const maxResponseBytes = 16 << 20 // 16 MiB

// HTTPClient speaks MCP over the Streamable HTTP transport: each JSON-RPC
// message is POSTed to a single endpoint, and the server replies with either a
// plain JSON response or a text/event-stream (SSE) carrying the response. A
// server-assigned Mcp-Session-Id (returned on initialize) is echoed on every
// subsequent request. Pure stdlib (net/http). It satisfies Conn, so the Manager
// treats HTTP and stdio servers identically.
type HTTPClient struct {
	name    string
	url     string
	headers map[string]string // extra request headers (e.g. Authorization for OAuth connectors)
	hc      *http.Client

	mu      sync.Mutex
	id      int
	session string // Mcp-Session-Id, set after initialize
}

// StartHTTP returns a client for a Streamable-HTTP MCP server at url.
func StartHTTP(name, url string) *HTTPClient { return StartHTTPWithHeaders(name, url, nil) }

// StartHTTPWithHeaders is StartHTTP plus extra headers sent on every request —
// used to attach a connector's OAuth bearer token (Authorization header).
func StartHTTPWithHeaders(name, url string, headers map[string]string) *HTTPClient {
	return &HTTPClient{name: name, url: url, headers: headers, hc: &http.Client{Timeout: 120 * time.Second}}
}

func (h *HTTPClient) Name() string { return h.name }

// post sends one JSON-RPC payload and returns the raw HTTP response. The caller
// must close the body. The Mcp-Session-Id header is sent (if known) and updated
// from the response.
func (h *HTTPClient) post(ctx context.Context, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json, text/event-stream")
	for k, v := range h.headers { // connector auth, etc.
		req.Header.Set(k, v)
	}
	if sid := h.sessionID(); sid != "" {
		req.Header.Set("mcp-session-id", sid)
	}
	resp, err := h.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if sid := resp.Header.Get("mcp-session-id"); sid != "" {
		h.setSessionID(sid)
	}
	return resp, nil
}

func (h *HTTPClient) sessionID() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.session
}

func (h *HTTPClient) setSessionID(s string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.session = s
}

func (h *HTTPClient) nextID() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.id++
	return h.id
}

// call performs a JSON-RPC request/response round trip over HTTP.
func (h *HTTPClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := h.nextID()
	resp, err := h.post(ctx, rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("mcp %s: %w", h.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("mcp %s: HTTP %d: %s", h.name, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	rpc, err := decodeResponse(resp, id)
	if err != nil {
		return nil, fmt.Errorf("mcp %s: %w", h.name, err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("mcp %s: %s (code %d)", h.name, rpc.Error.Message, rpc.Error.Code)
	}
	return rpc.Result, nil
}

// notify sends a JSON-RPC notification (no id, no response expected).
func (h *HTTPClient) notify(ctx context.Context, method string, params any) error {
	resp, err := h.post(ctx, rpcNotification{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// decodeResponse reads a JSON-RPC response from an HTTP body that is either
// application/json (one object) or text/event-stream (SSE; the response is in a
// data: line whose id matches).
func decodeResponse(resp *http.Response, wantID int) (*rpcResponse, error) {
	if strings.Contains(resp.Header.Get("content-type"), "text/event-stream") {
		return decodeSSE(resp.Body, wantID)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if len(data) >= maxResponseBytes {
		return nil, fmt.Errorf("response exceeded %d bytes", maxResponseBytes)
	}
	var out rpcResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &out, nil
}

// decodeSSE scans Server-Sent Events for the JSON-RPC response with wantID.
func decodeSSE(r io.Reader, wantID int) (*rpcResponse, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" {
			continue
		}
		var out rpcResponse
		if err := json.Unmarshal([]byte(data), &out); err != nil {
			continue // a notification or unrelated event
		}
		if out.ID == wantID || out.Error != nil {
			return &out, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("event stream ended without a response")
}

// Initialize performs the MCP handshake over HTTP.
func (h *HTTPClient) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "cliche", "version": "0.1"},
	}
	if _, err := h.call(ctx, "initialize", params); err != nil {
		return err
	}
	return h.notify(ctx, "notifications/initialized", map[string]any{})
}

// ListTools returns the server's advertised tools.
func (h *HTTPClient) ListTools(ctx context.Context) ([]Tool, error) {
	res, err := h.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseToolsResult(res)
}

// CallTool invokes a tool and returns its text content plus the error flag.
func (h *HTTPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	res, err := h.call(ctx, "tools/call", toolCallParams(name, args))
	if err != nil {
		return "", false, err
	}
	return parseCallResult(res)
}

// Close terminates the session (best effort) — DELETE per the spec.
func (h *HTTPClient) Close() error {
	sid := h.sessionID()
	if sid == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, h.url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("mcp-session-id", sid)
	if resp, err := h.hc.Do(req); err == nil {
		resp.Body.Close()
	}
	return nil
}
