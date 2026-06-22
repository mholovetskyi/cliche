package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestConfinementBlocksOutsideRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "in.txt")
	if err := os.WriteFile(inside, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := OSExecutor{Root: root, Policy: Policy{AllowWrite: true}}

	// In-root read works.
	if r := e.Execute(context.Background(), "read_file", map[string]string{"file": inside}); !r.Success {
		t.Fatalf("in-root read should succeed: %s", r.Output)
	}
	// Out-of-root read is denied.
	if r := e.Execute(context.Background(), "read_file", map[string]string{"file": filepath.Join(root, "..", "escape.txt")}); r.Success {
		t.Fatal("out-of-root read must be denied")
	}
	// Out-of-root write is denied (and nothing created).
	outside := filepath.Join(filepath.Dir(root), "pwned.txt")
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": outside, "content": "x"}); r.Success {
		t.Fatal("out-of-root write must be denied")
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("out-of-root file must not exist")
	}
}

func TestConfinementBlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	secret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An in-root symlink pointing OUTSIDE the root must not be a read primitive.
	link := filepath.Join(root, "link")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Skipf("cannot create symlinks on this host: %v", err)
	}
	e := OSExecutor{Root: root}
	r := e.Execute(context.Background(), "read_file", map[string]string{"file": filepath.Join(link, "secret.txt")})
	if r.Success {
		t.Fatal("reading through an out-of-root symlink must be denied")
	}
}

func TestAllowOutsideRootEscapeHatch(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "ok.txt")
	if err := os.WriteFile(outside, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := OSExecutor{Root: root, Policy: Policy{AllowOutsideRoot: true}}
	if r := e.Execute(context.Background(), "read_file", map[string]string{"file": outside}); !r.Success {
		t.Fatalf("escape hatch should permit the read: %s", r.Output)
	}
}

type spyApprover struct {
	calls int
	allow bool
}

func (s *spyApprover) approve(action, detail string) bool { s.calls++; return s.allow }

func TestPermissionMatrix(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "x.txt")

	// Default: nil approver -> write denied, run denied.
	def := OSExecutor{Root: root}
	if r := def.Execute(context.Background(), "write_file", map[string]string{"file": f, "content": "a"}); r.Success {
		t.Fatal("default policy must deny writes")
	}
	if r := def.Execute(context.Background(), "run_command", map[string]string{"command": "exit 0"}); r.Success {
		t.Fatal("default policy must deny run_command")
	}

	// Yolo authorizes WITHOUT consulting the approver.
	spy := &spyApprover{allow: false}
	yolo := OSExecutor{Root: root, Policy: Policy{Yolo: true}, Approve: spy.approve}
	if r := yolo.Execute(context.Background(), "write_file", map[string]string{"file": f, "content": "a"}); !r.Success {
		t.Fatalf("yolo should authorize write: %s", r.Output)
	}
	if spy.calls != 0 {
		t.Fatalf("yolo must not call the approver, got %d calls", spy.calls)
	}

	// Approver consulted (and honored) when not pre-authorized.
	deny := &spyApprover{allow: false}
	e := OSExecutor{Root: root, Approve: deny.approve}
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": f, "content": "a"}); r.Success {
		t.Fatal("a denying approver must block the write")
	}
	if deny.calls != 1 {
		t.Fatalf("approver should be consulted once, got %d", deny.calls)
	}
	allow := &spyApprover{allow: true}
	e2 := OSExecutor{Root: root, Approve: allow.approve}
	if r := e2.Execute(context.Background(), "write_file", map[string]string{"file": f, "content": "a"}); !r.Success {
		t.Fatalf("an allowing approver should permit the write: %s", r.Output)
	}
}
