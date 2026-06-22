package verifier

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// TestResult is the outcome of an independent test re-run.
type TestResult struct {
	Ran     bool   `json:"ran"`
	Passed  bool   `json:"passed"`
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
	Err     string `json:"err,omitempty"`
}

// RunTests executes command in dir via the platform shell and reports whether
// it passed (exit code 0). The provided context bounds the run; the caller is
// responsible for setting a timeout (and, for model-driven verification, a
// budget).
func RunTests(ctx context.Context, dir, command string) TestResult {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	tr := TestResult{Ran: true, Command: command, Passed: err == nil, Output: string(out)}
	if err != nil {
		tr.Err = err.Error()
	}
	return tr
}

// DiscoverTestCommand resolves the test command for a project: an AGENTS.md
// "## verify" / "test:" line wins, otherwise it auto-detects from project
// marker files. Returns the command and whether one was found.
func DiscoverTestCommand(dir string) (string, bool) {
	if cmd, ok := testCmdFromAgents(dir); ok {
		return cmd, true
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	switch {
	case exists("go.mod"):
		return "go test ./...", true
	case exists("Cargo.toml"):
		return "cargo test", true
	case exists("package.json"):
		return "npm test", true
	case exists("pyproject.toml"), exists("setup.py"), exists("pytest.ini"):
		return "pytest -q", true
	}
	return "", false
}

// testCmdFromAgents reads AGENTS.md and returns a "test:" command found under a
// "## verify" heading (the planned verify-rules extension to AGENTS.md).
func testCmdFromAgents(dir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		return "", false
	}
	inVerify := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			inVerify = strings.Contains(strings.ToLower(line), "verify")
			continue
		}
		if !inVerify {
			continue
		}
		low := strings.ToLower(line)
		if idx := strings.Index(low, "test:"); idx >= 0 {
			cmd := strings.TrimSpace(line[idx+len("test:"):])
			cmd = strings.TrimSpace(strings.Trim(cmd, "`"))
			if cmd != "" {
				return cmd, true
			}
		}
	}
	return "", false
}
