// Package memory is Cliche's cross-session project memory: a durable,
// human-readable notebook at <root>/.cliche/memory.md. The agent appends facts
// worth keeping (conventions, decisions, gotchas, preferences) via the remember
// tool, and the file is loaded into the system prompt at the start of every
// session — so learnings persist across runs. Plain Markdown on purpose: a trust
// tool's memory should be readable, editable, and diffable, not a hidden store.
package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const header = "# Project memory\n\nDurable facts Cliche has learned about this project. Edit or delete freely.\n\n"

// Path is the memory file for a project root.
func Path(root string) string { return filepath.Join(root, ".cliche", "memory.md") }

// Load returns the memory contents (trimmed), or "" if there is none.
func Load(root string) string {
	data, err := os.ReadFile(Path(root))
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\n")
}

// Append adds a fact as a bullet, creating the file (with a header) on first
// use. Exact-duplicate facts are skipped so the notebook doesn't bloat.
func Append(root, fact string) error {
	fact = oneLine(fact)
	if fact == "" {
		return errors.New("empty fact")
	}
	bullet := "- " + fact
	for _, ln := range strings.Split(Load(root), "\n") {
		if strings.TrimSpace(ln) == bullet {
			return nil // already remembered
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".cliche"), 0o755); err != nil {
		return err
	}
	p := Path(root)
	prefix := ""
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		prefix = header
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(prefix + bullet + "\n")
	return err
}

// Clear removes the memory file. A missing file is not an error.
func Clear(root string) error {
	if err := os.Remove(Path(root)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// SystemNote frames loaded memory for injection into the agent's system prompt.
func SystemNote(mem string) string {
	if strings.TrimSpace(mem) == "" {
		return ""
	}
	return "\n\nProject memory — durable facts you saved in earlier sessions. Use them, and call the remember tool to save new facts worth keeping:\n" + mem
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
