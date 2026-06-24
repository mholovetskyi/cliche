package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

func skillTemplate(name string) string {
	return "---\nname: " + name + "\ndescription: when to use this skill — one line the agent matches a task against\n---\n\n" +
		"# " + name + "\n\n" +
		"Instructions the agent follows when this skill applies. Be specific: the\n" +
		"steps to take, which tools to use, the expected output, and any constraints.\n\n" +
		"You can keep helper files (scripts, templates) alongside this SKILL.md and\n" +
		"reference them by relative path.\n"
}
