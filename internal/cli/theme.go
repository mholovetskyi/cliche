package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdThemes lists the available UI palettes with a colored swatch, marking the
// active one. Set a theme with CLICHE_THEME=<name> or "theme" in config.
func cmdThemes(_ []string, out, _ io.Writer) int {
	fmt.Fprintln(out, "\n  "+style.BoldWhite("themes")+style.Gray("  ·  CLICHE_THEME=<name> or \"theme\" in .cliche/config.json"))
	for _, name := range style.ThemeNames() {
		mark := "  "
		if name == style.CurrentTheme {
			mark = style.Green(gl("✓", "ok")) + " "
		}
		fmt.Fprintf(out, "  %s%s  %s\n", mark, style.White(fmt.Sprintf("%-8s", name)), style.ThemeSwatch(name))
	}
	return 0
}

// themeCmd (/theme [name]) shows the palette list or switches it live.
func (s *session) themeCmd(line string) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		fmt.Fprintln(s.out, "  "+style.Gray("theme: ")+style.White(style.CurrentTheme))
		for _, n := range style.ThemeNames() {
			fmt.Fprintf(s.out, "    %s  %s\n", style.White(style.Pad(n, 8)), style.ThemeSwatch(n))
		}
		fmt.Fprintln(s.out, "  "+style.Gray("switch with /theme <name>"))
		return
	}
	if style.ApplyTheme(fields[1]) {
		fmt.Fprintf(s.out, "  theme → %s  %s\n", style.White(fields[1]), style.ThemeSwatch(fields[1]))
	} else {
		fmt.Fprintf(s.out, "  unknown theme %q — try /theme\n", fields[1])
	}
}
