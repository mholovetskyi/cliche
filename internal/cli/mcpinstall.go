package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// knownMCPServers is the catalogue of supported `cliche mcp install <name>` targets.
var knownMCPServers = map[string]mcpInstaller{
	"github": installGitHub,
}

type mcpInstaller func(binDir string, out, errOut io.Writer) (config.MCPServer, error)

// cmdMcpInstall is `cliche mcp install <name>`.
// It builds/downloads the named MCP server binary into .cliche/bin/, resolves
// authentication, and upserts the server entry in .cliche/config.json — so new
// users never have to touch Docker, npm, or a JSON file by hand.
func cmdMcpInstall(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("mcp install", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(errOut, "usage: cliche mcp install <name> [--dir <path>]")
		fmt.Fprintln(errOut, "available: "+strings.Join(mcpInstallNames(), ", "))
		return 2
	}
	name := strings.ToLower(fs.Arg(0))
	installer, ok := knownMCPServers[name]
	if !ok {
		fmt.Fprintf(errOut, "mcp install: unknown server %q\n", name)
		fmt.Fprintln(errOut, "available: "+strings.Join(mcpInstallNames(), ", "))
		return 2
	}

	root := *dir
	binDir := filepath.Join(config.Dir(root), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		fmt.Fprintln(errOut, "mcp install: "+err.Error())
		return 1
	}

	fmt.Fprintf(out, "\n  %s installing %s MCP server\n", style.Gray(gl("→", ">")), style.White(name))

	srv, err := installer(binDir, out, errOut)
	if err != nil {
		fmt.Fprintln(errOut, "mcp install: "+err.Error())
		return 1
	}

	if err := upsertMCPServer(root, srv, out, errOut); err != nil {
		fmt.Fprintln(errOut, "mcp install: "+err.Error())
		return 1
	}

	fmt.Fprintf(out, "\n  %s %s ready  %s\n",
		style.Red(gl("✔", "+")),
		style.White(name),
		style.Gray("restart cliche to connect"),
	)
	return 0
}

// ── GitHub installer ──────────────────────────────────────────────────────────

// installGitHub builds github.com/github/github-mcp-server from source using
// the Go toolchain (already required by Cliche itself), stores the binary in
// binDir, and resolves the token via `gh auth token` or the saved credentials.
// No Docker, no npm, no manual token copying.
func installGitHub(binDir string, out, errOut io.Writer) (config.MCPServer, error) {
	// 1. Resolve a GitHub PAT — gh CLI first, then saved cliche credentials.
	token, err := resolveGitHubToken(out)
	if err != nil {
		return config.MCPServer{}, fmt.Errorf("could not find a GitHub token: %w\n"+
			"       run `gh auth login` or `cliche auth github --key <token>`", err)
	}

	// 2. Build the binary.
	binPath, err := buildGitHubMCPServer(binDir, out, errOut)
	if err != nil {
		return config.MCPServer{}, err
	}

	return config.MCPServer{
		Name:    "github",
		Command: binPath,
		Args:    []string{"stdio"},
		Env:     []string{"GITHUB_PERSONAL_ACCESS_TOKEN=" + token},
	}, nil
}

// resolveGitHubToken tries, in order:
//  1. GITHUB_PERSONAL_ACCESS_TOKEN env var
//  2. `gh auth token` (GitHub CLI — already authenticated)
//  3. Cliche's own saved credentials for "github"
func resolveGitHubToken(out io.Writer) (string, error) {
	if t := strings.TrimSpace(os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")); t != "" {
		fmt.Fprintln(out, "  "+style.Gray("token: using GITHUB_PERSONAL_ACCESS_TOKEN env var"))
		return t, nil
	}
	if tok, err := ghAuthToken(); err == nil && tok != "" {
		fmt.Fprintln(out, "  "+style.Gray("token: using `gh auth token`"))
		return tok, nil
	}
	if tok, _ := secrets.Lookup("github"); tok != "" {
		fmt.Fprintln(out, "  "+style.Gray("token: using saved cliche credentials"))
		return tok, nil
	}
	return "", fmt.Errorf("no token found")
}

// ghAuthToken runs `gh auth token` and returns its output.
func ghAuthToken() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// buildGitHubMCPServer clones and builds github/github-mcp-server, returning
// the path to the produced binary. If the binary already exists and is fresh
// (non-zero size) it is reused.
func buildGitHubMCPServer(binDir string, out, errOut io.Writer) (string, error) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	binPath := filepath.Join(binDir, "github-mcp-server"+ext)

	// Reuse existing binary.
	if fi, err := os.Stat(binPath); err == nil && fi.Size() > 0 {
		fmt.Fprintln(out, "  "+style.Gray("binary: already built, reusing "+binPath))
		return binPath, nil
	}

	// Need Go.
	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("go toolchain not found; install Go from https://go.dev/dl/ and retry")
	}

	// Clone into a temp dir.
	tmpDir, err := os.MkdirTemp("", "github-mcp-server-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintln(out, "  "+style.Gray("cloning github/github-mcp-server …"))
	clone := exec.Command("git", "clone", "--depth=1",
		"https://github.com/github/github-mcp-server.git", tmpDir)
	clone.Stdout = io.Discard
	clone.Stderr = errOut
	if err := clone.Run(); err != nil {
		return "", fmt.Errorf("git clone failed: %w", err)
	}

	fmt.Fprintln(out, "  "+style.Gray("building binary (this takes ~30 s the first time) …"))
	build := exec.Command("go", "build", "-o", binPath, "./cmd/github-mcp-server")
	build.Dir = tmpDir
	build.Stdout = io.Discard
	build.Stderr = errOut
	if err := build.Run(); err != nil {
		return "", fmt.Errorf("go build failed: %w", err)
	}

	fmt.Fprintln(out, "  "+style.Gray("built → "+binPath))
	return binPath, nil
}

// ── Config upsert ─────────────────────────────────────────────────────────────

// upsertMCPServer reads .cliche/config.json (creating it from defaults if
// absent), replaces or appends the MCP server entry with the same name, and
// writes it back atomically.
func upsertMCPServer(root string, srv config.MCPServer, out, errOut io.Writer) error {
	cfgPath := filepath.Join(config.Dir(root), "config.json")

	// Read existing config as raw JSON so we preserve any fields we don't know about.
	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(data, &raw)
	}

	// Decode the mcp array (may be absent).
	var servers []config.MCPServer
	if v, ok := raw["mcp"]; ok {
		_ = json.Unmarshal(v, &servers)
	}

	// Replace existing entry with same name, or append.
	replaced := false
	for i, s := range servers {
		if s.Name == srv.Name {
			servers[i] = srv
			replaced = true
			break
		}
	}
	if !replaced {
		servers = append(servers, srv)
	}

	encoded, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	raw["mcp"] = encoded

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(config.Dir(root), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, append(data, '\n'), 0o644); err != nil {
		return err
	}

	verb := "added"
	if replaced {
		verb = "updated"
	}
	fmt.Fprintf(out, "  %s %s in .cliche/config.json\n", style.Gray(gl("•", "-")), style.Gray(verb+" \""+srv.Name+"\" MCP server"))
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mcpInstallNames() []string {
	names := make([]string, 0, len(knownMCPServers))
	for k := range knownMCPServers {
		names = append(names, k)
	}
	return names
}
