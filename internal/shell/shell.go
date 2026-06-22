// Package shell builds the command used to run model-authored and test
// commands. The model writes POSIX shell syntax (&&, ||, 2>/dev/null,
// VAR=x cmd), so we prefer a POSIX shell — including on Windows, where Git for
// Windows ships one even when it isn't on PATH. We fall back to PowerShell 7
// (pwsh, which supports && / ||), then to legacy Windows PowerShell with an
// explicit exit-code propagation. Describe() reports the active shell so the
// agent can tell the model what syntax to use.
package shell

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

// posixShell returns a POSIX sh path: PATH first, then common Git for Windows
// install locations.
func posixShell() (string, bool) {
	if p, err := exec.LookPath("sh"); err == nil {
		return p, true
	}
	if runtime.GOOS == "windows" {
		for _, cand := range []string{
			`C:\Program Files\Git\bin\sh.exe`,
			`C:\Program Files\Git\usr\bin\sh.exe`,
			`C:\Program Files (x86)\Git\bin\sh.exe`,
		} {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				return cand, true
			}
		}
	}
	return "", false
}

// Command returns an *exec.Cmd that runs command in dir (cwd) using the best
// available shell.
func Command(ctx context.Context, dir, command string) *exec.Cmd {
	var cmd *exec.Cmd
	switch {
	case hasPosix():
		sh, _ := posixShell()
		cmd = exec.CommandContext(ctx, sh, "-c", command)
	case hasPwsh():
		// PowerShell 7 supports && / ||; still force the native exit code.
		cmd = exec.CommandContext(ctx, "pwsh", "-NoProfile", "-Command", command+"; exit $LASTEXITCODE")
	case runtime.GOOS == "windows":
		// Legacy Windows PowerShell: append exit so a failing native command in
		// a pipeline propagates its real exit code.
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command+"; exit $LASTEXITCODE")
	default:
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

// Describe reports the active shell and any syntax constraints, for the model.
func Describe() string {
	switch {
	case hasPosix():
		return "POSIX sh (use normal shell syntax: &&, ||, |)"
	case hasPwsh():
		return "PowerShell 7 / pwsh (&& and || work)"
	case runtime.GOOS == "windows":
		return "Windows PowerShell 5.1 — do NOT use && or ||; run one command per call, or separate with ;"
	default:
		return "POSIX sh"
	}
}

func hasPosix() bool { _, ok := posixShell(); return ok }

func hasPwsh() bool { _, err := exec.LookPath("pwsh"); return err == nil }
