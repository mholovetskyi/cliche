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

// artWidth is the uniform rune width of every wordmark row (8+8+3+8+8+8).
const artWidth = 43

// accentCol is the column (0-based) where the é acute accent sits — centered
// over the final E block, which spans columns 35-42.
const accentCol = 38

// heroLogo renders the ANSI-Shadow "CLICHÉ" block. The letters sweep on a
// diagonal (Gradient2D) so the stacked rows read as one corner-to-corner coral
// sheen, and the acute accent over the final E is forced brand-RED — so the
// wordmark's most distinctive element finally reads as "cliché".
func heroLogo() string {
	var b strings.Builder
	b.WriteString("  " + style.Red(strings.Repeat(" ", accentCol)+"╱╱") + "\n")
	for i, row := range clicheLetters {
		if pad := artWidth - utf8.RuneCountInString(row); pad > 0 {
			row += strings.Repeat(" ", pad)
		}
		b.WriteString("  " + style.Gradient2D(row, i, len(clicheLetters)) + "\n")
	}
	return b.String()
}

// heroHeader is the shared hero: the block wordmark over the dictionary motif,
// a gradient rule, and the taglines. Reserved for the first-run splash / bare
// `cliche` (an everyday session opens with the compact header instead).
func heroHeader() string {
	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString(heroLogo())
	b.WriteString("\n")
	b.WriteString("  " + style.Color(gl("⬡", "*"), style.Sample(0)) +
		style.Gray(" cli·ché  noun · the ") + style.Red("loop breaker") + "\n")
	b.WriteString("  " + style.GradientRule(artWidth) + "\n")
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

// compactHeader is the one-line interactive-session identity: the gradient
// wordmark plus a metadata strip. The full ASCII hero is reserved for the
// first-run splash / bare `cliche`, so an everyday session opens with the prompt
// near the top of the screen instead of behind 13 lines of wallpaper.
func compactHeader(provider, model, mode, keySrc string) string {
	meta := style.Gray(provider+" · "+model+" · "+mode) + style.Dim(" · key "+keySrc)
	return "  " + gradientWordmark() + "   " + meta
}

// verdictStyled renders a verify verdict with an icon AND a color, so the
// safety-critical "flagged" survives NO_COLOR / piping (uppercased, not just
// red) rather than relying on color alone.
func verdictStyled(status string) string {
	switch status {
	case verifier.StatusVerified:
		return style.BoldGreen(gl("✓ ", "") + "verdict: verified")
	case verifier.StatusFlagged:
		return style.BoldRed(gl("✗ ", "") + "verdict: FLAGGED")
	default:
		return style.Gray(gl("• ", "") + "verdict: " + status)
	}
}
