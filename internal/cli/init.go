package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/style"
)

// agentsTemplate scaffolds an AGENTS.md for the user's project. The "## verify"
// section is the one Cliche's verifier reads: a real `test:` command there is
// what makes a "verified" verdict mean the project's own tests actually passed.
const agentsTemplate = `# AGENTS.md

Project context for AI coding agents (Cliche reads this; so do other tools).
Keep it short and high-signal — it is loaded into the agent's context.

## What this is

<one paragraph: what this project does, the language/framework, where to start>

## Build / test

- build: <command, e.g. ` + "`go build ./...`" + `>
- test:  <command, e.g. ` + "`go test ./...`" + `>

## Conventions

- <house style, libraries to prefer/avoid, anything an agent should not do>

## verify

Cliche's verifier re-runs the command below and only reports "verified" when it
actually passes — so a faked green bar gets caught. Set it to your real tests:

test: <your test command, e.g. go test ./... | npm test | pytest -q>
`

// cmdInit scaffolds a project for Cliche: a default .cliche/config.json and, if
// none exists, an AGENTS.md template. It never overwrites existing files, so
// re-running it is safe.
func cmdInit(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root := *dir

	if err := os.MkdirAll(config.Dir(root), 0o755); err != nil {
		fmt.Fprintln(errOut, "init: "+err.Error())
		return 1
	}

	kept := func(what string) {
		fmt.Fprintf(out, "  %s %s\n", style.Gray(gl("•", "-")), style.Gray("kept existing "+what))
	}
	made := func(what string) {
		fmt.Fprintf(out, "  %s %s\n", style.Red(gl("✔", "+")), style.White("created "+what))
	}

	// .cliche/config.json — the effective, validated defaults, ready to edit.
	cfgRel := filepath.ToSlash(filepath.Join(".cliche", "config.json"))
	cfgPath := filepath.Join(config.Dir(root), "config.json")
	if _, err := os.Stat(cfgPath); err == nil {
		kept(cfgRel)
	} else {
		data, _ := json.MarshalIndent(config.Default(), "", "  ")
		if err := os.WriteFile(cfgPath, append(data, '\n'), 0o644); err != nil {
			fmt.Fprintln(errOut, "init: "+err.Error())
			return 1
		}
		made(cfgRel)
	}

	// AGENTS.md — only if no project-context file already exists.
	if name, ok := config.HasAgentsFile(root); ok {
		kept(name + " (project context)")
	} else {
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(agentsTemplate), 0o644); err != nil {
			fmt.Fprintln(errOut, "init: "+err.Error())
			return 1
		}
		made("AGENTS.md")
	}

	fmt.Fprintln(out, "\n  "+style.Gray("next: export your API key, then `cliche chat`. edit ")+cfgRel+style.Gray(" to tune caps."))
	return 0
}
