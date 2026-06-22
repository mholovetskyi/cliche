package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildMapsGoSymbolsAndSkipsNoise(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "internal", "agent", "agent.go"), `package agent
type Agent struct{}
type Config struct{}
func New() *Agent { return nil }
func (a *Agent) Run() error { return nil }
`)
	write(t, filepath.Join(root, "main.go"), "package main\nfunc main() {}\n")
	write(t, filepath.Join(root, "readme.txt"), "not source")           // skipped ext
	write(t, filepath.Join(root, "node_modules", "lib", "x.js"), "var") // skipped dir

	m, err := Build(root, 8000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m, "internal/agent/") || !strings.Contains(m, "agent.go") {
		t.Fatalf("map should include the agent dir and file:\n%s", m)
	}
	if !strings.Contains(m, "type Agent, Config") {
		t.Fatalf("map should list Go types:\n%s", m)
	}
	if !strings.Contains(m, "Agent.Run") || !strings.Contains(m, "New") {
		t.Fatalf("map should list funcs incl. methods:\n%s", m)
	}
	if strings.Contains(m, "node_modules") || strings.Contains(m, "readme.txt") {
		t.Fatalf("map should skip node_modules and non-source files:\n%s", m)
	}
}

func TestBuildRespectsByteBudget(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		write(t, filepath.Join(root, "pkg", "f"+itoa(i)+".go"), "package pkg\nfunc F() {}\n")
	}
	m, err := Build(root, 300)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) > 360 { // small slack for the truncation note
		t.Fatalf("map should respect the byte budget, got %d bytes", len(m))
	}
	if !strings.Contains(m, "truncated") {
		t.Fatalf("an over-budget map should be marked truncated:\n%s", m)
	}
}
