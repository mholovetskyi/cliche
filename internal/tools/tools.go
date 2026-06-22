// Package tools executes the agent's tool calls behind a permission gate and a
// project-root confinement boundary.
//
// v0 ships two executors:
//
//   - OSExecutor: real file/command tools, gated by a permission Policy and
//     confined to a project Root (no reading/writing outside it by default).
//   - SimExecutor: deterministic, side-effect-free outcomes for the demo and
//     tests.
//
// The permission model is graduated: read is confined but otherwise allowed;
// writes and shell commands are denied by default unless explicitly allowed (or
// --yolo, or interactively approved). Note that --yolo bypasses APPROVALS only
// — it never bypasses the Budget Kernel or the Governor. That is the brand.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholovetskyi/cliche/internal/shell"
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

// Policy controls what the OSExecutor is allowed to do without asking.
type Policy struct {
	AllowWrite       bool
	AllowRun         bool
	Yolo             bool
	AllowOutsideRoot bool // permit file access outside the project root (escape hatch)
}

// Approver is consulted when a Policy does not pre-authorize an action. It
// returns true to allow. Used for interactive y/N/always prompts. A nil
// Approver means "deny if not pre-authorized".
type Approver func(action, detail string) bool

// OSExecutor performs real file and command operations under a Policy, confined
// to Root, asking the Approver for anything the Policy doesn't already allow.
type OSExecutor struct {
	Root    string // project root; "" disables confinement
	Policy  Policy
	Approve Approver
}

// permit decides whether an action ("write" or "run") may proceed. --yolo and
// the explicit allow flags pre-authorize; otherwise the Approver is asked.
func (e OSExecutor) permit(action, detail string) bool {
	if e.Policy.Yolo {
		return true
	}
	switch action {
	case "write":
		if e.Policy.AllowWrite {
			return true
		}
	case "run":
		if e.Policy.AllowRun {
			return true
		}
	}
	if e.Approve != nil {
		return e.Approve(action, detail)
	}
	return false
}

// resolve confines a path to the project root unless confinement is disabled.
// It returns the absolute path to use.
func (e OSExecutor) resolve(path string) (string, error) {
	if e.Root == "" || e.Policy.AllowOutsideRoot {
		return path, nil
	}
	root, err := filepath.Abs(e.Root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the project root (use --allow-outside-root to permit)", path)
	}
	return abs, nil
}

// Execute runs a tool call against the real filesystem/shell.
func (e OSExecutor) Execute(ctx context.Context, name string, args map[string]string) Result {
	edit := isEditTool(name)
	switch name {
	case "read_file":
		if strings.TrimSpace(args["file"]) == "" {
			return Result{Output: "read error: no file specified", Success: false}
		}
		p, err := e.resolve(args["file"])
		if err != nil {
			return Result{Output: "read denied: " + err.Error(), Success: false}
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return Result{Output: "read error: " + err.Error(), Success: false}
		}
		return Result{Output: string(data), Success: true}

	case "write_file":
		if strings.TrimSpace(args["file"]) == "" {
			return Result{Output: "write error: no file specified", IsEdit: edit, Success: false}
		}
		p, err := e.resolve(args["file"])
		if err != nil {
			return Result{Output: "write denied: " + err.Error(), IsEdit: edit, Success: false}
		}
		if !e.permit("write", "write_file "+args["file"]) {
			return Result{Output: "permission denied: write to " + args["file"], IsEdit: edit, Success: false}
		}
		if err := validateSyntax(p, args["content"]); err != nil {
			return Result{Output: "write rejected: " + err.Error(), IsEdit: edit, Success: false}
		}
		if err := os.WriteFile(p, []byte(args["content"]), 0o644); err != nil {
			return Result{Output: "write error: " + err.Error(), IsEdit: edit, Success: false}
		}
		return Result{Output: "wrote " + args["file"], IsEdit: edit, Success: true}

	case "edit_file":
		if strings.TrimSpace(args["file"]) == "" {
			return Result{Output: "edit error: no file specified", IsEdit: edit, Success: false}
		}
		p, err := e.resolve(args["file"])
		if err != nil {
			return Result{Output: "edit denied: " + err.Error(), IsEdit: edit, Success: false}
		}
		if !e.permit("write", "edit_file "+args["file"]) {
			return Result{Output: "permission denied: edit " + args["file"], IsEdit: edit, Success: false}
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return Result{Output: "edit error: " + err.Error(), IsEdit: edit, Success: false}
		}
		updated, err := applyEdit(string(data), args["old_string"], args["new_string"], args["replace_all"] == "true")
		if err != nil {
			return Result{Output: "edit error: " + err.Error(), IsEdit: edit, Success: false}
		}
		if err := validateSyntax(p, updated); err != nil {
			return Result{Output: "edit rejected (file left unchanged): " + err.Error(), IsEdit: edit, Success: false}
		}
		if err := os.WriteFile(p, []byte(updated), 0o644); err != nil {
			return Result{Output: "edit error: " + err.Error(), IsEdit: edit, Success: false}
		}
		return Result{Output: "edited " + args["file"], IsEdit: edit, Success: true}

	case "run_command":
		if strings.TrimSpace(args["command"]) == "" {
			return Result{Output: "run error: empty command", Success: false}
		}
		if !e.permit("run", args["command"]) {
			return Result{Output: "permission denied: run command", Success: false}
		}
		out, err := shell.Command(ctx, e.Root, args["command"]).CombinedOutput()
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
