package cli

import (
	"fmt"
	"strconv"
	"strings"

	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/style"
)

// addTask (/plan <task>) appends a task to the session's lightweight plan, which
// persists with the transcript so a resumed session keeps its focus. It is a
// thin to-do list, complementary to the read-only plan *mode*.
func (s *session) addTask(line string) {
	title := strings.TrimSpace(strings.TrimPrefix(line, "/plan"))
	if title == "" {
		fmt.Fprintln(s.out, "  usage: /plan <task description>")
		return
	}
	s.nextTaskID++
	s.tasks = append(s.tasks, sess.Task{ID: s.nextTaskID, Title: title})
	fmt.Fprintf(s.out, "  %s %s %s\n", style.Green(gl("+", "+")),
		style.Gray(fmt.Sprintf("#%d", s.nextTaskID)), style.White(title))
	s.persist() // checkpoint the plan immediately (no-op until the session has an id)
}

// showTasks (/tasks) lists the plan with done/open marks and a progress count.
func (s *session) showTasks() {
	if len(s.tasks) == 0 {
		fmt.Fprintln(s.out, "  no tasks yet — add one with `/plan <task>`")
		return
	}
	done := 0
	for _, t := range s.tasks {
		if t.Done {
			done++
		}
	}
	fmt.Fprintf(s.out, "  %s %s\n", style.White("plan"), style.Gray(fmt.Sprintf("· %d/%d done", done, len(s.tasks))))
	for _, t := range s.tasks {
		mark, title := style.Gray(gl("○", " ")), style.White(t.Title)
		if t.Done {
			mark, title = style.Green(gl("✓", "x")), style.Dim(t.Title)
		}
		fmt.Fprintf(s.out, "    %s %s %s\n", mark, style.Gray(fmt.Sprintf("#%d", t.ID)), title)
	}
}

// markTaskDone (/done <id>) marks a plan task complete.
func (s *session) markTaskDone(line string) {
	arg := strings.TrimSpace(strings.TrimPrefix(line, "/done"))
	id, err := strconv.Atoi(arg)
	if err != nil {
		fmt.Fprintln(s.out, "  usage: /done <task id>  (see /tasks)")
		return
	}
	for i := range s.tasks {
		if s.tasks[i].ID == id {
			s.tasks[i].Done = true
			fmt.Fprintf(s.out, "  %s %s\n", style.Green(gl("✓", "x")), style.Dim(s.tasks[i].Title))
			s.persist()
			return
		}
	}
	fmt.Fprintf(s.out, "  no task #%d (see /tasks)\n", id)
}
