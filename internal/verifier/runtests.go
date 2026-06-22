package verifier

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholovetskyi/cliche/internal/shell"
)

// TestResult is the outcome of an independent test re-run.
type TestResult struct {
	Ran     bool   `json:"ran"`
	Passed  bool   `json:"passed"`
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
	Err     string `json:"err,omitempty"`
}

// RunTests executes command in dir via the best available shell and reports
// whether it passed (exit code 0). The provided context bounds the run; the
// caller is responsible for setting a timeout.
func RunTests(ctx context.Context, dir, command string) TestResult {
	out, err := shell.Command(ctx, dir, command).CombinedOutput()
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
// "## verify" heading (the planned verify-rules extension to AGENTS.md). The
// heading and the key are matched as whole tokens, not substrings, so "latest:"
// is not mistaken for "test:" and a prose "## verifying X" heading is not a
// verify section.
func testCmdFromAgents(dir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		return "", false
	}
	inVerify := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			heading := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			inVerify = heading == "verify" || strings.HasPrefix(heading, "verify ") || strings.HasPrefix(heading, "verification")
			continue
		}
		if !inVerify {
			continue
		}
		// Strip a leading list marker and backticks, then require the line to
		// START with "test:" (not merely contain it).
		cand := strings.TrimSpace(strings.TrimLeft(line, "-*` \t"))
		if low := strings.ToLower(cand); strings.HasPrefix(low, "test:") {
			cmd := strings.TrimSpace(strings.Trim(cand[len("test:"):], "` "))
			if cmd != "" {
				return cmd, true
			}
		}
	}
	return "", false
}
