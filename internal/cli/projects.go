package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/projects"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// touchProject records a project root in the global registry (best-effort — the
// registry is a convenience index, never load-bearing, so errors are ignored).
// Called whenever real work runs in a directory.
func touchProject(dir string) {
	home, err := secrets.ConfigHome()
	if err != nil {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	reg, _ := projects.Load(home)
	reg.Upsert(abs, filepath.Base(abs), time.Now())
	_ = reg.Save(home)
}

// cmdProjects lists / manages the cross-project registry.
func cmdProjects(args []string, out, errOut io.Writer) int {
	home, err := secrets.ConfigHome()
	if err != nil {
		fmt.Fprintln(errOut, "projects: "+err.Error())
		return 1
	}
	reg, err := projects.Load(home)
	if err != nil {
		fmt.Fprintln(errOut, "projects: "+err.Error())
		return 1
	}

	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "", "list":
		renderProjects(out, reg)
	case "add":
		p := "."
		if len(args) > 1 {
			p = args[1]
		}
		abs, _ := filepath.Abs(p)
		reg.Upsert(abs, filepath.Base(abs), time.Now())
		if err := reg.Save(home); err != nil {
			fmt.Fprintln(errOut, "projects: "+err.Error())
			return 1
		}
		fmt.Fprintf(out, "  %s registered %s\n", style.Green("+"), style.White(abs))
	case "rm", "remove", "forget":
		if len(args) < 2 {
			fmt.Fprintln(errOut, "projects rm: need a path or name")
			return 2
		}
		key := args[1]
		abs, _ := filepath.Abs(key)
		if !reg.Remove(abs) && !reg.Remove(key) {
			fmt.Fprintln(errOut, "projects: not tracked: "+key)
			return 1
		}
		if err := reg.Save(home); err != nil {
			fmt.Fprintln(errOut, "projects: "+err.Error())
			return 1
		}
		fmt.Fprintf(out, "  %s forgot %s\n", style.Gray("−"), style.White(key))
	case "workspace":
		if len(args) > 1 {
			abs, _ := filepath.Abs(args[1])
			reg.Workspace = abs
			if err := reg.Save(home); err != nil {
				fmt.Fprintln(errOut, "projects: "+err.Error())
				return 1
			}
			fmt.Fprintf(out, "  workspace set to %s\n", style.White(abs))
		} else if reg.Workspace == "" {
			fmt.Fprintln(out, "  "+style.Gray("no workspace set · cliche projects workspace <path>"))
		} else {
			fmt.Fprintln(out, "  workspace: "+style.White(reg.Workspace))
		}
	default:
		fmt.Fprintln(errOut, "projects: unknown subcommand "+sub+" (list | add | rm | workspace)")
		return 2
	}
	return 0
}

func renderProjects(out io.Writer, reg *projects.Registry) {
	rec := reg.Recent()
	if len(rec) == 0 {
		fmt.Fprintln(out, "\n  "+style.Gray("no projects yet — run `cliche chat` in any folder, or `cliche new <name>`"))
		return
	}
	fmt.Fprintf(out, "\n  %s %s\n", style.BoldWhite("projects"), style.Gray("· where you've used Cliche"))
	if reg.Workspace != "" {
		fmt.Fprintln(out, "  "+style.Gray("workspace: "+reg.Workspace))
	}
	for _, p := range rec {
		tag := ""
		if _, err := os.Stat(p.Path); err != nil {
			tag = style.Red(" (missing)")
		}
		fmt.Fprintf(out, "  %s %s %s%s\n", style.Green("•"), style.White(style.Pad(p.Name, 18)), style.Gray(agoString(p.LastUsed)), tag)
		fmt.Fprintf(out, "    %s %s\n", style.Gray(p.Path), projectSpend(p.Path))
	}
	fmt.Fprintln(out, "\n  "+style.Gray("cd into one and run `cliche chat`, or `cliche chat --dir <path>`"))
}

// projectSpend reads a project's own ledger for its lifetime spend/turns —
// without creating anything (skips projects that have no ledger yet).
func projectSpend(path string) string {
	lpath := filepath.Join(config.Dir(path), "ledger.jsonl")
	if _, err := os.Stat(lpath); err != nil {
		return ""
	}
	led, err := ledger.Open(config.Dir(path)) // dir already exists → no creation
	if err != nil {
		return ""
	}
	s, err := led.Summarize()
	if err != nil || (s.USD == 0 && s.Turns == 0) {
		return ""
	}
	return style.Gray(fmt.Sprintf("$%.4f · %d turns", s.USD, s.Turns))
}

// cmdNew scaffolds a new project (folder + .cliche/config.json + AGENTS.md) and
// registers it. It places the project under --dir, else the configured
// workspace, else the current directory.
func cmdNew(args []string, out, errOut io.Writer) int {
	// Parse by hand so the name and --dir work in any order — Go's flag package
	// stops at the first positional, which would silently ignore a trailing
	// `--dir` (the natural `cliche new my-app --dir X` form).
	var parentVal, name string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--dir" || a == "-dir":
			if i+1 < len(args) {
				parentVal = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--dir="):
			parentVal = strings.TrimPrefix(a, "--dir=")
		case strings.HasPrefix(a, "-dir="):
			parentVal = strings.TrimPrefix(a, "-dir=")
		case name == "" && !strings.HasPrefix(a, "-"):
			name = a
		}
	}
	if name == "" {
		fmt.Fprintln(errOut, "new: a project name is required, e.g. cliche new my-app")
		return 2
	}
	parent := &parentVal

	home, err := secrets.ConfigHome()
	if err != nil {
		fmt.Fprintln(errOut, "new: "+err.Error())
		return 1
	}
	reg, _ := projects.Load(home)
	base := *parent
	if base == "" {
		base = reg.Workspace
	}
	if base == "" {
		base = "."
	}
	dst := filepath.Join(base, name)
	if entries, err := os.ReadDir(dst); err == nil && len(entries) > 0 {
		fmt.Fprintln(errOut, "new: "+dst+" already exists and is not empty (use `cliche projects add "+dst+"` to adopt it)")
		return 1
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		fmt.Fprintln(errOut, "new: "+err.Error())
		return 1
	}
	// Reuse `init` to scaffold config.json + AGENTS.md into the new folder.
	if code := cmdInit([]string{"--dir", dst}, out, errOut); code != 0 {
		return code
	}
	abs, _ := filepath.Abs(dst)
	reg.Upsert(abs, name, time.Now())
	_ = reg.Save(home)
	fmt.Fprintf(out, "\n  %s created %s\n", style.Green(gl("✓", "ok")), style.White(abs))
	fmt.Fprintf(out, "  %s\n", style.Gray("cd "+abs+" && cliche chat"))
	return 0
}

// agoString renders a coarse "time since" for the project list.
func agoString(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
