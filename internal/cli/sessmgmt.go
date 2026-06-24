package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/cli/lineedit"
	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/style"
)

// pickSession opens the arrow-key picker over this project's saved sessions and
// returns the chosen id. ok=false on cancel, when there are none, or when raw
// mode is unavailable (callers fall back to typed input / the latest session).
func (s *session) pickSession(header string) (string, bool) {
	metas, err := sess.List(s.dir)
	if err != nil || len(metas) == 0 {
		return "", false
	}
	items := make([]lineedit.SelectItem, len(metas))
	for i, m := range metas {
		title := m.Title
		if title == "" {
			title = "(untitled)"
		}
		label := m.ID
		if m.ID == s.id {
			label += " (current)"
		}
		items[i] = lineedit.SelectItem{
			Label: label,
			Desc:  fmt.Sprintf("%s · %d msgs · %s", style.Truncate(title, 32), m.Messages, m.Updated.Local().Format("Jan 2 15:04")),
		}
	}
	if idx, ok := s.pick(header, items); ok {
		return metas[idx].ID, true
	}
	return "", false
}

// showSessions (/sessions) lists this project's saved sessions, marking the
// current one, so the user can /resume any of them without leaving chat.
func (s *session) showSessions() {
	metas, err := sess.List(s.dir)
	if err != nil {
		fmt.Fprintln(s.out, "  sessions: "+err.Error())
		return
	}
	if len(metas) == 0 {
		fmt.Fprintln(s.out, "  no saved sessions yet.")
		return
	}
	fmt.Fprintln(s.out, "  "+style.White("sessions")+style.Gray("  ·  /resume <id> to switch · /new for a fresh one"))
	for _, m := range metas {
		title := m.Title
		if title == "" {
			title = "(untitled)"
		}
		marker, id := "  ", style.Color(m.ID, style.Sample(0))
		if m.ID == s.id {
			marker, id = style.BoldGreen(gl("›", ">"))+" ", style.BoldGreen(m.ID)
		}
		fmt.Fprintf(s.out, "  %s%s  %s  %s\n", marker, id,
			style.White(style.Pad(style.Truncate(title, 40), 40)),
			style.Gray(fmt.Sprintf("%d msgs · %s", m.Messages, m.Updated.Local().Format("Jan 2 15:04"))))
	}
}

// killSession (/kill <id>) deletes a saved session from disk. Deleting the
// current session is allowed but noted — it will re-save on exit unless you /new
// first.
func (s *session) killSession(line string) {
	id := strings.TrimSpace(strings.TrimPrefix(line, "/kill"))
	if id == "" {
		picked, ok := s.pickSession("kill a session")
		if !ok {
			fmt.Fprintln(s.out, "  usage: /kill <id>  (see /sessions)")
			return
		}
		id = picked
	}
	if err := sess.Delete(s.dir, id); err != nil {
		fmt.Fprintln(s.out, "  kill: "+err.Error())
		return
	}
	msg := "deleted session " + id
	if id == s.id {
		msg += " (current — re-saves on exit unless you /new first)"
	}
	fmt.Fprintf(s.out, "  %s %s\n", style.Red(gl("✗", "x")), style.White(msg))
}

// forkSession (/fork) branches the current conversation: the live transcript is
// kept but future turns save under a NEW id, so the original session file is
// frozen at the fork point and the two diverge from here.
func (s *session) forkSession() {
	s.persist() // freeze the original at the fork point
	old := s.id
	s.created = time.Now()
	s.id = sess.NewID(s.created)
	s.resumed = 0
	s.persist() // write the fork immediately
	fmt.Fprintf(s.out, "  %s forked %s → %s %s\n",
		style.Green(gl("⑃", "Y")), style.Gray(old), style.White(s.id), style.Gray("· same history, diverges from here"))
}

// newSession (/new) persists the current session and starts a fresh one: a new
// id and an empty transcript. The process-wide budget is preserved, so opening a
// new session can never be used to slip past the spend cap.
func (s *session) newSession() {
	s.persist()
	s.a.Reset()
	s.id = sess.NewID(time.Now())
	s.title, s.resumed = "", 0
	s.tasks, s.nextTaskID = nil, 0
	fmt.Fprintf(s.out, "  %s new session %s %s\n",
		style.Green(gl("✦", "*")), style.White(s.id), style.Gray("· budget preserved"))
}

// resumeSession (/resume [id]) persists the current session and loads a saved one
// (the most recent when no id is given) into the live chat. Only the transcript
// is swapped — the live budget keeps counting this process's cumulative spend.
func (s *session) resumeSession(line string) {
	id := strings.TrimSpace(strings.TrimPrefix(line, "/resume"))
	if id == "" {
		// Bare /resume opens the picker; fall back to the most recent if raw mode
		// is unavailable or the picker is cancelled.
		if picked, ok := s.pickSession("resume a session"); ok {
			id = picked
		} else {
			id = sess.Latest(s.dir)
		}
	}
	if id == "" {
		fmt.Fprintln(s.out, "  no saved session to resume (see /sessions).")
		return
	}
	if id == s.id {
		fmt.Fprintln(s.out, "  already on that session.")
		return
	}
	rec, err := sess.Load(s.dir, id)
	if err != nil {
		fmt.Fprintln(s.out, "  resume: "+err.Error())
		return
	}
	s.persist() // checkpoint the session we're leaving
	s.a.RestoreTranscript(rec.Messages)
	s.id, s.title, s.created = rec.ID, rec.Title, rec.Created
	s.resumed = len(rec.Messages)
	s.tasks, s.nextTaskID = rec.Tasks, 0
	for _, t := range rec.Tasks {
		if t.ID > s.nextTaskID {
			s.nextTaskID = t.ID
		}
	}
	fmt.Fprintf(s.out, "  %s resumed %s %s\n", style.Green(gl("↺", "*")), style.White(s.id),
		style.Gray(fmt.Sprintf("· %d messages · this session previously ~$%.4f", s.resumed, rec.Usage.USD)))
}
