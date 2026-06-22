// Package shell builds the command used to run model-authored and test
// commands. It prefers a POSIX shell (sh) when available — including on
// Windows, where Git ships one — because the model and AGENTS.md author POSIX
// syntax (&&, ||, 2>/dev/null, VAR=x cmd). It falls back to PowerShell with an
// explicit exit-code propagation so a failing native command in a pipeline is
// not silently reported as success.
package shell

import (
	"context"
	"os/exec"
	"runtime"
)

// Command returns an *exec.Cmd that runs command in dir (cwd) using the best
// available shell.
func Command(ctx context.Context, dir, command string) *exec.Cmd {
	var cmd *exec.Cmd
	switch {
	case hasSh():
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	case runtime.GOOS == "windows":
		// Append `; exit $LASTEXITCODE` so the real exit code of the last
		// native command propagates (PowerShell otherwise reports the success
		// of the last cmdlet, e.g. `go test | Out-Null` would exit 0).
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command+"; exit $LASTEXITCODE")
	default:
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

func hasSh() bool {
	_, err := exec.LookPath("sh")
	return err == nil
}
