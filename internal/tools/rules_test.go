package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseRulesAndDecision(t *testing.T) {
	r, err := ParseRules(
		[]string{"Bash(go test *)", "Edit(src/**)"},
		[]string{"Read(**/.env)", "Bash(rm -rf *)"},
	)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		cat, target string
		want        ruleAction
	}{
		{"run", "go test ./...", ruleAllow},
		{"run", "rm -rf /", ruleDeny},
		{"run", "git push", ruleNone},
		{"edit", "src/app/main.go", ruleAllow},
		{"edit", "docs/readme.md", ruleNone},
		{"read", ".env", ruleDeny},
		{"read", "config/.env", ruleDeny},
		{"read", "main.go", ruleNone},
	}
	for _, c := range cases {
		if got := r.Decision(c.cat, c.target); got != c.want {
			t.Errorf("Decision(%q,%q) = %d, want %d", c.cat, c.target, got, c.want)
		}
	}
}

func TestParseRulesRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"justtext", "Nope(x)", "Read()", "Bash"} {
		if _, err := ParseRules([]string{bad}, nil); err == nil {
			t.Errorf("ParseRules(%q) should have errored", bad)
		}
	}
}

func TestExecutorEnforcesRules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, _ := ParseRules([]string{"Bash(echo *)"}, []string{"Read(**/.env)", "Write(**/.env)"})
	// Yolo would normally allow everything; deny rules must still win.
	e := OSExecutor{Root: root, Policy: Policy{Yolo: true}, Rules: rules}

	if r := e.Execute(context.Background(), "read_file", map[string]string{"file": ".env"}); r.Success {
		t.Fatal("deny rule must block reading .env even under --yolo")
	}
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": ".env", "content": "x"}); r.Success {
		t.Fatal("deny rule must block writing .env even under --yolo")
	}
	// A non-denied read still works.
	if r := e.Execute(context.Background(), "read_file", map[string]string{"file": "missing.txt"}); r.Success {
		t.Fatal("missing file read should fail (but not via deny)")
	}
}

func TestRuleAllowPreauthorizesWithoutApprover(t *testing.T) {
	root := t.TempDir()
	rules, _ := ParseRules([]string{"Write(out/**)"}, nil)
	deny := &spyApprover{allow: false} // would deny if consulted
	e := OSExecutor{Root: root, Approve: deny.approve, Rules: rules}
	// An allow rule pre-authorizes, so the approver is never consulted.
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": "out/a.txt", "content": "hi"}); !r.Success {
		t.Fatalf("allow rule should pre-authorize the write: %s", r.Output)
	}
	if deny.calls != 0 {
		t.Fatalf("allow rule should skip the approver, got %d calls", deny.calls)
	}
	// A path outside the allow rule still falls through to the (denying) approver.
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": "other.txt", "content": "hi"}); r.Success {
		t.Fatal("a write outside the allow rule should fall through to the denying approver")
	}
}
