// Package tools executes the agent's tool calls behind a permission gate.
//
// v0 ships two executors:
//
//   - OSExecutor: real file/command tools, gated by a permission Policy.
//   - SimExecutor: deterministic, side-effect-free outcomes for the demo and
//     tests.
//
// The permission model is graduated: read is always allowed; writes and shell
// commands are denied by default unless explicitly allowed (or --yolo). Note
// that --yolo bypasses APPROVALS only — it never bypasses the Budget Kernel
// or the Governor. That is the brand.
package tools

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Result is the outcome of executing one tool call.
type Result struct {
	Output  string
	IsEdit  bool // true for file-mutating tools (write/edit/apply_diff)
	Success bool
}

// Executor runs a single tool call by name with string args.
type Executor interface {
	Execute(ctx context.Context, name string, args map[string]string) Result
}

func isEditTool(name string) bool {
	switch name {
	case "write_file", "edit_file", "apply_diff":
		return true
	default:
		return false
	}
}

// Policy controls what the OSExecutor is allowed to do.
type Policy struct {
	AllowWrite bool
	AllowRun   bool
	Yolo       bool
}

// OSExecutor performs real file and command operations under a Policy.
type OSExecutor struct {
	Policy Policy
}

// Execute runs a tool call against the real filesystem/shell.
func (e OSExecutor) Execute(ctx context.Context, name string, args map[string]string) Result {
	edit := isEditTool(name)
	switch name {
	case "read_file":
		if strings.TrimSpace(args["file"]) == "" {
			return Result{Output: "read error: no file specified", Success: false}
		}
		data, err := os.ReadFile(args["file"])
		if err != nil {
			return Result{Output: "read error: " + err.Error(), Success: false}
		}
		return Result{Output: string(data), Success: true}

	case "write_file":
		if !(e.Policy.AllowWrite || e.Policy.Yolo) {
			return Result{Output: "permission denied: writes are not allowed (use --yolo or allow writes)", IsEdit: edit, Success: false}
		}
		if strings.TrimSpace(args["file"]) == "" {
			return Result{Output: "write error: no file specified", IsEdit: edit, Success: false}
		}
		if err := os.WriteFile(args["file"], []byte(args["content"]), 0o644); err != nil {
			return Result{Output: "write error: " + err.Error(), IsEdit: edit, Success: false}
		}
		return Result{Output: "wrote " + args["file"], IsEdit: edit, Success: true}

	case "run_command":
		if !(e.Policy.AllowRun || e.Policy.Yolo) {
			return Result{Output: "permission denied: shell commands are not allowed (use --yolo or allow run)", Success: false}
		}
		if strings.TrimSpace(args["command"]) == "" {
			return Result{Output: "run error: empty command", Success: false}
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", args["command"])
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", args["command"])
		}
		out, err := cmd.CombinedOutput()
		return Result{Output: string(out), Success: err == nil}

	default:
		return Result{Output: "unknown tool: " + name, IsEdit: edit, Success: false}
	}
}

// SimExecutor returns deterministic outcomes without side effects. When
// FailEdits is true, every edit tool reports failure (used to simulate the
// failing-edit loop in the demo).
type SimExecutor struct {
	FailEdits bool
}

// Execute returns a scripted result based on the tool name.
func (s SimExecutor) Execute(_ context.Context, name string, _ map[string]string) Result {
	if isEditTool(name) {
		return Result{Output: "simulated edit", IsEdit: true, Success: !s.FailEdits}
	}
	switch name {
	case "read_file":
		return Result{Output: "simulated file contents", Success: true}
	default:
		return Result{Output: "ok", Success: true}
	}
}
