package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/shell"
)

// diagnostics runs the project's REAL type-checker / compiler / linter and returns
// the findings as structured file:line:col diagnostics — the "does this serious
// codebase actually compile and type-check" feedback loop, without an external
// language server. It auto-detects the toolchain (Go, Node/TS, Python, Rust),
// only runs a checker whose binary is installed, and never mutates anything.
//
// It is read-only (allowed in plan mode) but still gated like run_command, because
// running a project's own npm/lint script executes project-authored code.
func (e OSExecutor) diagnostics(ctx context.Context, args map[string]string) Result {
	if e.ruleDecision("run", "diagnostics") == ruleDeny {
		return Result{Output: "blocked by deny rule: diagnostics", Success: false}
	}
	if !e.permit("run", "diagnostics", "run", "run project diagnostics (type-check / build — read-only)") {
		return Result{Output: "permission denied: diagnostics", Success: false}
	}
	root := e.Root
	if root == "" {
		root, _ = os.Getwd()
	}
	if p := strings.TrimSpace(firstNonEmpty(args["path"], args["dir"])); p != "" {
		if abs, err := e.resolve(p); err == nil {
			root = abs
		}
	}

	projects := detectProjects(root)
	if len(projects) == 0 {
		return Result{Output: "diagnostics: no supported project found under " + root + " (looked for go.mod, package.json, pyproject.toml/requirements.txt, Cargo.toml).", Success: true}
	}

	var diags []diagnostic
	var ran, skipped []string
	var raw strings.Builder
	for _, pr := range projects {
		for _, chk := range checkersFor(pr) {
			if _, err := exec.LookPath(chk.bin); err != nil {
				skipped = append(skipped, chk.label+" (no "+chk.bin+")")
				continue
			}
			out, failed := e.runChecker(ctx, pr.dir, chk.cmd)
			ran = append(ran, chk.label)
			found := chk.parse(out, pr.dir, root, chk.source)
			diags = append(diags, found...)
			// If a checker failed but we couldn't structure its output, surface the
			// raw (bounded) output so nothing is hidden from the agent.
			if failed && len(found) == 0 && strings.TrimSpace(out) != "" {
				raw.WriteString("\n── raw output from " + chk.label + " ──\n" + strings.TrimSpace(out) + "\n")
			}
		}
	}

	report := renderDiagnostics(diags, ran, skipped, raw.String())
	// Success = no errors (warnings don't fail the check), so the agent can branch on it.
	ok := true
	for _, d := range diags {
		if d.Severity == "error" {
			ok = false
			break
		}
	}
	return Result{Output: boundOutput(report, runOutputLimit), Success: ok}
}

type diagnostic struct {
	File, Severity, Message, Source string
	Line, Col                       int
}

type project struct{ dir, lang string }

type checker struct {
	bin, cmd, label, source string
	parse                   func(out, dir, root, source string) []diagnostic
}

// detectProjects finds the manifests under root and one level down (so a monorepo
// like this repo's studio/ subtree is checked alongside the root Go module),
// skipping the usual VCS/deps/build dirs.
func detectProjects(root string) []project {
	var ps []project
	seen := map[string]bool{}
	add := func(dir, lang string) {
		k := dir + "|" + lang
		if !seen[k] {
			seen[k] = true
			ps = append(ps, project{dir, lang})
		}
	}
	scan := func(dir string, primary bool) {
		has := func(f string) bool { _, err := os.Stat(filepath.Join(dir, f)); return err == nil }
		switch {
		case has("go.mod"):
			add(dir, "go")
		case primary && hasGoFiles(dir) && insideGoModule(dir):
			// A package subdir of a module (no go.mod of its own): `go build ./...`
			// resolves the module from a parent, so check it where the user pointed.
			add(dir, "go")
		}
		if has("package.json") {
			add(dir, "node")
		}
		if has("pyproject.toml") || has("requirements.txt") {
			add(dir, "python")
		}
		if has("Cargo.toml") {
			add(dir, "rust")
		}
	}
	scan(root, true)
	if entries, err := os.ReadDir(root); err == nil {
		for _, en := range entries {
			if en.IsDir() && !skipDirs[en.Name()] && !strings.HasPrefix(en.Name(), ".") {
				scan(filepath.Join(root, en.Name()), false)
			}
		}
	}
	return ps
}

func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, en := range entries {
		if !en.IsDir() && strings.HasSuffix(en.Name(), ".go") {
			return true
		}
	}
	return false
}

// insideGoModule reports whether dir is within a Go module (a parent has go.mod).
func insideGoModule(dir string) bool {
	d := filepath.Dir(dir)
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}

// checkersFor returns the ordered checkers to try for a detected project. Only
// those whose binary is on PATH actually run; the rest are reported as skipped.
func checkersFor(p project) []checker {
	switch p.lang {
	case "go":
		// build catches compile errors; vet adds Printf/shadow checks build misses.
		return []checker{
			{bin: "go", cmd: "go build ./...", label: "go build", source: "go build", parse: parseGo},
			{bin: "go", cmd: "go vet ./...", label: "go vet", source: "go vet", parse: parseGoVet},
		}
	case "node":
		// Prefer the project's locally-installed tsc; never trigger an npx download.
		local := filepath.Join(p.dir, "node_modules", ".bin", "tsc")
		if _, err := os.Stat(local); err == nil {
			return []checker{{bin: local, cmd: shellQuote(local) + " --noEmit", label: "tsc", source: "tsc", parse: parseTSC}}
		}
		if s := npmCheckScript(p.dir); s != "" {
			return []checker{{bin: "npm", cmd: "npm run " + s, label: "npm run " + s, source: s, parse: parseTSC}}
		}
		return nil
	case "python":
		return []checker{
			{bin: "mypy", cmd: "mypy --no-error-summary --show-column-numbers .", label: "mypy", source: "mypy", parse: parseColonSeverity},
			{bin: "ruff", cmd: "ruff check .", label: "ruff", source: "ruff", parse: parseColonSeverity},
		}
	case "rust":
		return []checker{{bin: "cargo", cmd: "cargo check", label: "cargo check", source: "cargo", parse: parseColonSeverity}}
	}
	return nil
}

// npmCheckScript returns a package.json script that performs a non-mutating check
// (typecheck/lint), preferring typecheck. Empty if none.
func npmCheckScript(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	s := string(b)
	for _, name := range []string{"typecheck", "type-check", "tsc", "lint", "check"} {
		if strings.Contains(s, "\""+name+"\"") {
			return name
		}
	}
	return ""
}

func (e OSExecutor) runChecker(ctx context.Context, dir, command string) (out string, failed bool) {
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := shell.Command(cctx, dir, command)
	if e.Policy.Sandbox {
		cmd.Env = scrubbedEnv()
	}
	b, err := cmd.CombinedOutput()
	return string(b), err != nil
}

var (
	// Real Go output (verified on Windows): ".\main.go:3:39: undefined: undeclared"
	// and "vet.exe: .\main.go:3:39: ...". No severity word; paths may be "./" or ".\".
	goDiagRe = regexp.MustCompile(`(?m)^(?:[^\s:]*vet[^\s:]*:\s+)?(?:\.[\\/])?(?P<file>[^:\n]+?\.go):(?P<line>\d+):(?P<col>\d+):\s+(?P<msg>.+?)\r?$`)
	// tsc:  src/index.ts(5,10): error TS2322: message
	tscRe = regexp.MustCompile(`(?m)^(?P<file>[^()\n]+?)\((?P<line>\d+),(?P<col>\d+)\):\s+error\s+TS\d+:\s+(?P<msg>.+?)\r?$`)
	// generic file:line:col: severity: message (mypy / many tools)
	colonRe = regexp.MustCompile(`(?m)^(?P<file>[^:\n]+?):(?P<line>\d+):(?P<col>\d+):\s+(?P<sev>error|warning|note):\s+(?P<msg>.+?)\r?$`)
)

func parseGo(out, dir, root, source string) []diagnostic {
	return parseWith(goDiagRe, out, dir, root, source, "error")
}
func parseGoVet(out, dir, root, source string) []diagnostic {
	// vet findings that aren't compile errors are warnings.
	return parseWith(goDiagRe, out, dir, root, source, "warning")
}
func parseTSC(out, dir, root, source string) []diagnostic {
	return parseWith(tscRe, out, dir, root, source, "error")
}
func parseColonSeverity(out, dir, root, source string) []diagnostic {
	return parseWith(colonRe, out, dir, root, source, "")
}

// parseWith applies a named-group regex (file/line/col, optional msg/sev) and
// resolves each file relative to the workspace root for stable, clickable paths.
func parseWith(re *regexp.Regexp, out, dir, root, source, defSev string) []diagnostic {
	var ds []diagnostic
	idx := re.SubexpIndex
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		get := func(name string) string {
			if i := idx(name); i >= 0 && i < len(m) {
				return m[i]
			}
			return ""
		}
		line, _ := strconv.Atoi(get("line"))
		col, _ := strconv.Atoi(get("col"))
		sev := get("sev")
		if sev == "" {
			sev = defSev
		}
		file := strings.TrimSpace(get("file"))
		if full := filepath.Join(dir, filepath.FromSlash(file)); full != "" {
			if rel, err := filepath.Rel(root, full); err == nil {
				file = filepath.ToSlash(rel)
			}
		}
		ds = append(ds, diagnostic{File: file, Line: line, Col: col, Severity: sev, Message: strings.TrimSpace(get("msg")), Source: source})
	}
	return ds
}

func renderDiagnostics(diags []diagnostic, ran, skipped []string, raw string) string {
	var errs, warns int
	for _, d := range diags {
		if d.Severity == "warning" || d.Severity == "note" {
			warns++
		} else {
			errs++
		}
	}
	// Deterministic order: by file, then line, then col.
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].File != diags[j].File {
			return diags[i].File < diags[j].File
		}
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Col < diags[j].Col
	})

	var b strings.Builder
	for _, d := range diags {
		sev := d.Severity
		if sev == "" {
			sev = "error"
		}
		msg := d.Message
		if d.Message == "" {
			msg = "(no message)"
		}
		fmt.Fprintf(&b, "%s:%d:%d: %s: %s [%s]\n", d.File, d.Line, d.Col, sev, msg, d.Source)
	}
	if len(diags) == 0 && raw == "" {
		b.WriteString("No problems found. ")
	}
	summary := fmt.Sprintf("diagnostics: %d error(s), %d warning(s)", errs, warns)
	if len(ran) > 0 {
		summary += " · ran: " + strings.Join(ran, ", ")
	}
	if len(skipped) > 0 {
		summary += " · skipped: " + strings.Join(skipped, ", ")
	}
	b.WriteString(summary)
	if raw != "" {
		b.WriteString("\n" + raw)
	}
	return b.String()
}

// shellQuote wraps a path in double quotes if it contains spaces, so a tsc path
// like "C:\Program Files\..." survives the shell.
func shellQuote(s string) string {
	if strings.ContainsAny(s, " \t") {
		return "\"" + s + "\""
	}
	return s
}
