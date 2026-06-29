package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mholovetskyi/cliche/internal/persona"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdPersona manages the agent personality (Hermes-style): list the built-in
// presets, set the active one, or point at PERSONA.md for a custom voice. A
// persona shapes tone/style only — the Trust Kernel is unaffected.
func cmdPersona(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		printPersonas(out)
		return 0
	}
	switch args[0] {
	case "show":
		name := persona.Active()
		body, title := persona.Resolve(name)
		if name == "" {
			name = "default"
		}
		fmt.Fprintf(out, "Active persona: %s\n", style.BoldWhite(title))
		if strings.TrimSpace(body) != "" {
			fmt.Fprintln(out, style.Gray(body))
		}
		return 0
	case "edit", "path":
		home, err := secrets.ConfigHome()
		if err != nil {
			fmt.Fprintln(errOut, "persona: "+err.Error())
			return 1
		}
		fmt.Fprintf(out, "Write your own voice here, then `cliche persona custom`:\n  %s\n", home+"/PERSONA.md")
		return 0
	default:
		name := args[0]
		if err := persona.SetActive(name); err != nil {
			fmt.Fprintf(errOut, "persona: %q is not a known preset (try `cliche persona`), or `custom` with no PERSONA.md written\n", name)
			return 1
		}
		_, title := persona.Resolve(name)
		fmt.Fprintf(out, "  persona → %s\n", style.BoldWhite(title))
		return 0
	}
}

func printPersonas(out io.Writer) {
	active := persona.Active()
	if active == "" {
		active = "default"
	}
	fmt.Fprintln(out, style.Gray("  personalities — shape the agent's tone/style (the Trust Kernel is unaffected)"))
	fmt.Fprintln(out)
	for _, p := range persona.Presets() {
		mark := "  "
		if p.Name == active {
			mark = style.Red("● ")
		}
		fmt.Fprintf(out, "%s%s %s\n", mark, style.White(fmt.Sprintf("%-10s", p.Name)), style.Gray(p.Desc))
	}
	if persona.HasCustom() {
		mark := "  "
		if active == "custom" {
			mark = style.Red("● ")
		}
		fmt.Fprintf(out, "%s%s %s\n", mark, style.White(fmt.Sprintf("%-10s", "custom")), style.Gray("your PERSONA.md"))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, style.Gray("  set with `cliche persona <name>` · write your own with `cliche persona edit`"))
}
