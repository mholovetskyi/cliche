// Package profile is Cliche's cross-PROJECT user profile: a durable,
// human-readable USER.md in the user-global config dir (next to credentials),
// holding facts about the USER — preferences, stack, working style — that apply
// to EVERY project. It's loaded into the system prompt of every session so the
// agent adapts to you everywhere, not per-repo. (Project-scoped facts go to the
// project memory instead; see internal/memory.) Plain Markdown on purpose: the
// user can read, edit, and delete it.
package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholovetskyi/cliche/internal/secrets"
)

const header = "# About me — durable preferences Cliché learned about how I work (edit freely)\n\n"

// Path is the global USER.md location (honors CLICHE_CONFIG_HOME via secrets).
func Path() (string, error) {
	home, err := secrets.ConfigHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "USER.md"), nil
}

// Load returns the profile contents (trimmed), or "" if there is none.
func Load() string {
	p, err := Path()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\n")
}

// Append adds a user fact as a bullet (deduped), creating the file on first use.
func Append(fact string) error {
	fact = oneLine(fact)
	if fact == "" {
		return errors.New("empty fact")
	}
	bullet := "- " + fact
	for _, ln := range strings.Split(Load(), "\n") {
		if strings.TrimSpace(ln) == bullet {
			return nil // already known
		}
	}
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	prefix := ""
	if _, statErr := os.Stat(p); errors.Is(statErr, os.ErrNotExist) {
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

// SystemNote renders the profile for the system prompt (empty when there's none).
func SystemNote(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	return "\n\nAbout the user — durable preferences from across their projects. Honor these, and call the remember_user tool to save a NEW lasting preference about how they like to work:\n" + p
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
