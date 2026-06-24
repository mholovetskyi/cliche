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

// buildPreToolHook returns a PreToolHook for a single command (kept for the
// single-hook callers/tests); it delegates to the chain.
func buildPreToolHook(dir, command string) func(name string, args map[string]string) (bool, string) {
	return buildPreToolHookChain(dir, []string{command})
}

// buildPreToolHookChain runs each command before a tool call (the project hook
// plus every plugin's pre-tool hook); the FIRST non-zero exit BLOCKS the call,
// with that hook's output as the reason — fail closed. Empty commands are
// skipped; nil when none remain.
func buildPreToolHookChain(dir string, commands []string) func(name string, args map[string]string) (bool, string) {
	var cmds []string
	for _, c := range commands {
		if strings.TrimSpace(c) != "" {
			cmds = append(cmds, c)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return func(name string, args map[string]string) (bool, string) {
		env := map[string]string{
			"CLICHE_TOOL":         name,
			"CLICHE_TOOL_FILE":    args["file"],
			"CLICHE_TOOL_COMMAND": args["command"],
			"CLICHE_TOOL_URL":     args["url"],
		}
		for _, c := range cmds {
			if exit, out := runHook(context.Background(), dir, c, env); exit != 0 {
				return false, out // a single deny blocks the call
			}
		}
		return true, ""
	}
}

// buildPostToolHook returns a PostToolHook that runs each command AFTER every
// tool call (observe-only — exit code ignored, never blocks). The tool context
// plus CLICHE_TOOL_OK ("true"/"false") is passed via env. nil when none remain.
func buildPostToolHook(dir string, commands []string) func(name string, args map[string]string, ok bool) {
	var cmds []string
	for _, c := range commands {
		if strings.TrimSpace(c) != "" {
			cmds = append(cmds, c)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return func(name string, args map[string]string, ok bool) {
		okStr := "false"
		if ok {
			okStr = "true"
		}
		env := map[string]string{
			"CLICHE_TOOL":         name,
			"CLICHE_TOOL_FILE":    args["file"],
			"CLICHE_TOOL_COMMAND": args["command"],
			"CLICHE_TOOL_URL":     args["url"],
			"CLICHE_TOOL_OK":      okStr,
		}
		for _, c := range cmds {
			runHook(context.Background(), dir, c, env) // observe-only
		}
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
