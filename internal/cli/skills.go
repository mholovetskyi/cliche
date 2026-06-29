package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/style"
)

// skill is a packaged capability from .cliche/skills/<name>/SKILL.md: frontmatter
// (name, description) the agent matches against, plus a body of instructions it
// follows. Skills are surfaced to the agent (via the system prompt) so it uses
// them autonomously, and can also be invoked explicitly with /skill <name>.
type skill struct {
	Name string
	Desc string
	Rel  string // SKILL.md path relative to the project root (for the agent to read)
	Body string
}

func skillsDir(root string) string { return filepath.Join(config.Dir(root), "skills") }

// loadSkills discovers skills from .cliche/skills/ AND every installed plugin's
// skills/ bundle, sorted by name.
func loadSkills(root string) []skill {
	out := loadSkillsFrom(skillsDir(root), root)
	for _, p := range loadPlugins(root) {
		out = append(out, loadSkillsFrom(filepath.Join(p.Dir, "skills"), root)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// loadSkillsFrom reads the <name>/SKILL.md skills directly under dir, computing
// each Rel relative to root (so the agent can read it with read_file).
func loadSkillsFrom(dir, root string) []skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		meta, body := parseFrontmatter(string(data))
		name := meta["name"]
		if name == "" {
			name = e.Name()
		}
		desc := meta["description"]
		if desc == "" {
			desc = "(no description)"
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, skill{Name: name, Desc: desc, Rel: filepath.ToSlash(rel), Body: strings.TrimSpace(body)})
	}
	return out
}

// skillsSystemNote is the "Available skills" addendum injected into the agent
// system prompt, so the model knows which skills exist and reads the full
// SKILL.md when a task matches. Empty when there are no skills.
func skillsSystemNote(root string) string {
	skills := loadSkills(root)
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nAvailable skills — when a task matches one, read its SKILL.md with read_file and follow it:\n")
	for _, s := range skills {
		b.WriteString(fmt.Sprintf("- %s: %s [%s]\n", s.Name, s.Desc, s.Rel))
	}
	return b.String()
}

// learnSkillNote is the always-on learning-loop nudge: it encourages capturing
// genuinely reusable workflows as skills (guard-railed so it doesn't spam the
// library), independent of whether any skills exist yet.
func learnSkillNote() string {
	return "\n\nWhen you finish a NON-TRIVIAL, REUSABLE workflow — one you'd repeat on similar tasks — call save_skill{name,description,content} to capture it for future sessions (the user approves it; it's saved to .cliche/skills/). Skip one-offs, trivial steps, and plain facts (use remember for facts)."
}

// skillMap keys loaded skills by name for /skill lookup.
func skillMap(root string) map[string]skill {
	m := map[string]skill{}
	for _, s := range loadSkills(root) {
		m[s.Name] = s
	}
	return m
}

// showSkills (/skills) lists installed skills in-session.
func (s *session) showSkills() {
	skills := loadSkills(s.dir)
	if len(skills) == 0 {
		fmt.Fprintln(s.out, "  no skills installed yet")
		fmt.Fprintln(s.out, "  "+style.Gray("create one: `cliche skills new <name>` → .cliche/skills/<name>/SKILL.md the agent reads when a task matches"))
		return
	}
	fmt.Fprintln(s.out, "  "+style.White("skills")+style.Gray("  ·  the agent uses these automatically; force one with /skill <name>"))
	for _, sk := range skills {
		fmt.Fprintf(s.out, "    %s %s\n", style.White(style.Pad(sk.Name, 16)), style.Gray(sk.Desc))
	}
}

// invokeSkill (/skill <name> [input]) returns the skill body as a task prompt; on
// a missing name it prints an error and returns run=false.
func (s *session) invokeSkill(args []string) (string, bool) {
	if len(args) == 0 {
		fmt.Fprintln(s.out, "  usage: /skill <name>  (see /skills)")
		return "", false
	}
	sk, ok := s.skills[args[0]]
	if !ok {
		fmt.Fprintf(s.out, "  no skill %q — see /skills\n", args[0])
		return "", false
	}
	prompt := "Follow this skill:\n\n" + sk.Body
	if extra := strings.Join(args[1:], " "); extra != "" {
		prompt += "\n\nInput: " + extra
	}
	return prompt, true
}

// cmdSkills is `cliche skills [new <name>]`: list, or scaffold a new skill.
func cmdSkills(args []string, out, errOut io.Writer) int {
	if len(args) >= 1 && args[0] == "add" {
		if len(args) < 2 {
			fmt.Fprintln(errOut, "usage: cliche skills add <https-url-to-a-SKILL.md>")
			return 2
		}
		return skillsAdd(args[1], out, errOut)
	}
	if len(args) >= 1 && args[0] == "new" {
		if len(args) < 2 {
			fmt.Fprintln(errOut, "usage: cliche skills new <name>")
			return 2
		}
		name := args[1]
		path := filepath.Join(skillsDir("."), name, "SKILL.md")
		created, err := scaffold(path, skillTemplate(name))
		switch {
		case err != nil:
			fmt.Fprintln(errOut, "skills: "+err.Error())
			return 1
		case !created:
			fmt.Fprintln(errOut, "skills: "+path+" already exists")
			return 1
		}
		fmt.Fprintln(out, "  created "+path)
		fmt.Fprintln(out, "  "+style.Gray("edit it; the agent will use it automatically, or force it with /skill "+name))
		return 0
	}
	skills := loadSkills(".")
	if len(skills) == 0 {
		fmt.Fprintln(out, "  no skills installed. create one with `cliche skills new <name>`")
		return 0
	}
	fmt.Fprintln(out, "\n  "+style.BoldWhite("skills")+style.Gray("  ·  .cliche/skills/<name>/SKILL.md"))
	for _, sk := range skills {
		fmt.Fprintf(out, "  %s %s\n", style.White(fmt.Sprintf("%-18s", sk.Name)), style.Gray(sk.Desc))
	}
	return 0
}

// skillsAdd downloads a SKILL.md from a URL and installs it under .cliche/skills —
// the community/hub path (`cliche skills add <url>`). The skill is a plain file the
// user can read; it's installed as-is (after validating it parses) with a nudge to
// review it, since the agent will follow its instructions.
func skillsAdd(url string, out, errOut io.Writer) int {
	name, path, err := installSkillFromURL(url, ".")
	if err != nil {
		fmt.Fprintln(errOut, "skills add: "+err.Error())
		return 1
	}
	fmt.Fprintf(out, "  installed skill %s → %s\n", style.BoldWhite(name), path)
	fmt.Fprintln(out, "  "+style.Gray("review it before relying on it — the agent follows its instructions"))
	return 0
}

// installSkillFromURL downloads + validates a SKILL.md and installs it under
// <root>/.cliche/skills/<slug>/. Shared by `cliche skills add` (CLI) and Studio's
// Skills & Tools panel. The slug sanitizer guarantees the install can never escape
// the skills dir; size (256KB) and time (25s) are bounded.
func installSkillFromURL(url, root string) (name, path string, err error) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return "", "", fmt.Errorf("expected an http(s) URL to a SKILL.md")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("%s returned %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", "", err
	}
	meta, body := parseFrontmatter(string(data))
	name = skillSlug(meta["name"])
	if name == "" || strings.TrimSpace(body) == "" {
		return "", "", fmt.Errorf("not a valid SKILL.md (needs `name:` frontmatter and a body)")
	}
	path = filepath.Join(skillsDir(root), name, "SKILL.md")
	if _, statErr := os.Stat(path); statErr == nil {
		return "", "", fmt.Errorf("%q is already installed (%s) — remove it first", name, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", "", err
	}
	return name, path, nil
}

// skillSlug turns a frontmatter name into a safe directory name (kebab, [a-z0-9-]),
// so an installed skill can never escape .cliche/skills/.
func skillSlug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == ' ' || r == '-' || r == '_' || r == '/' || r == '.':
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func skillTemplate(name string) string {
	return "---\nname: " + name + "\ndescription: when to use this skill — one line the agent matches a task against\n---\n\n" +
		"# " + name + "\n\n" +
		"Instructions the agent follows when this skill applies. Be specific: the\n" +
		"steps to take, which tools to use, the expected output, and any constraints.\n\n" +
		"You can keep helper files (scripts, templates) alongside this SKILL.md and\n" +
		"reference them by relative path.\n"
}
