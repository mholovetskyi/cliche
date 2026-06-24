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
// pick one to `cliche chat --resume <id>`. The `rm` subcommand deletes sessions:
// cliche sessions rm <id>...
func cmdSessions(args []string, out, errOut io.Writer) int {
	if len(args) > 0 && args[0] == "rm" {
		return rmSessions(args[1:], out, errOut)
	}
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
	fmt.Fprintln(out, "  "+style.Gray("delete one with `cliche sessions rm <id>`"))
	return 0
}

// rmSessions deletes one or more saved sessions by id.
func rmSessions(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("sessions rm", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ids := fs.Args()
	if len(ids) == 0 {
		fmt.Fprintln(errOut, "usage: cliche sessions rm <id>... (list ids with `cliche sessions`)")
		return 2
	}
	deleted := 0
	for _, id := range ids {
		if err := sess.Delete(*dir, id); err != nil {
			fmt.Fprintln(errOut, "  rm "+id+": "+err.Error())
			continue
		}
		fmt.Fprintln(out, "  deleted "+id)
		deleted++
	}
	if deleted == 0 {
		return 1
	}
	return 0
}
