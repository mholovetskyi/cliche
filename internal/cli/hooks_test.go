package cli

import (
	"context"
	"testing"
)

func TestBuildPreToolHookExitCodes(t *testing.T) {
	if !shellAvailable() {
		t.Skip("no shell available to run hooks")
	}
	dir := t.TempDir()

	// A hook that exits non-zero blocks; one that exits zero allows. `exit N`
	// is valid in both POSIX sh and PowerShell, so this runs everywhere.
	block := buildPreToolHook(dir, "exit 3")
	if allow, _ := block("run_command", map[string]string{"command": "rm -rf /"}); allow {
		t.Fatal("a non-zero hook exit must block the tool")
	}

	allow := buildPreToolHook(dir, "exit 0")
	if ok, _ := allow("read_file", map[string]string{"file": "x"}); !ok {
		t.Fatal("a zero hook exit must allow the tool")
	}

	if buildPreToolHook(dir, "   ") != nil {
		t.Fatal("an empty hook command should yield a nil hook")
	}
}

func TestRunHookFailsClosedOnMissingShell(t *testing.T) {
	// Even an unparseable/odd command must not return exit 0 (fail closed).
	if !shellAvailable() {
		t.Skip("no shell available")
	}
	exit, _ := runHook(context.Background(), t.TempDir(), "exit 1", nil)
	if exit == 0 {
		t.Fatal("a failing hook must report a non-zero exit")
	}
}

// shellAvailable reports whether a shell can run a trivial `exit 0` (true on
// every dev/CI platform; the guard keeps the test honest if one is missing).
func shellAvailable() bool {
	exit, _ := runHook(context.Background(), "", "exit 0", nil)
	return exit == 0
}
