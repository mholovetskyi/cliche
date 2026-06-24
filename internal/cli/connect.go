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
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/mcp"
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
	// directToken, if set, tries to obtain a token WITHOUT any OAuth dance — e.g.
	// from an already-authenticated CLI (`gh`) or an env var. This is the seamless
	// path: when it succeeds, no client id / browser / device code is needed.
	directToken func(out io.Writer) (string, bool)
}

var knownConnectors = map[string]knownConnector{
	"github": {
		name:        "github",
		desc:        "GitHub MCP — repos, issues, pull requests",
		mcpURL:      "https://api.githubcopilot.com/mcp/",
		deviceURL:   "https://github.com/login/device/code",
		tokenURL:    "https://github.com/login/oauth/access_token",
		scopes:      []string{"repo", "read:org", "read:user"},
		clientEnv:   "CLICHE_GITHUB_CLIENT_ID",
		register:    "github.com/settings/applications/new (OAuth app; enable Device Flow)",
		directToken: ghDirectToken,
	},
}

// ghDirectToken is GitHub's seamless path: reuse a token from the `gh` CLI, an
// env var, or saved cliche creds — so `connect github` Just Works for the many
// developers who already have the GitHub CLI authenticated (no OAuth app needed).
func ghDirectToken(out io.Writer) (string, bool) {
	t, err := resolveGitHubToken(out)
	return t, err == nil && t != ""
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

// connect is the in-chat /connect: run the connector OAuth flow, then HOT-ATTACH
// the new connector to the live session so its tools are usable immediately — no
// restart. Falls back to "next session" if the live attach fails.
func (s *session) connect(args []string) {
	if cmdConnect(args, s.out, s.out) != 0 || len(args) == 0 {
		return
	}
	name := args[0]
	c, ok := knownConnectors[name]
	if !ok {
		return
	}
	tok, ok := secrets.Connector(name)
	if !ok || tok.Token == "" {
		return
	}
	conn := mcp.StartHTTPWithHeaders(name, c.mcpURL, map[string]string{"Authorization": "Bearer " + tok.Token})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	if ad, ok := s.a.MCP().(*mcpAdapter); ok && ad != nil {
		err = ad.attach(ctx, conn) // merge into the live manager
	} else {
		// No MCP attached yet this session — stand one up with just this connector.
		mgr, merr := mcp.NewManager(ctx, []mcp.Conn{conn})
		if merr != nil {
			err = merr
		} else {
			s.a.SetMCP(&mcpAdapter{mgr: mgr, allow: s.mcpAllow, approve: s.app.Approve})
		}
	}
	if err != nil {
		fmt.Fprintf(s.out, "  %s connected, but couldn't attach live (%s)\n", style.Gray("·"), err.Error())
		fmt.Fprintln(s.out, "  "+style.Gray("it'll load in your next session"))
		return
	}
	fmt.Fprintf(s.out, "  %s %s is live now %s\n", style.Green(gl("✓", "ok")), style.White(name), style.Gray("· its tools are available this session"))
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
	// Seamless path first: if we can reuse an existing token (e.g. the `gh` CLI),
	// connect with zero setup — no OAuth app, no browser, no device code.
	if c.directToken != nil {
		if tok, ok := c.directToken(out); ok {
			if err := secrets.SaveConnector(name, secrets.ConnectorToken{Token: tok, Type: "bearer"}); err != nil {
				fmt.Fprintln(errOut, "connect: "+err.Error())
				return 1
			}
			fmt.Fprintf(out, "  %s connected %s %s\n", style.Green(gl("✓", "ok")), style.White(name), style.Gray("· available in every chat"))
			return 0
		}
	}

	clientID := connectorClientID(c)
	if clientID == "" {
		fmt.Fprintf(errOut, "connect: no %s token found.\n", name)
		if c.directToken != nil {
			fmt.Fprintf(errOut, "  easiest: run `gh auth login`, then `cliche connect %s` again.\n", name)
		}
		fmt.Fprintf(errOut, "  or set up a one-time OAuth app at %s,\n  then export %s and re-run.\n", c.register, c.clientEnv)
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
