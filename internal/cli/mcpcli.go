package cli

import (
	"fmt"
	"io"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/style"
)

// mcpServers merges the project's configured MCP servers with any contributed by
// installed plugins and any connected OAuth connectors (the set launched for a
// run).
func mcpServers(root string, cfg config.Config) []config.MCPServer {
	all := append(append([]config.MCPServer(nil), cfg.MCP...), pluginMCP(root)...)
	return append(all, connectorMCP()...)
}

func renderMCP(out io.Writer, servers []config.MCPServer) {
	if len(servers) == 0 {
		fmt.Fprintln(out, "  no MCP servers configured")
		fmt.Fprintln(out, "  "+style.Gray("add them under `mcp` in .cliche/config.json or a plugin manifest (stdio command or HTTP url)"))
		return
	}
	fmt.Fprintln(out, "\n  "+style.BoldWhite("mcp servers")+style.Gray("  ·  external tools, permission-gated by the same rules"))
	for _, m := range servers {
		via, kind := m.Command, "stdio"
		if m.URL != "" {
			via, kind = m.URL, "http"
		}
		fmt.Fprintf(out, "  %s %s %s\n", style.White(style.Pad(m.Name, 16)), style.Gray(style.Pad(kind, 6)), style.Gray(via))
	}
}

// showMCP (/mcp) lists the configured MCP servers in-session.
func (s *session) showMCP() { renderMCP(s.out, mcpServers(s.dir, s.cfg)) }

// cmdMcp is `cliche mcp [install <name>]`: list or install MCP servers.
func cmdMcp(args []string, out, errOut io.Writer) int {
	if len(args) > 0 && args[0] == "install" {
		return cmdMcpInstall(args[1:], out, errOut)
	}
	cfg, _ := config.Load(".")
	renderMCP(out, mcpServers(".", cfg))
	return 0
}
