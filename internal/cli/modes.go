package cli

import "github.com/mholovetskyi/cliche/internal/tools"

// Permission modes are the graduated trust ladder (Claude Code-style), layered
// over the same Policy the kernel already enforces — they are presets, not a new
// mechanism. --yolo never bypasses the budget cap or governor in any mode.
const (
	modePlan     = "plan"      // read-only: investigate + propose; mutations/commands hard-blocked
	modeSuggest  = "suggest"   // default: ask before each write/command
	modeAutoEdit = "auto-edit" // auto-apply edits; still ask before commands
	modeFull     = "full"      // auto-approve everything (like --yolo)
)

var modeNames = []string{modePlan, modeSuggest, modeAutoEdit, modeFull}

// nextMode returns the next mode in the ladder (wrapping), for Shift-Tab cycling.
func nextMode(m string) string {
	for i, name := range modeNames {
		if name == m {
			return modeNames[(i+1)%len(modeNames)]
		}
	}
	return modeSuggest
}

// validMode reports whether m is a known mode ("" means "unset / default").
func validMode(m string) bool {
	switch m {
	case "", modePlan, modeSuggest, modeAutoEdit, modeFull:
		return true
	}
	return false
}

// applyMode folds a mode preset into a base Policy (built from the legacy
// --allow-* / --yolo flags). Modes only ADD authority, except plan which
// hard-denies all mutations.
func applyMode(p tools.Policy, mode string) tools.Policy {
	switch mode {
	case modePlan:
		p.ReadOnly = true
	case modeAutoEdit:
		p.AllowWrite = true
	case modeFull:
		p.Yolo = true
	}
	return p
}

// modeSystemNote returns a system-prompt addendum for modes that change how the
// agent should behave (plan mode tells it to propose, not act).
func modeSystemNote(mode string) string {
	if mode == modePlan {
		return " You are in PLAN MODE: investigate the codebase and produce a concrete, step-by-step plan. Do NOT edit files or run commands — they are blocked. Read and search freely, then present the plan and stop."
	}
	return ""
}
