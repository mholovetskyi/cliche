// Package mcp is a minimal Model Context Protocol client: it speaks JSON-RPC
// 2.0 over a newline-delimited stdio transport, performs the initialize
// handshake, lists a server's tools, and calls them. It is intentionally
// transport-agnostic (any io.Reader/io.Writer) so it can be tested against an
// in-process fake server without spawning a subprocess.
//
// The package has no dependency on the rest of cliche; an adapter in the CLI
// layer maps mcp.Tool -> provider.ToolSpec and routes calls through the Trust
// Kernel's permission gate.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

const protocolVersion = "2024-11-05"

// Tool describes an MCP tool. For Manager-returned tools, Name is namespaced
// as "mcp__<server>__<tool>".
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Conn is one MCP server connection, independent of transport (stdio or HTTP).
type Conn interface {
	Name() string
	Initialize(ctx context.Context) error
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, args json.RawMessage) (string, bool, error)
	Close() error
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client is a single MCP server connection.
type Client struct {
	name   string
	w      io.Writer
	scan   *bufio.Scanner
	closer io.Closer
	mu     sync.Mutex
	id     int
	broken bool // a prior call was abandoned (ctx) with a reader still blocked
}

type callResult struct {
	raw json.RawMessage
	err error
}

// NewClient wires a client to an already-open transport. toServer receives
// client->server bytes; fromServer yields server->client bytes; closer (may be
// nil) tears the transport down.
func NewClient(name string, toServer io.Writer, fromServer io.Reader, closer io.Closer) *Client {
	sc := bufio.NewScanner(fromServer)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // allow large messages
	return &Client{name: name, w: toServer, scan: sc, closer: closer}
}

// Name returns the server name.
func (c *Client) Name() string { return c.name }

func (c *Client) writeMsg(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = c.w.Write(b)
	return err
}

// call sends a request and reads until the matching response, skipping
// notifications and unrelated lines.
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken {
		return nil, fmt.Errorf("mcp %s: connection unavailable (a prior call was cancelled)", c.name)
	}
	c.id++
	id := c.id
	if err := c.writeMsg(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.broken = true
		return nil, err
	}

	// Read in a goroutine so a stalled server can't ignore ctx. The mutex is
	// held for the whole exchange, so there's never more than one reader on the
	// scanner — except the rare cancelled-call case, which we fence off with
	// `broken` so no second reader is ever started concurrently. The abandoned
	// reader unblocks when the transport closes (Close/cleanup kills the proc).
	ch := make(chan callResult, 1)
	go func() {
		for {
			if !c.scan.Scan() {
				if err := c.scan.Err(); err != nil {
					ch <- callResult{nil, fmt.Errorf("mcp %s: %w", c.name, err)}
				} else {
					ch <- callResult{nil, fmt.Errorf("mcp %s: connection closed", c.name)}
				}
				return
			}
			line := strings.TrimSpace(c.scan.Text())
			if line == "" {
				continue
			}
			var resp rpcResponse
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				continue // not a JSON-RPC response (notification/log line)
			}
			if resp.ID != id {
				continue // a notification (id 0) or a stale response
			}
			if resp.Error != nil {
				ch <- callResult{nil, fmt.Errorf("mcp %s: %s (code %d)", c.name, resp.Error.Message, resp.Error.Code)}
				return
			}
			ch <- callResult{resp.Result, nil}
			return
		}
	}()

	select {
	case <-ctx.Done():
		c.broken = true // the reader is still blocked; refuse further calls to avoid a concurrent Scan
		return nil, ctx.Err()
	case r := <-ch:
		return r.raw, r.err
	}
}

func (c *Client) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeMsg(rpcNotification{JSONRPC: "2.0", Method: method, Params: params})
}

// Initialize performs the MCP handshake.
func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "cliche", "version": "0.1"},
	}
	if _, err := c.call(ctx, "initialize", params); err != nil {
		return err
	}
	return c.notify("notifications/initialized", map[string]any{})
}

// ListTools returns the server's advertised tools.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	res, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseToolsResult(res)
}

// parseToolsResult decodes a tools/list result (shared by both transports).
func parseToolsResult(res json.RawMessage) ([]Tool, error) {
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

// toolCallParams builds the tools/call params with normalized arguments.
func toolCallParams(name string, args json.RawMessage) map[string]any {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	} else {
		params["arguments"] = map[string]any{}
	}
	return params
}

// parseCallResult decodes a tools/call result into concatenated text + isError.
func parseCallResult(res json.RawMessage) (string, bool, error) {
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return "", false, err
	}
	var sb strings.Builder
	for _, b := range out.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String(), out.IsError, nil
}

// CallTool invokes a tool with the given raw JSON arguments and returns the
// concatenated text content plus whether the server flagged an error.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	res, err := c.call(ctx, "tools/call", toolCallParams(name, args))
	if err != nil {
		return "", false, err
	}
	return parseCallResult(res)
}

// Close tears down the transport.
func (c *Client) Close() error {
	if c.closer != nil {
		return c.closer.Close()
	}
	return nil
}

// ---- Manager: many servers, namespaced tools ----

type routed struct {
	client Conn
	tool   string
}

// Manager aggregates several MCP servers and exposes their tools under
// "mcp__<server>__<tool>" names.
type Manager struct {
	clients []Conn
	tools   []Tool
	route   map[string]routed
}

// NewManager initializes each client (handshake + tools/list) and namespaces
// their tools. A server that fails to initialize is reported as an error; any
// already-initialized clients are still returned for cleanup.
func NewManager(ctx context.Context, clients []Conn) (*Manager, error) {
	m := &Manager{route: map[string]routed{}}
	for _, c := range clients {
		m.clients = append(m.clients, c)
		if err := c.Initialize(ctx); err != nil {
			return m, fmt.Errorf("initializing mcp server %q: %w", c.Name(), err)
		}
		ts, err := c.ListTools(ctx)
		if err != nil {
			return m, fmt.Errorf("listing tools for mcp server %q: %w", c.Name(), err)
		}
		for _, t := range ts {
			ns := "mcp__" + c.Name() + "__" + t.Name
			m.tools = append(m.tools, Tool{Name: ns, Description: t.Description, InputSchema: t.InputSchema})
			m.route[ns] = routed{client: c, tool: t.Name}
		}
	}
	return m, nil
}

// Tools returns the namespaced tools across all servers.
func (m *Manager) Tools() []Tool { return m.tools }

// Call routes a namespaced tool call to the owning server.
func (m *Manager) Call(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	r, ok := m.route[name]
	if !ok {
		return "", true, fmt.Errorf("unknown mcp tool %q", name)
	}
	return r.client.CallTool(ctx, r.tool, args)
}

// Close tears down every server connection.
func (m *Manager) Close() {
	for _, c := range m.clients {
		_ = c.Close()
	}
}
