package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/mcp"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/tools"
)

// mcpAdapter exposes an mcp.Manager to the agent as a permission-gated MCP tool
// source. MCP tools can do anything (filesystem, network), so a call must be
// pre-authorized (--allow-mcp / --yolo) or interactively approved.
type mcpAdapter struct {
	mgr     *mcp.Manager
	allow   bool
	approve tools.Approver
}

func (a *mcpAdapter) Tools() []provider.ToolSpec {
	var specs []provider.ToolSpec
	for _, t := range a.mgr.Tools() {
		specs = append(specs, provider.ToolSpec{Name: t.Name, Description: t.Description, Schema: t.InputSchema})
	}
	return specs
}

func (a *mcpAdapter) Call(ctx context.Context, name string, raw json.RawMessage) tools.Result {
	if !a.permit(name) {
		return tools.Result{Output: "permission denied: mcp tool " + name + " (use --allow-mcp)", Success: false}
	}
	out, isErr, err := a.mgr.Call(ctx, name, raw)
	if err != nil {
		return tools.Result{Output: "mcp error: " + err.Error(), Success: false}
	}
	return tools.Result{Output: out, Success: !isErr}
}

func (a *mcpAdapter) permit(name string) bool {
	if a.allow {
		return true
	}
	if a.approve != nil {
		return a.approve("mcp", name)
	}
	return false
}

// startMCP launches the configured MCP servers and returns an adapter plus a
// cleanup func. With no servers configured it returns (nil, no-op, nil).
func startMCP(servers []config.MCPServer, allow bool, approve tools.Approver) (*mcpAdapter, func(), error) {
	if len(servers) == 0 {
		return nil, func() {}, nil
	}
	var clients []mcp.Conn
	for _, s := range servers {
		// A server is HTTP if it has a URL, otherwise stdio (a launched command).
		if s.URL != "" {
			clients = append(clients, mcp.StartHTTP(s.Name, s.URL))
			continue
		}
		c, err := mcp.StartStdio(context.Background(), s.Name, s.Command, s.Args, s.Env)
		if err != nil {
			for _, started := range clients {
				_ = started.Close()
			}
			return nil, func() {}, fmt.Errorf("starting mcp server %q: %w", s.Name, err)
		}
		clients = append(clients, c)
	}
	mgr, err := mcp.NewManager(context.Background(), clients)
	if err != nil {
		mgr.Close()
		return nil, func() {}, err
	}
	return &mcpAdapter{mgr: mgr, allow: allow, approve: approve}, mgr.Close, nil
}
