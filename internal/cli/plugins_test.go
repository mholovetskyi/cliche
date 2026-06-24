package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginBundleAggregation(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(pluginsDir(dir), "pr-bot")
	if err := os.MkdirAll(filepath.Join(base, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(base, "commands"), 0o755)
	os.WriteFile(filepath.Join(base, "plugin.json"),
		[]byte(`{"name":"pr-bot","version":"1.2.0","description":"PR helpers","hooks":{"pre_tool_use":"echo hi"},"mcp":[{"name":"gh","command":"gh-mcp"}]}`), 0o644)
	os.WriteFile(filepath.Join(base, "skills", "review", "SKILL.md"),
		[]byte("---\nname: pr-review\ndescription: review PRs\n---\nDo it."), 0o644)
	os.WriteFile(filepath.Join(base, "commands", "shipit.md"),
		[]byte("---\ndescription: ship it\n---\nShip $ARGUMENTS"), 0o644)

	plugins := loadPlugins(dir)
	if len(plugins) != 1 || plugins[0].Name != "pr-bot" || plugins[0].Version != "1.2.0" {
		t.Fatalf("loadPlugins = %+v", plugins)
	}
	if plugins[0].skills != 1 || plugins[0].cmds != 1 || len(plugins[0].MCP) != 1 {
		t.Fatalf("counts: skills=%d cmds=%d mcp=%d", plugins[0].skills, plugins[0].cmds, len(plugins[0].MCP))
	}

	// The plugin's skill is merged into the skill set and the agent system note.
	foundSkill := false
	for _, s := range loadSkills(dir) {
		if s.Name == "pr-review" && strings.Contains(s.Rel, "plugins/pr-bot") {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Fatal("plugin skill not aggregated into loadSkills")
	}
	if !strings.Contains(skillsSystemNote(dir), "pr-review") {
		t.Fatal("plugin skill missing from the agent system note")
	}

	// The plugin's command is merged into the custom-command set.
	if _, ok := loadCommands(dir)["/shipit"]; !ok {
		t.Fatalf("plugin command not aggregated: %v", loadCommands(dir))
	}

	// MCP + hook contributions are collected.
	if mcp := pluginMCP(dir); len(mcp) != 1 || mcp[0].Name != "gh" {
		t.Fatalf("pluginMCP = %+v", mcp)
	}
	if h := pluginHookCommands(dir, "pre"); len(h) != 1 || h[0] != "echo hi" {
		t.Fatalf("pluginHookCommands(pre) = %v", h)
	}
	if h := pluginHookCommands(dir, "stop"); len(h) != 0 {
		t.Fatalf("pluginHookCommands(stop) = %v, want none", h)
	}

	var out bytes.Buffer
	renderPlugins(&out, dir)
	for _, want := range []string{"pr-bot", "1.2.0", "skill", "cmd", "mcp", "hooks", "PR helpers"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("renderPlugins missing %q:\n%s", want, out.String())
		}
	}
}

func TestPluginScaffolder(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	var out, errOut bytes.Buffer
	if code := cmdPlugins([]string{"new", "demo"}, &out, &errOut); code != 0 {
		t.Fatalf("plugins new exit = %d, err=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(pluginsDir("."), "demo", "plugin.json")); err != nil {
		t.Fatalf("manifest not created: %v", err)
	}
	ps := loadPlugins(".")
	if len(ps) != 1 || ps[0].Name != "demo" || ps[0].skills != 1 || ps[0].cmds != 1 {
		t.Fatalf("scaffolded bundle not loadable as expected: %+v", ps)
	}
	// Refuses to clobber an existing plugin.
	if code := cmdPlugins([]string{"new", "demo"}, &out, &errOut); code == 0 {
		t.Fatal("scaffolding over an existing plugin should fail")
	}
}

func TestPreToolHookChainNilWhenEmpty(t *testing.T) {
	if buildPreToolHookChain(".", []string{"", "  "}) != nil {
		t.Fatal("an all-empty hook chain should be nil (no hook)")
	}
	if buildPreToolHookChain(".", []string{"echo hi"}) == nil {
		t.Fatal("a non-empty hook chain should build a hook")
	}
}
