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
}

// Approve is passed to tools.OSExecutor as its Approver.
func (a *approver) Approve(action, detail string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
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
