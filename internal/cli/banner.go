package cli

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

// clicheLetters is the ANSI-Shadow block wordmark C-L-I-C-H-E (each letter
// 8 cols wide except I at 3 → 43 cols total). The é acute accent is added over
// the final E at render time (see splash). Every row is padded to a uniform
// rune width so a per-row gradient sweep lines up column-for-column into one
// coherent vertical coral band.
var clicheLetters = []string{
	" ██████╗██╗     ██╗ ██████╗██╗  ██╗███████╗",
	"██╔════╝██║     ██║██╔════╝██║  ██║██╔════╝",
	"██║     ██║     ██║██║     ███████║█████╗  ",
	"██║     ██║     ██║██║     ██╔══██║██╔══╝  ",
	"╚██████╗███████╗██║╚██████╗██║  ██║███████╗",
	" ╚═════╝╚══════╝╚═╝ ╚═════╝╚═╝  ╚═╝╚══════╝",
}

// accentCol is the column (0-based) of the é acute accent — centered over the
// final E block, which spans columns 35-42.
const accentCol = 39

// heroLogo renders the gradient ANSI-Shadow "CLICHÉ" block (with the é accent),
// every row padded to a uniform width so the per-row sweep forms one coherent
// vertical coral band. Each line is prefixed with a two-space indent.
func heroLogo() string {
	art := append([]string{strings.Repeat(" ", accentCol) + "╱╱"}, clicheLetters...)
	width := 0
	for _, row := range art {
		if n := utf8.RuneCountInString(row); n > width {
			width = n
		}
	}
	var b strings.Builder
	for _, row := range art {
		if pad := width - utf8.RuneCountInString(row); pad > 0 {
			row += strings.Repeat(" ", pad)
		}
		b.WriteString("  " + style.Gradient(row) + "\n")
	}
	return b.String()
}

// heroHeader is the shared hero: the block wordmark over the dictionary motif,
// a gradient rule, and the taglines. Used by both the first-run splash and the
// interactive session header.
func heroHeader() string {
	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString(heroLogo())
	b.WriteString("\n")
	b.WriteString("  " + style.Color(gl("⬡", "*"), style.Sample(0)) +
		style.Gray(" cli·ché  /ˈklē-ˌshā/  noun · the ") + style.Red("loop breaker") + "\n")
	b.WriteString("  " + style.GradientRule(45) + "\n")
	b.WriteString("  " + style.White("the AI coding agent you can actually leave running.") + "\n")
	b.WriteString("  " + style.Gray("trust kernel · on by default · auditable to the token") + "\n")
	return b.String()
}

// splash is the first-run hero for a bare `cliche`: the shared header plus a
// get-started command palette.
func splash() string {
	var b strings.Builder
	b.WriteString(heroHeader())
	b.WriteString("\n")
	cmds := []struct{ name, desc string }{
		{"login", "connect your model key — BYO, never marked up"},
		{"chat", "start a session with the trust kernel on"},
		{"demo", "watch the guardrails fire — no key, no network"},
	}
	for i, c := range cmds {
		bar := style.Color(gl("▌", " "), style.Sample(float64(i)/float64(len(cmds)-1)))
		b.WriteString("  " + bar + " " + style.BoldWhite(fmt.Sprintf("%-7s", c.name)) + style.Gray(c.desc) + "\n")
	}
	b.WriteString("\n  " + style.Gray("get started ›  ") + style.White("cliche login · chat · demo") + "\n")
	return b.String()
}

// wordmark renders "cliché" with the brand's white "clich" + red "é".
func wordmark() string {
	return style.Red(gl("⬡", "*")) + " " + style.BoldWhite("clich") + style.BoldRed("é")
}

// gradientWordmark renders the hexagon + "cliché" with the brand gradient sweep.
func gradientWordmark() string {
	return style.Color(gl("⬡", "*"), style.Sample(0)) + " " + style.GradientBold("cliché")
}

// banner is the interactive-session header — the same impressive hero as the
// first-run splash (the session loop adds the provider/model + slash lines).
func banner() string { return heroHeader() }

// verdictStyled colors a verify verdict: verified=white, flagged=red.
func verdictStyled(status string) string {
	switch status {
	case verifier.StatusVerified:
		return style.BoldWhite("verdict: verified")
	case verifier.StatusFlagged:
		return style.BoldRed("verdict: flagged")
	default:
		return style.Gray("verdict: " + status)
	}
}
