package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/mholovetskyi/cliche/internal/style"
)

// approver implements interactive y/N/always permission prompts, reading from
// a shared bufio.Reader so it coexists with an interactive session's prompt
// loop (single-threaded). "always" sticks for the rest of the process. The
// mutex serializes prompts when parallel subagents request approval at once.
type approver struct {
	mu          sync.Mutex
	r           *bufio.Reader
	out         io.Writer
	alwaysWrite bool
	alwaysRun   bool
	mode        string // permission mode (mutable via /mode); "" == suggest
}

// setMode changes the permission mode (mutex-guarded; Approve reads it under
// the same lock).
func (a *approver) setMode(m string) {
	a.mu.Lock()
	a.mode = m
	a.mu.Unlock()
}

// Approve is passed to tools.OSExecutor as its Approver. The mode short-circuits
// the prompt: plan denies, full allows, auto-edit auto-allows writes.
func (a *approver) Approve(action, detail string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch a.mode {
	case modePlan:
		fmt.Fprintf(a.out, "  %s plan mode is read-only — %s blocked\n", gl("■", "x"), action)
		return false
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
	}
	// detail's first line names the action target; any following lines are a
	// change preview (a diff). Render the preview as its own indented block so
	// the y/N/a question reads cleanly on its own line.
	head, preview, hasPreview := strings.Cut(detail, "\n")
	fmt.Fprintf(a.out, "  %s allow %s: %s\n", gl("⚠", "!"), action, head)
	if hasPreview {
		fmt.Fprintln(a.out, colorizeDiff(preview))
	}
	fmt.Fprint(a.out, "    [y/N/a=always] ")
	line, err := a.r.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "a", "always":
		if action == "write" {
			a.alwaysWrite = true
		} else {
			a.alwaysRun = true
		}
		return true
	default:
		return false
	}
}

// colorizeDiff tints a change-preview block with the brand palette: removed
// lines red, the summary/elision lines gray, added lines left as primary text.
func colorizeDiff(preview string) string {
	lines := strings.Split(preview, "\n")
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, "-"):
			lines[i] = style.Red(ln) // removed
		case strings.HasPrefix(t, "+"):
			// added — leave as primary text
		default:
			lines[i] = style.Gray(ln) // summary / elision note
		}
	}
	return strings.Join(lines, "\n")
}
