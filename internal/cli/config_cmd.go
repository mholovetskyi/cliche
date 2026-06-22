package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/mholovetskyi/cliche/internal/config"
)

// cmdConfig prints the effective configuration for a project (defaults merged
// with .cliche/config.json) and validates it, so a user can see exactly what
// guardrails are in force and catch a disarming mistake before a run.
func cmdConfig(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*dir)
	if err != nil {
		fmt.Fprintln(errOut, "config: "+err.Error())
		return 1
	}
	if name, ok := config.HasAgentsFile(*dir); ok {
		fmt.Fprintf(out, "// project context: %s\n", name)
	}
	writeJSON(out, cfg)

	if verr := cfg.Validate(); verr != nil {
		fmt.Fprintln(errOut, "config is INVALID: "+verr.Error())
		return 2
	}
	fmt.Fprintln(out, "config is valid.")
	return 0
}
