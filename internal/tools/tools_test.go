package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestRelativePathResolvesToRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "in.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := OSExecutor{Root: root, Policy: Policy{AllowWrite: true}}
	// A RELATIVE path must resolve under the root, not the process cwd.
	if r := e.Execute(context.Background(), "read_file", map[string]string{"file": "in.txt"}); !r.Success || r.Output != "hi" {
		t.Fatalf("relative read should resolve under root: success=%v out=%q", r.Success, r.Output)
	}
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": "out.txt", "content": "x"}); !r.Success {
		t.Fatalf("relative write should resolve under root: %s", r.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "out.txt")); err != nil {
		t.Fatalf("file should be written inside root: %v", err)
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

func TestWriteFileCreatesParentDirs(t *testing.T) {
	root := t.TempDir()
	e := OSExecutor{Root: root, Policy: Policy{Yolo: true}}
	// A path into folders that don't exist yet must succeed (scaffolding).
	r := e.Execute(context.Background(), "write_file", map[string]string{
		"file": "pkg/sub/deep/file.txt", "content": "hi",
	})
	if !r.Success {
		t.Fatalf("write into a new folder tree should succeed: %s", r.Output)
	}
	got, err := os.ReadFile(filepath.Join(root, "pkg", "sub", "deep", "file.txt"))
	if err != nil || string(got) != "hi" {
		t.Fatalf("file not created in new dirs: %v %q", err, got)
	}
}

type spyApprover struct {
	calls      int
	allow      bool
	lastAction string
	lastDetail string
}

func (s *spyApprover) approve(action, detail string) bool {
	s.calls++
	s.lastAction, s.lastDetail = action, detail
	return s.allow
}

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

// TestApprovalShowsDiffPreview verifies the executor hands the approver a change
// preview (so the user sees what they're authorizing) for both write and edit,
// and that no preview work leaks when the action is pre-authorized.
func TestApprovalShowsDiffPreview(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "x.txt")
	if err := os.WriteFile(f, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// edit_file: the approval detail names the file and shows the changed lines.
	spy := &spyApprover{allow: true}
	e := OSExecutor{Root: root, Approve: spy.approve}
	r := e.Execute(context.Background(), "edit_file", map[string]string{
		"file": f, "old_string": "two", "new_string": "TWO",
	})
	if !r.Success {
		t.Fatalf("edit should apply: %s", r.Output)
	}
	if !strings.Contains(spy.lastDetail, "x.txt") || !strings.Contains(spy.lastDetail, "- two") || !strings.Contains(spy.lastDetail, "+ TWO") {
		t.Fatalf("edit approval detail should carry a diff preview, got:\n%s", spy.lastDetail)
	}

	// write_file over an existing file previews the overwrite too.
	spy2 := &spyApprover{allow: true}
	e2 := OSExecutor{Root: root, Approve: spy2.approve}
	if r := e2.Execute(context.Background(), "write_file", map[string]string{"file": f, "content": "brand new\n"}); !r.Success {
		t.Fatalf("write should apply: %s", r.Output)
	}
	if !strings.Contains(spy2.lastDetail, "+ brand new") {
		t.Fatalf("write approval detail should carry a diff preview, got:\n%s", spy2.lastDetail)
	}

	// Pre-authorized (yolo): the approver is never consulted, so no preview.
	spy3 := &spyApprover{allow: false}
	e3 := OSExecutor{Root: root, Policy: Policy{Yolo: true}, Approve: spy3.approve}
	if r := e3.Execute(context.Background(), "write_file", map[string]string{"file": f, "content": "z\n"}); !r.Success {
		t.Fatalf("yolo write should apply: %s", r.Output)
	}
	if spy3.calls != 0 {
		t.Fatalf("pre-authorized write must not consult the approver, got %d calls", spy3.calls)
	}
}
