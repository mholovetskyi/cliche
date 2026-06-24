package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/style"
)

// A plugin is an installable bundle at .cliche/plugins/<name>/ that packages the
// extension primitives Cliche already has: skills (skills/<name>/SKILL.md),
// custom commands (commands/<name>.md), pre-tool/stop hooks, and MCP tool-servers
// — declared in a plugin.json manifest. Bundles need no new runtime: their
// contributions are merged into the same skills, commands, MCP, and hook
// machinery (governed by the same caps, governor, permissions, and ledger). This
// is the brand-aligned alternative to native/WASM plugins, which can't satisfy
// zero-dependency + Windows + single-static-binary.

// pluginManifest is the JSON shape of plugin.json. Skills and commands are
// auto-discovered from the bundle's subdirs (convention); hooks and MCP servers
// are declared (they can't be inferred). MCPServer/Hooks are reused verbatim from
// config, so a plugin's entries have the exact same shape as the project config.
type pluginManifest struct {
	Name        string             `json:"name"`
	Version     string             `json:"version"`
	Description string             `json:"description"`
	Hooks       config.Hooks       `json:"hooks"`
	MCP         []config.MCPServer `json:"mcp"`
}

type plugin struct {
	Name    string
	Version string
	Desc    string
	Dir     string
	Hooks   config.Hooks
	MCP     []config.MCPServer
	skills  int // counts, for the /plugins listing
	cmds    int
}

func pluginsDir(root string) string { return filepath.Join(config.Dir(root), "plugins") }

// loadPlugins discovers .cliche/plugins/<name>/plugin.json bundles, sorted by
// name. A directory without a (valid) manifest is silently skipped.
func loadPlugins(root string) []plugin {
	entries, err := os.ReadDir(pluginsDir(root))
	if err != nil {
		return nil
	}
	var out []plugin
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pdir := filepath.Join(pluginsDir(root), e.Name())
		data, err := os.ReadFile(filepath.Join(pdir, "plugin.json"))
		if err != nil {
			continue
		}
		var m pluginManifest
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		name := m.Name
		if name == "" {
			name = e.Name()
		}
		out = append(out, plugin{
			Name: name, Version: m.Version, Desc: m.Description, Dir: pdir,
			Hooks: m.Hooks, MCP: m.MCP,
			skills: countSubdirs(filepath.Join(pdir, "skills")),
			cmds:   countMarkdown(filepath.Join(pdir, "commands")),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// pluginMCP collects every plugin's declared MCP servers, to append to the
// project's own MCP list before launching them.
func pluginMCP(root string) []config.MCPServer {
	var out []config.MCPServer
	for _, p := range loadPlugins(root) {
		out = append(out, p.MCP...)
	}
	return out
}

// pluginHookCommands collects every plugin's pre-tool ("pre") or stop ("stop")
// hook command, to compose with the project's own.
func pluginHookCommands(root, kind string) []string {
	var out []string
	for _, p := range loadPlugins(root) {
		cmd := p.Hooks.PreToolUse
		switch kind {
		case "post":
			cmd = p.Hooks.PostToolUse
		case "stop":
			cmd = p.Hooks.Stop
		}
		if strings.TrimSpace(cmd) != "" {
			out = append(out, cmd)
		}
	}
	return out
}

func countSubdirs(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n
}

func countMarkdown(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			n++
		}
	}
	return n
}

// pluginSummary describes a plugin's contributions in one line.
func pluginSummary(p plugin) string {
	var parts []string
	if p.skills > 0 {
		parts = append(parts, fmt.Sprintf("%d skill(s)", p.skills))
	}
	if p.cmds > 0 {
		parts = append(parts, fmt.Sprintf("%d cmd(s)", p.cmds))
	}
	if len(p.MCP) > 0 {
		parts = append(parts, fmt.Sprintf("%d mcp", len(p.MCP)))
	}
	if p.Hooks.PreToolUse != "" || p.Hooks.Stop != "" {
		parts = append(parts, "hooks")
	}
	if len(parts) == 0 {
		return "(empty)"
	}
	return strings.Join(parts, " · ")
}

func renderPlugins(out io.Writer, root string) {
	plugins := loadPlugins(root)
	if len(plugins) == 0 {
		fmt.Fprintln(out, "  no plugins installed")
		fmt.Fprintln(out, "  "+style.Gray("create one: `cliche plugins new <name>` → a bundle of skills + commands + hooks + MCP at .cliche/plugins/<name>/"))
		return
	}
	fmt.Fprintln(out, "\n  "+style.BoldWhite("plugins")+style.Gray("  ·  .cliche/plugins/<name>/  ·  governed by the same caps/governor/permissions"))
	for _, p := range plugins {
		v := p.Version
		if v != "" {
			v = " " + style.Gray("v"+v)
		}
		fmt.Fprintf(out, "  %s %s%s  %s\n",
			style.Green(gl("✓", "ok")), style.White(p.Name), v, style.Gray(pluginSummary(p)))
		if p.Desc != "" {
			fmt.Fprintf(out, "      %s\n", style.Gray(p.Desc))
		}
	}
}

// showPlugins (/plugins) lists installed plugins in-session.
func (s *session) showPlugins() { renderPlugins(s.out, s.dir) }

// cmdPlugins is `cliche plugins [new <name>]`: list, or scaffold a bundle.
func cmdPlugins(args []string, out, errOut io.Writer) int {
	if len(args) >= 1 && args[0] == "new" {
		if len(args) < 2 {
			fmt.Fprintln(errOut, "usage: cliche plugins new <name>")
			return 2
		}
		name := args[1]
		base := filepath.Join(pluginsDir("."), name)
		if _, err := os.Stat(filepath.Join(base, "plugin.json")); err == nil {
			fmt.Fprintln(errOut, "plugins: "+name+" already exists")
			return 1
		}
		// A starter bundle: a manifest, one sample skill, and one sample command.
		writes := map[string]string{
			filepath.Join(base, "plugin.json"):                   pluginManifestTemplate(name),
			filepath.Join(base, "skills", "example", "SKILL.md"): skillTemplate("example"),
			filepath.Join(base, "commands", "example.md"):        commandTemplate("example"),
		}
		for path, content := range writes {
			if _, err := scaffold(path, content); err != nil {
				fmt.Fprintln(errOut, "plugins: "+err.Error())
				return 1
			}
		}
		fmt.Fprintln(out, "  created "+base+"/")
		fmt.Fprintln(out, "  "+style.Gray("a plugin.json + a sample skill and command — edit, then `cliche plugins` to verify"))
		return 0
	}
	renderPlugins(out, ".")
	return 0
}

func pluginManifestTemplate(name string) string {
	return fmt.Sprintf(`{
  "name": %q,
  "version": "0.1.0",
  "description": "what this plugin adds",
  "hooks": {},
  "mcp": []
}
`, name)
}
