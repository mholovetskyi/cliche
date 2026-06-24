package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestParseFrontmatter(t *testing.T) {
	meta, body := parseFrontmatter("---\nname: foo\ndescription: \"a thing\"\n---\nthe body\nline two")
	if meta["name"] != "foo" || meta["description"] != "a thing" {
		t.Fatalf("frontmatter = %v", meta)
	}
	if body != "the body\nline two" {
		t.Fatalf("body = %q", body)
	}
	// No header → whole content is the body.
	if _, b := parseFrontmatter("no header here"); b != "no header here" {
		t.Fatalf("no-header body = %q", b)
	}
	// Unterminated header → treated as body, not parsed.
	if m, _ := parseFrontmatter("---\nname: x\nstill going"); len(m) != 0 {
		t.Fatalf("unterminated header should not parse: %v", m)
	}
}

func TestLoadAndExpandCommands(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(commandsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ndescription: review changes\n---\nReview $ARGUMENTS for bugs and missing tests."
	os.WriteFile(filepath.Join(commandsDir(dir), "review.md"), []byte(body), 0o644)

	cmds := loadCommands(dir)
	c, ok := cmds["/review"]
	if !ok || c.Desc != "review changes" {
		t.Fatalf("loadCommands = %+v", cmds)
	}
	if got := c.expand([]string{"the", "auth", "handler"}); got != "Review the auth handler for bugs and missing tests." {
		t.Fatalf("$ARGUMENTS expand = %q", got)
	}
	pos := userCommand{Body: "Compare $1 with $2"}
	if got := pos.expand([]string{"a", "b"}); got != "Compare a with b" {
		t.Fatalf("positional expand = %q", got)
	}
}

func TestLoadSkillsAndSystemNote(t *testing.T) {
	dir := t.TempDir()
	skDir := filepath.Join(skillsDir(dir), "pr-review")
	if err := os.MkdirAll(skDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(skDir, "SKILL.md"),
		[]byte("---\nname: pr-review\ndescription: review a pull request\n---\n# steps\nDo X then Y."), 0o644)

	skills := loadSkills(dir)
	if len(skills) != 1 || skills[0].Name != "pr-review" || skills[0].Desc != "review a pull request" {
		t.Fatalf("loadSkills = %+v", skills)
	}
	if !strings.Contains(skills[0].Body, "Do X then Y") {
		t.Fatalf("skill body = %q", skills[0].Body)
	}
	note := skillsSystemNote(dir)
	for _, want := range []string{"pr-review", "review a pull request", "SKILL.md"} {
		if !strings.Contains(note, want) {
			t.Fatalf("system note missing %q:\n%s", want, note)
		}
	}
	if skillsSystemNote(t.TempDir()) != "" {
		t.Fatal("no skills should yield an empty system note")
	}
}

func TestInvokeSkill(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	s := &session{out: &bytes.Buffer{}, skills: map[string]skill{"x": {Name: "x", Body: "do the thing"}}}
	prompt, run := s.invokeSkill([]string{"x", "with", "input"})
	if !run || !strings.Contains(prompt, "do the thing") || !strings.Contains(prompt, "with input") {
		t.Fatalf("invokeSkill = %q, %v", prompt, run)
	}

	var out bytes.Buffer
	s2 := &session{out: &out, skills: map[string]skill{}}
	if _, run := s2.invokeSkill([]string{"nope"}); run {
		t.Fatal("missing skill should not run")
	}
	if !strings.Contains(out.String(), "no skill") {
		t.Fatalf("should report a missing skill:\n%s", out.String())
	}
}

func TestBugReportAndIssueURL(t *testing.T) {
	dir := t.TempDir()
	path, err := writeBugReport(dir, "it crashed on /status", "openrouter", "gpt-4o-mini", "suggest", "20200101-000000")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	body := string(data)
	for _, want := range []string{"it crashed on /status", "openrouter", "gpt-4o-mini", "os/arch", "No secrets"} {
		if !strings.Contains(body, want) {
			t.Errorf("bug report missing %q:\n%s", want, body)
		}
	}
	if u := issueURL("crash on status"); !strings.HasPrefix(u, issuesURL+"?") || !strings.Contains(u, "title=") {
		t.Fatalf("issue URL = %q", u)
	}
}
