package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mholovetskyi/cliche/internal/cli/lineedit"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// switchProvider (/provider [name]) changes the whole backend mid-chat — without
// restarting. With no name it opens the arrow picker over the built-in catalog;
// selecting a provider with no key prompts for one inline (so you never have to
// exit to `cliche login`). The transcript and budget carry over.
func (s *session) switchProvider(line string) {
	if fields := strings.Fields(line); len(fields) >= 2 {
		s.applyProvider(fields[1])
		return
	}
	names := make([]string, 0, len(builtinProviders))
	for n := range builtinProviders {
		names = append(names, n)
	}
	sort.Strings(names)

	items := make([]lineedit.SelectItem, len(names))
	for i, n := range names {
		p := builtinProviders[n]
		status := "needs a key — will prompt"
		switch {
		case p.local:
			status = "local · no key"
		case hasProviderKey(n):
			status = "ready"
		}
		marker := ""
		if n == s.cfg.Provider {
			marker = " ● current"
		}
		items[i] = lineedit.SelectItem{Label: p.label + marker, Desc: status}
	}
	if idx, ok := s.pick("switch provider", items); ok {
		s.applyProvider(names[idx])
		return
	}
	// Fallback (no raw mode): show the current provider + how to switch.
	fmt.Fprintf(s.out, "  provider: %s %s\n", style.White(s.cfg.Provider), style.Gray("· /provider <name> to switch"))
}

// applyProvider switches to the named provider, prompting for a key inline when
// one isn't configured, and resetting to that provider's default model.
func (s *session) applyProvider(name string) {
	info, known := lookupProvider(s.cfg, name)
	if !known {
		fmt.Fprintf(s.out, "  unknown provider %q — see `cliche login` for the catalog\n", name)
		return
	}
	// Inline hidden key entry when the provider needs a key and has none (local
	// servers need none) — so switching never sends you out to `cliche login`.
	if !info.local && !hasProviderKey(name) {
		key, err := readSecret(s.out, s.r, fmt.Sprintf("  paste your %s API key (hidden): ", name))
		if err != nil || strings.TrimSpace(key) == "" {
			fmt.Fprintln(s.out, "  cancelled.")
			return
		}
		if _, err := secrets.Save(name, strings.TrimSpace(key)); err != nil {
			fmt.Fprintln(s.out, "  save failed: "+err.Error())
			return
		}
	}

	// Resolve with the model cleared so we pick the NEW provider's default (the
	// old provider's model id likely isn't valid here); the user can /model after.
	cfg := s.cfg
	cfg.Model = ""
	b, err := resolveBackend(cfg, &runFlags{provider: name, dir: s.dir})
	if err != nil {
		fmt.Fprintln(s.out, "  provider: "+err.Error())
		return
	}
	key, _ := secrets.Lookup(b.name)
	prov, err := buildProvider(b, key)
	if err != nil {
		fmt.Fprintln(s.out, "  provider: "+err.Error())
		return
	}
	s.a.SetProvider(prov, b.model)
	s.cfg.Provider, s.cfg.Model = b.name, b.model
	fmt.Fprintf(s.out, "  %s provider → %s %s\n", style.Green(gl("✓", "ok")), style.White(b.name), style.Gray("· "+b.model))
}
