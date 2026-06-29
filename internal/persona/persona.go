// Package persona is Cliche's agent personality layer — the analog of Hermes'
// SOUL.md / personalities. A persona shapes the agent's TONE and STYLE (how it
// talks and prioritizes), never its permissions: it is injected as plain system-
// prompt text inside the same Trust-Kernel-governed run, so it can't widen the
// budget cap, the governor, or the deny rules. There are built-in presets plus a
// user-authored PERSONA.md in the global config dir; the active choice persists
// next to it. Like USER.md, it's plain Markdown the user can read, edit, delete.
package persona

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mholovetskyi/cliche/internal/secrets"
)

// maxBody bounds the injected persona text — keeps the token cost and the
// prompt-injection surface small even if PERSONA.md is huge.
const maxBody = 4000

// Preset is a built-in personality.
type Preset struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
	Body  string `json:"body,omitempty"`
}

// Presets are the built-in personalities, shipped with Cliche (safe, curated).
func Presets() []Preset {
	return []Preset{
		{"default", "Default", "Cliché's standard voice — careful, concise, honest.", ""},
		{"concise", "Concise", "Answer first, minimal preamble, no filler.",
			"Adopt a terse, answer-first voice. Lead with the result or the change, then a one-line why only if it matters. No preamble, no recap of the request, no closing summary. Prefer a short list or a single sentence over a paragraph."},
		{"mentor", "Mentor", "Explains the why and teaches as it works.",
			"Adopt a patient, teaching voice. As you work, briefly explain the reasoning behind each decision and call out the concept or pattern at play, so the user learns — without becoming verbose. Define a term the first time it appears."},
		{"architect", "Architect", "Thinks in systems, surfaces tradeoffs.",
			"Adopt a systems-thinking voice. Before acting on anything non-trivial, name the design options and their tradeoffs in a sentence or two, flag where a choice has long-term or cross-cutting consequences, and prefer the simplest design that holds."},
		{"pair", "Pair programmer", "Collaborative, thinks out loud, suggests next steps.",
			"Adopt a collaborative pair-programming voice. Think out loud as you go, narrate what you're about to try, and end with a concrete suggested next step the user can accept or redirect. Treat it as a shared session, not a handoff."},
		{"reviewer", "Skeptic", "Critical eye — risks, edge cases, pushback.",
			"Adopt a skeptical, reviewing voice. Actively look for what could break: edge cases, failure modes, missing tests, and unstated assumptions. Push back when a request looks risky or under-specified, and say what you'd verify before trusting the result."},
		{"shipper", "Shipper", "Bias to action — the smallest working change.",
			"Adopt a pragmatic, bias-to-action voice. Reach for the smallest change that actually works and can ship now; avoid gold-plating and speculative abstraction. If something is out of scope, note it briefly and move on rather than expanding the task."},
	}
}

// preset returns the named built-in, or false.
func preset(name string) (Preset, bool) {
	for _, p := range Presets() {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

func customPath() (string, error) {
	home, err := secrets.ConfigHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "PERSONA.md"), nil
}

func activePath() (string, error) {
	home, err := secrets.ConfigHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "persona.active"), nil
}

// LoadCustom returns the user-authored PERSONA.md (trimmed), or "".
func LoadCustom() string {
	p, err := customPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// HasCustom reports whether a non-empty PERSONA.md exists.
func HasCustom() bool { return LoadCustom() != "" }

// Active is the selected persona name ("" / "default" = none).
func Active() string {
	p, err := activePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SetActive persists the chosen persona. "default"/"" clears it. A name must be a
// known preset or "custom" (requires a PERSONA.md) — anything else is rejected so
// the stored value is always resolvable.
func SetActive(name string) error {
	name = strings.TrimSpace(name)
	p, err := activePath()
	if err != nil {
		return err
	}
	if name == "" || name == "default" {
		_ = os.Remove(p)
		return nil
	}
	if name != "custom" {
		if _, ok := preset(name); !ok {
			return os.ErrInvalid
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(name+"\n"), 0o644)
}

// Resolve returns the persona body + a human title for a name. "custom" reads
// PERSONA.md; an unknown or default name yields an empty body.
func Resolve(name string) (body, title string) {
	switch strings.TrimSpace(name) {
	case "", "default":
		return "", "Default"
	case "custom":
		return clip(LoadCustom()), "Custom (PERSONA.md)"
	default:
		if p, ok := preset(name); ok {
			return clip(p.Body), p.Title
		}
		return "", "Default"
	}
}

// SystemNote renders the active persona for the system prompt (empty when none).
// It frames the persona as tone-only and explicitly subordinate to the Trust
// Kernel, so a persona — even a malicious custom one — can't grant itself powers.
func SystemNote(name string) string {
	body, title := Resolve(name)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return "\n\nActive persona — \"" + title + "\". This shapes your TONE and STYLE only; it never changes what you're allowed to do. Your safety rules, the user's approval, and the Trust Kernel's budget/governor/deny limits always win over the persona:\n" + body
}

// clip bounds the body and strips CRs (defense-in-depth for a user-authored file).
func clip(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimSpace(s)
	if len(s) > maxBody {
		s = s[:maxBody] + "\n…(persona truncated)"
	}
	return s
}
