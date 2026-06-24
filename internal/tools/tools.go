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
	"strconv"
	"strings"

	"github.com/mholovetskyi/cliche/internal/shell"
)

// defaultReadLines bounds a read_file with no explicit limit, so reading a huge
// file can't dump megabytes into the next model request (a budget protection,
// not just an ergonomic one). The model can page the rest with offset/limit.
const defaultReadLines = 2000

// runOutputLimit bounds the bytes of a command's combined output fed back to the
// model. A noisy build or an accidental `cat` of a large file must not be able
// to flood the context window (and the token budget) in a single turn.
const runOutputLimit = 60_000

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
	ReadOnly         bool // plan mode: hard-deny all mutations/commands (overrides everything)
	AllowWeb         bool // pre-authorize web_fetch network egress
	Sandbox          bool // strict posture: force confinement, deny network by default, scrub run env
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
	Journal *EditJournal // optional; records mutations for /diff and /undo (nil = off)
	Rules   Rules        // optional allow/deny rules evaluated before the Policy/approver
	Egress  Egress       // optional host allowlist for web_fetch (empty = unrestricted)

	// PreToolHook, if set, runs before every tool as the outermost gate.
	// Returning allow=false blocks the call; reason (the hook's output) is shown.
	PreToolHook func(name string, args map[string]string) (allow bool, reason string)
	// PostToolHook, if set, runs after every tool completes (observe-only): it
	// gets the tool name, args, and whether it succeeded. It cannot block.
	PostToolHook func(name string, args map[string]string, ok bool)
}

// preauthorized reports whether an action proceeds without prompting (--yolo or
// the matching allow flag). It mirrors the fast path of permit, so callers can
// skip building an expensive approval detail (e.g. a diff) that won't be shown.
func (e OSExecutor) preauthorized(action string) bool {
	if e.Policy.Yolo {
		return true
	}
	switch action {
	case "write":
		return e.Policy.AllowWrite
	case "run":
		return e.Policy.AllowRun
	}
	return false
}

// permit decides whether an action may proceed. category is the rule category
// ("write"/"edit"/"run"); action is the coarse approver label ("write"/"run");
// target is the file path or command for rule matching. A matching ALLOW rule
// pre-authorizes; otherwise --yolo / the allow flags pre-authorize; otherwise
// the Approver is asked. (DENY rules are handled before this, in Execute.)
func (e OSExecutor) permit(category, target, action, detail string) bool {
	if e.ruleDecision(category, target) == ruleAllow {
		return true
	}
	if e.preauthorized(action) {
		return true
	}
	if e.Approve != nil {
		return e.Approve(action, detail)
	}
	return false
}

// ruleDecision evaluates the allow/deny rules (none when no rules configured).
func (e OSExecutor) ruleDecision(category, target string) ruleAction {
	if e.Rules.Empty() {
		return ruleNone
	}
	return e.Rules.Decision(category, target)
}

// resolve confines a path to the project root unless confinement is disabled.
// It resolves symlinks on the root and on the longest existing prefix of the
// target, so an in-root symlink that points outside the root is also rejected.
// It returns the absolute path to use for the operation.
// ResolveWithin resolves path against root and confirms it does not escape root
// (following symlinks), returning the absolute path. It is the standalone form
// of the executor's confinement check, so callers outside the executor — like
// the chat @file include — reuse the exact path-safety rules instead of
// re-implementing them. An empty root disables confinement.
func ResolveWithin(root, path string) (string, error) {
	return OSExecutor{Root: root}.resolve(path)
}

func (e OSExecutor) resolve(path string) (string, error) {
	// Sandbox forces confinement: --allow-outside-root is ignored in sandbox mode.
	if e.Root == "" || (e.Policy.AllowOutsideRoot && !e.Policy.Sandbox) {
		return path, nil
	}
	root, err := filepath.Abs(e.Root)
	if err != nil {
		return "", err
	}
	if rp, err := filepath.EvalSymlinks(root); err == nil {
		root = rp
	}
	// Resolve RELATIVE paths against the project root (not the process cwd), so
	// file tools and run_command (which runs with cwd=root) agree on "where".
	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Join(root, path)
	}
	// Check the symlink-resolved real location, but operate on abs (the OS
	// follows the same symlinks). This catches symlink escapes.
	rel, err := filepath.Rel(root, resolveExisting(abs))
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the project root (use --allow-outside-root to permit)", path)
	}
	return abs, nil
}

// resolveExisting resolves symlinks on the longest existing ancestor of p and
// re-appends the non-existent tail (so a not-yet-created file still resolves
// through real, symlink-followed parent directories).
func resolveExisting(p string) string {
	cur := p
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, strings.TrimPrefix(p, cur))
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p // nothing along the path exists
		}
		cur = parent
	}
}

// Execute runs a tool call against the real filesystem/shell, then fires the
// observe-only PostToolUse hook (if set).
func (e OSExecutor) Execute(ctx context.Context, name string, args map[string]string) Result {
	res := e.execute(ctx, name, args)
	if e.PostToolHook != nil {
		e.PostToolHook(name, args, res.Success)
	}
	return res
}

// execute is the core tool dispatch (Execute wraps it with the PostToolUse hook).
func (e OSExecutor) execute(ctx context.Context, name string, args map[string]string) Result {
	edit := isEditTool(name)
	// PreToolHook is the outermost, programmable gate: an operator-supplied
	// command decides (via exit code) whether any tool call may proceed, before
	// rules/permission/confinement. It runs deterministically on every call.
	if e.PreToolHook != nil {
		if allow, reason := e.PreToolHook(name, args); !allow {
			msg := "blocked by pre-tool-use hook"
			if reason != "" {
				msg += ": " + reason
			}
			return Result{Output: msg, IsEdit: edit, Success: false}
		}
	}
	// Plan mode is a hard read-only boundary: mutations and commands are blocked
	// outright (even under --yolo), so the agent proposes instead of acting.
	if e.Policy.ReadOnly && (edit || name == "run_command") {
		return Result{Output: "blocked: plan mode is read-only — describe the change, don't apply it", IsEdit: edit, Success: false}
	}
	switch name {
	case "read_file":
		if strings.TrimSpace(args["file"]) == "" {
			return Result{Output: "read error: no file specified", Success: false}
		}
		if e.ruleDecision("read", args["file"]) == ruleDeny {
			return Result{Output: "blocked by deny rule: read " + args["file"], Success: false}
		}
		p, err := e.resolve(args["file"])
		if err != nil {
			return Result{Output: "read denied: " + err.Error(), Success: false}
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return Result{Output: "read error: " + err.Error(), Success: false}
		}
		return Result{Output: readView(string(data), args["offset"], args["limit"]), Success: true}

	case "search_files":
		return e.searchFiles(args)

	case "find_files":
		return e.findFiles(args)

	case "list_files":
		return e.listFiles(args)

	case "web_fetch":
		return e.webFetch(ctx, args)

	case "write_file":
		if strings.TrimSpace(args["file"]) == "" {
			return Result{Output: "write error: no file specified", IsEdit: edit, Success: false}
		}
		if e.ruleDecision("write", args["file"]) == ruleDeny {
			return Result{Output: "blocked by deny rule: write " + args["file"], IsEdit: edit, Success: false}
		}
		p, err := e.resolve(args["file"])
		if err != nil {
			return Result{Output: "write denied: " + err.Error(), IsEdit: edit, Success: false}
		}
		// Read the prior content if we'll show a preview or journal the change.
		var oldData []byte
		var oldExisted bool
		if !e.preauthorized("write") || e.Journal != nil {
			b, err := os.ReadFile(p)
			oldData, oldExisted = b, err == nil
		}
		detail := "write_file " + args["file"]
		if !e.preauthorized("write") {
			detail += "\n  " + changePreview(string(oldData), args["content"]) // missing file → previewed as new
		}
		if !e.permit("write", args["file"], "write", detail) {
			return Result{Output: "permission denied: write to " + args["file"], IsEdit: edit, Success: false}
		}
		if err := validateSyntax(p, args["content"]); err != nil {
			return Result{Output: "write rejected: " + err.Error(), IsEdit: edit, Success: false}
		}
		// Create parent directories so the agent can scaffold new folders (like
		// Claude Code). The dir is inside the confined root (p resolved above).
		if dir := filepath.Dir(p); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return Result{Output: "write error: " + err.Error(), IsEdit: edit, Success: false}
			}
		}
		if err := os.WriteFile(p, []byte(args["content"]), 0o644); err != nil {
			return Result{Output: "write error: " + err.Error(), IsEdit: edit, Success: false}
		}
		e.Journal.record(p, string(oldData), oldExisted)
		return Result{Output: "wrote " + args["file"], IsEdit: edit, Success: true}

	case "edit_file":
		if strings.TrimSpace(args["file"]) == "" {
			return Result{Output: "edit error: no file specified", IsEdit: edit, Success: false}
		}
		if e.ruleDecision("edit", args["file"]) == ruleDeny {
			return Result{Output: "blocked by deny rule: edit " + args["file"], IsEdit: edit, Success: false}
		}
		p, err := e.resolve(args["file"])
		if err != nil {
			return Result{Output: "edit denied: " + err.Error(), IsEdit: edit, Success: false}
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return Result{Output: "edit error: " + err.Error(), IsEdit: edit, Success: false}
		}
		updated, err := applyEdit(string(data), args["old_string"], args["new_string"], args["replace_all"] == "true")
		if err != nil {
			return Result{Output: "edit error: " + err.Error(), IsEdit: edit, Success: false}
		}
		detail := "edit_file " + args["file"]
		if !e.preauthorized("write") {
			detail += "\n  " + changePreview(string(data), updated)
		}
		if !e.permit("edit", args["file"], "write", detail) {
			return Result{Output: "permission denied: edit " + args["file"], IsEdit: edit, Success: false}
		}
		if err := validateSyntax(p, updated); err != nil {
			return Result{Output: "edit rejected (file left unchanged): " + err.Error(), IsEdit: edit, Success: false}
		}
		if err := guardCollateralDeletion(p, string(data), updated, args["old_string"]); err != nil {
			return Result{Output: "edit rejected (file left unchanged): " + err.Error(), IsEdit: edit, Success: false}
		}
		if err := os.WriteFile(p, []byte(updated), 0o644); err != nil {
			return Result{Output: "edit error: " + err.Error(), IsEdit: edit, Success: false}
		}
		e.Journal.record(p, string(data), true)
		return Result{Output: "edited " + args["file"], IsEdit: edit, Success: true}

	case "run_command":
		if strings.TrimSpace(args["command"]) == "" {
			return Result{Output: "run error: empty command", Success: false}
		}
		if e.ruleDecision("run", args["command"]) == ruleDeny {
			cmd := args["command"]
			if len(cmd) > 80 {
				cmd = cmd[:80] + "…"
			}
			return Result{Output: "blocked by deny rule: " + cmd, Success: false}
		}
		if !e.permit("run", args["command"], "run", args["command"]) {
			return Result{Output: "permission denied: run command", Success: false}
		}
		cmd := shell.Command(ctx, e.Root, args["command"])
		if e.Policy.Sandbox {
			cmd.Env = scrubbedEnv() // don't leak the operator's provider keys to model commands
		}
		out, err := cmd.CombinedOutput()
		return Result{Output: boundOutput(string(out), runOutputLimit), Success: err == nil}

	default:
		return Result{Output: "unknown tool: " + name, IsEdit: edit, Success: false}
	}
}

// boundOutput caps a command's output to limit bytes by keeping the head and
// tail (errors usually surface at the end) with a note in the middle, so the
// model still sees how a long run started and finished without the whole flood.
func boundOutput(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	head := limit * 2 / 3
	tail := limit - head
	return s[:head] +
		fmt.Sprintf("\n\n… [%d bytes truncated by cliche to protect the budget] …\n\n", len(s)-limit) +
		s[len(s)-tail:]
}

// readView returns a line-bounded view of file content for read_file. With no
// offset/limit it returns the whole file, unless the file exceeds
// defaultReadLines — then it returns the head and a note pointing at
// offset/limit, so a huge file can't blow the token budget in one read. Any
// partial view is annotated so the model knows it isn't seeing the whole file.
// A full read round-trips the bytes exactly (including a trailing newline).
func readView(content, offsetArg, limitArg string) string {
	if content == "" {
		return ""
	}
	trailingNL := strings.HasSuffix(content, "\n")
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	total := len(lines)

	start := 0
	if v, err := strconv.Atoi(strings.TrimSpace(offsetArg)); err == nil && v > 1 {
		start = v - 1
	}
	if start > total {
		start = total
	}
	limit, explicit := defaultReadLines, false
	if v, err := strconv.Atoi(strings.TrimSpace(limitArg)); err == nil && v > 0 {
		limit, explicit = v, true
	}
	end := start + limit
	if end > total {
		end = total
	}

	view := strings.Join(lines[start:end], "\n")
	if end == total && trailingNL {
		view += "\n"
	}
	if start == 0 && end == total {
		return view // whole file
	}
	note := fmt.Sprintf("\n\n[read_file: showing lines %d-%d of %d", start+1, end, total)
	if end < total && !explicit {
		note += "; file is large — pass offset/limit to read the rest"
	} else if end < total {
		note += "; pass offset to continue"
	}
	return view + note + "]"
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
