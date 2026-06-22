package cli

import (
	"flag"
	"fmt"
	"io"
	"time"

	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdSessions lists saved chat sessions (most recent first), so the user can
// pick one to `cliche chat --resume <id>`.
func cmdSessions(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	metas, err := sess.List(*dir)
	if err != nil {
		fmt.Fprintln(errOut, "sessions: "+err.Error())
		return 1
	}
	if len(metas) == 0 {
		fmt.Fprintln(out, "  no saved sessions yet. start one with `cliche chat`.")
		return 0
	}
	fmt.Fprintln(out, "\n  "+style.BoldWhite("saved sessions")+style.Gray("  ·  resume with `cliche chat --resume <id>` (or --continue for the latest)"))
	for _, m := range metas {
		title := m.Title
		if len(title) > 48 {
			title = title[:47] + "…"
		}
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(out, "  %s  %s  %s\n",
			style.Color(m.ID, style.Sample(0)),
			style.White(fmt.Sprintf("%-49s", title)),
			style.Gray(fmt.Sprintf("%s · %d msgs · %s", m.Model, m.Messages, m.Updated.Local().Format(time.RFC822))))
	}
	return 0
}
