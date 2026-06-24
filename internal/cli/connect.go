package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strings"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/oauth"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// knownConnector is a built-in OAuth connector: an MCP server reached over HTTP,
// gated by an OAuth 2.0 device flow. The client id is BYO (register an OAuth app
// once) — Cliche hosts no credentials, in the same spirit as BYO API keys.
type knownConnector struct {
	name      string
	desc      string
	mcpURL    string
	deviceURL string
	tokenURL  string
	scopes    []string
	clientEnv string // env var supplying the BYO OAuth-app client id
	register  string // where to create the OAuth app (shown when the id is missing)
}

var knownConnectors = map[string]knownConnector{
	"github": {
		name:      "github",
		desc:      "GitHub MCP — repos, issues, pull requests",
		mcpURL:    "https://api.githubcopilot.com/mcp/",
		deviceURL: "https://github.com/login/device/code",
		tokenURL:  "https://github.com/login/oauth/access_token",
		scopes:    []string{"repo", "read:org", "read:user"},
		clientEnv: "CLICHE_GITHUB_CLIENT_ID",
		register:  "github.com/settings/applications/new (OAuth app; enable Device Flow)",
	},
}

func connectorClientID(c knownConnector) string { return strings.TrimSpace(os.Getenv(c.clientEnv)) }

// connectorMCP returns MCP servers for every connector that has been connected
// (token stored globally), with the OAuth bearer attached. Connect once, and the
// connector is available in every project's chat — like a hosted connector, but
// the token lives 0600 in your config dir.
func connectorMCP() []config.MCPServer {
	var out []config.MCPServer
	for _, name := range secrets.ConnectedNames() {
		c, ok := knownConnectors[name]
		if !ok {
			continue
		}
		tok, ok := secrets.Connector(name)
		if !ok || tok.Token == "" {
			continue
		}
		out = append(out, config.MCPServer{
			Name:    name,
			URL:     c.mcpURL,
			Headers: map[string]string{"Authorization": "Bearer " + tok.Token},
		})
	}
	return out
}

// connect is the in-chat /connect: run the connector OAuth flow, then note that
// it goes live next session (MCP servers are started once, at session start).
func (s *session) connect(args []string) {
	if cmdConnect(args, s.out, s.out) == 0 && len(args) > 0 {
		fmt.Fprintln(s.out, "  "+style.Gray("· it activates in your next session (MCP servers start at launch)"))
	}
}

// cmdConnect runs the OAuth device flow for a connector and stores the token.
func cmdConnect(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		renderConnectors(out)
		fmt.Fprintln(out, "\n  connect one with: cliche connect <name>")
		return 0
	}
	name := args[0]
	c, ok := knownConnectors[name]
	if !ok {
		fmt.Fprintf(errOut, "connect: unknown connector %q (see `cliche connectors`)\n", name)
		return 1
	}
	clientID := connectorClientID(c)
	if clientID == "" {
		fmt.Fprintf(errOut, "connect: %s needs a one-time OAuth app. Create one at\n  %s\nthen export %s and re-run.\n", name, c.register, c.clientEnv)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cfg := oauth.DeviceConfig{ClientID: clientID, Scopes: c.scopes, DeviceURL: c.deviceURL, TokenURL: c.tokenURL}
	dc, err := oauth.RequestCode(ctx, cfg)
	if err != nil {
		fmt.Fprintln(errOut, "connect: "+err.Error())
		return 1
	}

	uri := dc.VerificationURI
	if uri == "" {
		uri = "https://github.com/login/device"
	}
	fmt.Fprintf(out, "\n  %s connect %s\n", style.BoldWhite(gl("⇄", "<>")), style.White(name))
	fmt.Fprintf(out, "  1. open  %s\n", style.Color(uri, style.Sample(0)))
	fmt.Fprintf(out, "  2. enter %s\n", style.BoldGreen(dc.UserCode))
	if openBrowser(uri) {
		fmt.Fprintln(out, "  "+style.Gray("(opened your browser…)"))
	}
	fmt.Fprintln(out, "  "+style.Gray("waiting for you to authorize · Ctrl-C to cancel"))

	tok, err := oauth.PollToken(ctx, cfg, dc)
	if err != nil {
		fmt.Fprintln(errOut, "connect: "+err.Error())
		return 1
	}
	if err := secrets.SaveConnector(name, secrets.ConnectorToken{Token: tok.AccessToken, Type: tok.TokenType}); err != nil {
		fmt.Fprintln(errOut, "connect: saving token: "+err.Error())
		return 1
	}
	fmt.Fprintf(out, "\n  %s connected %s %s\n", style.Green(gl("✓", "ok")), style.White(name), style.Gray("· available as an MCP connector in every chat"))
	return 0
}

// cmdConnectors lists connectors and which are connected; `connectors rm <name>`
// disconnects one.
func cmdConnectors(args []string, out, errOut io.Writer) int {
	if len(args) >= 2 && (args[0] == "rm" || args[0] == "remove" || args[0] == "disconnect") {
		if err := secrets.DeleteConnector(args[1]); err != nil {
			fmt.Fprintln(errOut, "connectors: "+err.Error())
			return 1
		}
		fmt.Fprintf(out, "  %s disconnected %s\n", style.Gray("−"), style.White(args[1]))
		return 0
	}
	renderConnectors(out)
	return 0
}

func renderConnectors(out io.Writer) {
	connected := map[string]bool{}
	for _, n := range secrets.ConnectedNames() {
		connected[n] = true
	}
	names := make([]string, 0, len(knownConnectors))
	for n := range knownConnectors {
		names = append(names, n)
	}
	sort.Strings(names)

	fmt.Fprintf(out, "\n  %s %s\n", style.BoldWhite("connectors"), style.Gray("· OAuth-gated MCP servers (device-flow login)"))
	for _, n := range names {
		c := knownConnectors[n]
		badge := style.Gray("○ not connected")
		if connected[n] {
			badge = style.Green(gl("✓", "*") + " connected")
		} else if connectorClientID(c) == "" {
			badge = style.Gray("○ set " + c.clientEnv)
		}
		fmt.Fprintf(out, "  %s %s  %s\n", style.White(style.Pad(n, 10)), badge, style.Gray(c.desc))
	}
}

// openBrowser best-effort opens url in the default browser; returns whether it
// launched something (never blocks, never errors out to the user).
func openBrowser(url string) bool {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd, args = "open", []string{url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	return exec.Command(cmd, args...).Start() == nil
}
