package cli

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/style"
)

const issuesURL = "https://github.com/mholovetskyi/cliche/issues/new"

// reportBug (/bug [note]) writes a structured local report with environment +
// session context and prints a prefilled GitHub issue link. No telemetry — the
// report is a local file and the user decides whether to share it.
func (s *session) reportBug(line string) {
	note := strings.TrimSpace(strings.TrimPrefix(line, "/bug"))
	path, err := writeBugReport(s.dir, note, s.cfg.Provider, s.a.Model(), s.modeName(), s.id)
	if err != nil {
		fmt.Fprintln(s.out, "  bug: "+err.Error())
		return
	}
	fmt.Fprintf(s.out, "  %s wrote %s\n", style.Green(gl("✓", "ok")), style.White(path))
	u := issueURL(note)
	fmt.Fprintln(s.out, "  "+style.Gray("share it → ")+style.Hyperlink(style.White(u), u))
}

// cmdBug is the `cliche bug [note...]` CLI form.
func cmdBug(args []string, out, errOut io.Writer) int {
	note := strings.TrimSpace(strings.Join(args, " "))
	path, err := writeBugReport(".", note, "", "", "", "")
	if err != nil {
		fmt.Fprintln(errOut, "bug: "+err.Error())
		return 1
	}
	fmt.Fprintln(out, "  wrote "+path)
	u := issueURL(note)
	fmt.Fprintln(out, "  share it → "+style.Hyperlink(u, u))
	return 0
}

func writeBugReport(root, note, provider, model, mode, sessionID string) (string, error) {
	path := filepath.Join(config.Dir(root), "bug-"+time.Now().UTC().Format("20060102-150405")+".md")
	if _, err := scaffold(path, bugReport(note, provider, model, mode, sessionID)); err != nil {
		return "", err
	}
	return path, nil
}

func bugReport(note, provider, model, mode, sessionID string) string {
	var b strings.Builder
	b.WriteString("# Bug report\n\n## What happened\n\n")
	if note != "" {
		b.WriteString(note + "\n\n")
	} else {
		b.WriteString("(describe the issue — steps, expected vs actual)\n\n")
	}
	b.WriteString("## Environment\n\n")
	fmt.Fprintf(&b, "- cliche: %s\n", versionString())
	fmt.Fprintf(&b, "- os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- go: %s\n", runtime.Version())
	if provider != "" {
		fmt.Fprintf(&b, "- provider: %s\n", provider)
	}
	if model != "" {
		fmt.Fprintf(&b, "- model: %s\n", model)
	}
	if mode != "" {
		fmt.Fprintf(&b, "- mode: %s\n", mode)
	}
	if sessionID != "" {
		fmt.Fprintf(&b, "- session: %s\n", sessionID)
	}
	b.WriteString("\n_No secrets or file contents are captured. Review before sharing._\n")
	return b.String()
}

func issueURL(note string) string {
	title := note
	if title == "" {
		title = "bug: "
	}
	if len(title) > 70 {
		title = title[:70]
	}
	q := url.Values{}
	q.Set("title", title)
	q.Set("body", fmt.Sprintf("**Environment:** cliche %s on %s/%s\n\n**What happened:**\n",
		strings.TrimPrefix(versionString(), "cliche "), runtime.GOOS, runtime.GOARCH))
	return issuesURL + "?" + q.Encode()
}
