package cli

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/mholovetskyi/cliche/internal/style"
)

// approver implements interactive y/N/always permission prompts, reading from
// a shared bufio.Reader so it coexists with an interactive session's prompt
// loop (single-threaded). "always" sticks for the rest of the process. The
// mutex serializes prompts when parallel subagents request approval at once.
type approver struct {
	mu            sync.Mutex
	r             *bufio.Reader
	out           io.Writer
	onPromptStart func() // called right before an interactive prompt is drawn (pause spinners)
	onPromptEnd   func() // called once the prompt read returns (resume spinners)
	// choose, if set, renders an arrow-key choice row (approve/reject/always) and
	// returns the chosen index; handled=false means it couldn't run (no raw mode)
	// so Approve falls back to the typed y/N read. idx 0=approve, 2=always, else deny.
	choose      func(choices []string) (idx int, handled bool)
	alwaysWrite bool
	alwaysRun   bool
	alwaysWeb   bool
	mode        string // permission mode (mutable via /mode); "" == suggest
}

// setMode changes the permission mode (mutex-guarded; Approve reads it under
// the same lock).
func (a *approver) setMode(m string) {
	a.mu.Lock()
	a.mode = m
	a.mu.Unlock()
}

// AlwaysFlags reports which "always allow" grants are active (write/run/web).
// These are as consequential as the permission mode but otherwise invisible, so
// /status surfaces them. Mutex-guarded to match Approve's writes.
func (a *approver) AlwaysFlags() (write, run, web bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.alwaysWrite, a.alwaysRun, a.alwaysWeb
}

// Approve is passed to tools.OSExecutor as its Approver. The mode short-circuits
// the prompt: plan denies, full allows, auto-edit auto-allows writes.
func (a *approver) Approve(action, detail string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch a.mode {
	case modePlan:
		// Read-only: block mutations and commands, but allow read-only fetches.
		if action == "write" || action == "run" {
			fmt.Fprintf(a.out, "  %s plan mode is read-only — %s blocked\n", gl("■", "x"), action)
			return false
		}
	case modeFull:
		return true
	case modeAutoEdit:
		if action == "write" {
			return true
		}
	}
	switch action {
	case "write":
		if a.alwaysWrite {
			return true
		}
	case "run":
		if a.alwaysRun {
			return true
		}
	case "fetch":
		if a.alwaysWeb {
			return true
		}
	}
	// We're about to block on input. Pause spinners for the whole prompt —
	// otherwise the active spinner's ticking frames ("writing foo… 2971s"), or a
	// concurrent subagent's events restarting a spinner, overwrite the prompt, so
	// the user can't see that a y/N is required and the session appears hung.
	if a.onPromptStart != nil {
		a.onPromptStart()
	}
	if a.onPromptEnd != nil {
		defer a.onPromptEnd()
	}
	// detail's first line names the action target; any following lines are a
	// change preview (a diff, already colored at generation). Frame it as a
	// permission card: a header, the body, then a scoped choice row.
	head, preview, hasPreview := strings.Cut(detail, "\n")
	verb, target := approvalHeader(action, head)
	fmt.Fprintf(a.out, "\n  %s %s %s\n", style.BoldRed(gl("⚠", "!")), style.BoldRed(verb), style.White(target))
	if hasPreview {
		fmt.Fprintln(a.out, preview)
	}
	if reason := riskyReason(action, head); reason != "" {
		fmt.Fprintf(a.out, "  %s\n", style.Red(gl("⚠", "!")+" "+reason))
	}
	// Arrow-key choice card (interactive raw-mode session): approve · reject ·
	// always, navigable with ←/→ and Enter (y/n/a still work). Reading through the
	// raw decoder also avoids the cooked-ReadString hazards (Ctrl-C is a key event;
	// no type-ahead stranded in a second buffer). Falls back to the typed read
	// below when raw mode is unavailable.
	if a.choose != nil {
		if idx, handled := a.choose(choiceLabels(action)); handled {
			switch idx {
			case 0:
				return true
			case 2:
				switch action {
				case "write":
					a.alwaysWrite = true
				case "fetch":
					a.alwaysWeb = true
				default:
					a.alwaysRun = true
				}
				return true
			default:
				return false
			}
		}
	}
	fmt.Fprint(a.out, "  "+style.Dim(choiceRow(action))+" ")
	line, err := a.r.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "a", "always":
		switch action {
		case "write":
			a.alwaysWrite = true
		case "fetch":
			a.alwaysWeb = true
		default:
			a.alwaysRun = true
		}
		return true
	default:
		return false
	}
}

// approvalHeader turns the raw action + detail head into a card title (the verb)
// and a target (the file / command / url).
func approvalHeader(action, head string) (verb, target string) {
	switch action {
	case "write":
		fields := strings.Fields(head)
		verb = "EDIT"
		target = head
		if len(fields) >= 2 {
			if strings.HasPrefix(fields[0], "write") {
				verb = "WRITE"
			}
			target = fields[len(fields)-1]
		}
		return verb, target
	case "run":
		return "RUN", head
	case "fetch":
		return "FETCH", head
	default:
		return strings.ToUpper(action), head
	}
}

// choiceLabels are the arrow-card options per action: index 0 approves (verb
// varies), 1 rejects, 2 grants "always". The verb mirrors choiceRow.
func choiceLabels(action string) []string {
	yes := "approve"
	switch action {
	case "run":
		yes = "run"
	case "fetch":
		yes = "fetch"
	}
	return []string{yes, "reject", "always"}
}

// choiceRow spells out the y/N/a choices with the scope of "always" per action,
// so the user knows exactly what an 'a' grants.
func choiceRow(action string) string {
	switch action {
	case "write":
		return "(y) approve · (N) reject · (a) always allow edits"
	case "run":
		return "(y) run · (N) reject · (a) always allow commands"
	case "fetch":
		return "(y) fetch · (N) reject · (a) always allow fetches"
	default:
		return "(y) yes · (N) no · (a) always"
	}
}

// riskyPatterns flag shell commands whose blast radius warrants a second look.
// Ordered most-specific-first: the first match wins, so "wget … | sudo bash"
// is flagged as a pipe-to-shell (the salient risk) rather than just "sudo".
var riskyPatterns = []struct {
	re  *regexp.Regexp
	why string
}{
	{regexp.MustCompile(`(curl|wget)[^|]*\|\s*(sudo\s+)?(sh|bash|zsh)`), "pipes a download straight into a shell"},
	{regexp.MustCompile(`:\s*\(\s*\)\s*\{.*\|.*&\s*\}`), "fork bomb"},
	{regexp.MustCompile(`\bmkfs\b`), "formats a filesystem (mkfs)"},
	{regexp.MustCompile(`\bdd\b[^\n]*\bof=`), "raw disk write (dd of=)"},
	{regexp.MustCompile(`>\s*/dev/sd`), "writes to a raw disk device"},
	{regexp.MustCompile(`rm\s+-[a-z]*r[a-z]*f|rm\s+-[a-z]*f[a-z]*r`), "deletes files recursively (rm -rf)"},
	{regexp.MustCompile(`chmod\s+-?R?\s*0?777`), "makes files world-writable (chmod 777)"},
	{regexp.MustCompile(`\bgit\b[^\n]*\bpush\b[^\n]*(--force|-f)\b`), "force-pushes (rewrites remote history)"},
	{regexp.MustCompile(`\bsudo\b`), "runs with elevated privileges (sudo)"},
}

// riskyReason returns a caution string when a run command matches a dangerous
// pattern (empty otherwise, and for non-run actions).
func riskyReason(action, target string) string {
	if action != "run" {
		return ""
	}
	for _, p := range riskyPatterns {
		if p.re.MatchString(target) {
			return p.why
		}
	}
	return ""
}
