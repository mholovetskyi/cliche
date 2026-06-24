package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mholovetskyi/cliche/internal/memory"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdMemory is `cliche memory [add <fact…> | clear]` — view, append to, or clear
// a project's cross-session memory. --dir may appear anywhere.
func cmdMemory(args []string, out, errOut io.Writer) int {
	dir := "."
	var rest []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--dir" || args[i] == "-dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case strings.HasPrefix(args[i], "--dir="):
			dir = strings.TrimPrefix(args[i], "--dir=")
		default:
			rest = append(rest, args[i])
		}
	}

	sub := ""
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "":
		renderMemory(out, dir)
	case "add":
		if err := memory.Append(dir, strings.Join(rest[1:], " ")); err != nil {
			fmt.Fprintln(errOut, "memory: "+err.Error())
			return 1
		}
		fmt.Fprintf(out, "  %s remembered\n", style.Green("+"))
	case "clear":
		if err := memory.Clear(dir); err != nil {
			fmt.Fprintln(errOut, "memory: "+err.Error())
			return 1
		}
		fmt.Fprintln(out, "  "+style.Gray("memory cleared"))
	default:
		fmt.Fprintln(errOut, "memory: unknown subcommand "+sub+" (add | clear)")
		return 2
	}
	return 0
}

func renderMemory(out io.Writer, dir string) {
	mem := memory.Load(dir)
	if mem == "" {
		fmt.Fprintln(out, "\n  "+style.Gray("no memory yet — the agent saves facts with the remember tool, or: cliche memory add \"…\""))
		return
	}
	fmt.Fprintf(out, "\n  %s %s\n", style.BoldWhite("memory"), style.Gray("· "+memory.Path(dir)))
	for _, ln := range strings.Split(mem, "\n") {
		fmt.Fprintln(out, "  "+ln)
	}
}

// showMemory is the in-chat /memory command.
func (s *session) showMemory() { renderMemory(s.out, s.dir) }
