package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/style"
)

// userCommand is a saved-prompt slash shortcut from .cliche/commands/<name>.md —
// the user types /<name> and the body (with argument substitution) runs as a task.
type userCommand struct {
	Name string // "/review"
	Desc string
	Body string // prompt template, with $ARGUMENTS and $1..$9 placeholders
}

func commandsDir(root string) string { return filepath.Join(config.Dir(root), "commands") }

// loadCommands reads .cliche/commands/*.md AND every plugin's commands/ bundle
// into a map keyed by "/name". Plugins load first so a project command of the
// same name takes precedence.
func loadCommands(root string) map[string]userCommand {
	out := map[string]userCommand{}
	for _, p := range loadPlugins(root) {
		mergeCommands(out, filepath.Join(p.Dir, "commands"))
	}
	mergeCommands(out, commandsDir(root))
	return out
}

func mergeCommands(out map[string]userCommand, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		meta, body := parseFrontmatter(string(data))
		name := "/" + strings.TrimSuffix(e.Name(), ".md")
		desc := meta["description"]
		if desc == "" {
			desc = "custom command"
		}
		out[name] = userCommand{Name: name, Desc: desc, Body: strings.TrimSpace(body)}
	}
}

// expand substitutes $ARGUMENTS (all args, space-joined) and $1..$9 (positional)
// in the body, clearing any leftover positional placeholders.
func (c userCommand) expand(args []string) string {
	body := strings.ReplaceAll(c.Body, "$ARGUMENTS", strings.Join(args, " "))
	for i := 1; i <= 9; i++ {
		v := ""
		if i <= len(args) {
			v = args[i-1]
		}
		body = strings.ReplaceAll(body, "$"+strconv.Itoa(i), v)
	}
	return strings.TrimSpace(body)
}

func sortedCommands(m map[string]userCommand) []userCommand {
	out := make([]userCommand, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// showCommands (/commands) lists the user's custom commands in-session.
func (s *session) showCommands() {
	cmds := sortedCommands(s.customCmds)
	if len(cmds) == 0 {
		fmt.Fprintln(s.out, "  no custom commands yet")
		fmt.Fprintln(s.out, "  "+style.Gray("create one: `cliche commands new <name>` → a prompt at .cliche/commands/<name>.md, run as /<name>"))
		return
	}
	fmt.Fprintln(s.out, "  "+style.White("custom commands"))
	for _, c := range cmds {
		fmt.Fprintf(s.out, "    %s %s\n", style.White(style.Pad(c.Name, 16)), style.Gray(c.Desc))
	}
}

// cmdCommands is `cliche commands [new <name>]`: list, or scaffold a new one.
func cmdCommands(args []string, out, errOut io.Writer) int {
	if len(args) >= 1 && args[0] == "new" {
		if len(args) < 2 {
			fmt.Fprintln(errOut, "usage: cliche commands new <name>")
			return 2
		}
		name := strings.TrimPrefix(args[1], "/")
		path := filepath.Join(commandsDir("."), name+".md")
		created, err := scaffold(path, commandTemplate(name))
		switch {
		case err != nil:
			fmt.Fprintln(errOut, "commands: "+err.Error())
			return 1
		case !created:
			fmt.Fprintln(errOut, "commands: "+path+" already exists")
			return 1
		}
		fmt.Fprintln(out, "  created "+path)
		fmt.Fprintln(out, "  "+style.Gray("edit it, then run it in chat as /"+name))
		return 0
	}
	cmds := sortedCommands(loadCommands("."))
	if len(cmds) == 0 {
		fmt.Fprintln(out, "  no custom commands. create one with `cliche commands new <name>`")
		return 0
	}
	fmt.Fprintln(out, "\n  "+style.BoldWhite("custom commands")+style.Gray("  ·  .cliche/commands/<name>.md  ·  invoke as /<name>"))
	for _, c := range cmds {
		fmt.Fprintf(out, "  %s %s\n", style.White(fmt.Sprintf("%-18s", c.Name)), style.Gray(c.Desc))
	}
	return 0
}

func commandTemplate(name string) string {
	return "---\ndescription: " + name + " — what this command does\n---\n\n" +
		"Write the prompt Cliche runs when you type /" + name + " in chat.\n\n" +
		"Use $ARGUMENTS for everything after the command, or $1, $2 … for positional args.\n\n" +
		"Example: Review the changes in $ARGUMENTS for bugs and missing tests, then propose fixes.\n"
}
