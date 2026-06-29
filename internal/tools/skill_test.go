package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeSkillName(t *testing.T) {
	cases := map[string]string{
		"scaffold-vite-app": "scaffold-vite-app",
		"My Skill!":         "my-skill",
		"../../etc/passwd":  "etc-passwd", // traversal can't survive
		"  spaced  out  ":   "spaced-out",
		"a/b\\c.d":          "a-bc-d",
		"!!!":               "",
		"UPPER_case":        "upper-case",
	}
	for in, want := range cases {
		if got := sanitizeSkillName(in); got != want {
			t.Errorf("sanitizeSkillName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteSkillRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := writeSkill(root, "make-release", "when cutting a release", "1. bump version\n2. tag\n3. push"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".cliche", "skills", "make-release", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"name: make-release", "description: when cutting a release", "1. bump version"} {
		if !strings.Contains(s, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, s)
		}
	}
}

// The save_skill tool writes when approved, and is blocked in plan mode.
func TestSaveSkillTool(t *testing.T) {
	root := t.TempDir()
	args := map[string]string{"name": "deploy-flow", "description": "how to ship this app", "content": "run make deploy"}

	// Plan mode (read-only) blocks it.
	ro := OSExecutor{Root: root, Policy: Policy{ReadOnly: true}}
	if res := ro.Execute(context.Background(), "save_skill", args); res.Success {
		t.Fatal("save_skill should be blocked in plan mode")
	}
	if _, err := os.Stat(filepath.Join(root, ".cliche", "skills", "deploy-flow")); err == nil {
		t.Fatal("plan mode must not write a skill")
	}

	// Pre-authorized write → saved.
	ex := OSExecutor{Root: root, Policy: Policy{AllowWrite: true}}
	res := ex.Execute(context.Background(), "save_skill", args)
	if !res.Success {
		t.Fatalf("save_skill failed: %s", res.Output)
	}
	if _, err := os.Stat(filepath.Join(root, ".cliche", "skills", "deploy-flow", "SKILL.md")); err != nil {
		t.Fatalf("skill not written: %v", err)
	}

	// Missing fields → graceful failure.
	if res := ex.Execute(context.Background(), "save_skill", map[string]string{"name": "x"}); res.Success {
		t.Fatal("save_skill should fail without description/content")
	}
}
