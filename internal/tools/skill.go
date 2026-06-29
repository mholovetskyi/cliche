package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// sanitizeSkillName turns a proposed skill name into a safe directory name:
// lowercase kebab-case, [a-z0-9-] only — so a learned skill can never escape
// .cliche/skills/ (no slashes, dots, or traversal survive).
func sanitizeSkillName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
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

// writeSkill saves an agent-learned skill to <root>/.cliche/skills/<name>/SKILL.md
// in the same frontmatter format loadSkills reads, so it's picked up automatically
// next session. Plain Markdown on purpose: a learned skill must be readable,
// reviewable, and deletable — never a hidden self-modification.
func writeSkill(root, name, desc, content string) error {
	dir := filepath.Join(root, ".cliche", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	desc = strings.TrimSpace(strings.ReplaceAll(desc, "\n", " "))
	doc := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + strings.TrimSpace(content) + "\n"
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(doc), 0o644)
}
