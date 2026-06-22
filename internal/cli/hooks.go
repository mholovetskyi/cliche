package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/shell"
	"github.com/mholovetskyi/cliche/internal/style"
)

// hookEnv builds the process environment for a hook: the parent environment
// plus the tool context as CLICHE_* variables, so a hook script can branch on
// what's being attempted without parsing anything.
func hookEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// runHook executes command in dir with the given extra env, returning its exit
// code and trimmed combined output. A missing shell / spawn failure is reported
// as a non-zero exit so a configured hook fails closed rather than silently
// passing.
func runHook(ctx context.Context, dir, command string, extra map[string]string) (exit int, out string) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := shell.Command(ctx, dir, command)
	cmd.Env = hookEnv(extra)
	raw, err := cmd.CombinedOutput()
	out = strings.TrimSpace(string(raw))
	if err == nil {
		return 0, out
	}
	if ee, ok := err.(interface{ ExitCode() int }); ok {
		code := ee.ExitCode()
		if code == 0 {
			code = 1
		}
		return code, out
	}
	return 1, out // could not even run the hook → fail closed
}

// buildPreToolHook returns a PreToolHook that runs the configured command before
// each tool call; a non-zero exit blocks the call, with the hook's output as the
// reason. nil when no hook is configured.
func buildPreToolHook(dir, command string) func(name string, args map[string]string) (bool, string) {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	return func(name string, args map[string]string) (bool, string) {
		exit, out := runHook(context.Background(), dir, command, map[string]string{
			"CLICHE_TOOL":         name,
			"CLICHE_TOOL_FILE":    args["file"],
			"CLICHE_TOOL_COMMAND": args["command"],
			"CLICHE_TOOL_URL":     args["url"],
		})
		if exit == 0 {
			return true, ""
		}
		return false, out
	}
}

// runStopHook fires the Stop hook (if configured) when the agent halts.
func runStopHook(out io.Writer, dir, command, reason, verdict string) {
	if strings.TrimSpace(command) == "" {
		return
	}
	_, hookOut := runHook(context.Background(), dir, command, map[string]string{
		"CLICHE_STOP_REASON": reason,
		"CLICHE_VERDICT":     verdict,
	})
	if hookOut != "" {
		fmt.Fprintln(out, "  "+style.Gray("stop hook: "+firstLine(hookOut)))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
