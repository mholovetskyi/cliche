package cli

import (
	"fmt"
	"strings"

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

// eBlockCol is the column (0-based) where the final E block begins. The wordmark
// is C-L-I-C-H-E with widths 8+8+3+8+8(+8), so "CLICH" occupies columns 0-34 and
// the E spans 35-42. The hero forces that E brand-red — mirroring the red "é" in
// the inline wordmark — so the eye reads CLICHÉ even where the accent stroke is
// unavailable (NO_COLOR), instead of an ambiguous CLICHE.
const eBlockCol = 35

// heroLogo renders the ANSI-Shadow "CLICHÉ" block. "CLICH" sweeps on a diagonal
// (Gradient2D) so the stacked rows read as one corner-to-corner coral sheen, the
// final E is forced brand-red, and an acute accent rides above it. The accent is
// routed through gl() so a dumb terminal degrades the box-drawing stroke to an
// ASCII tick rather than emitting an unrenderable glyph.
func heroLogo() string {
	var b strings.Builder
	rows := len(clicheLetters)
	b.WriteString("  " + style.Red(strings.Repeat(" ", accentCol)+gl("╱╱", "'")) + "\n")
	for i, row := range clicheLetters {
		rs := []rune(row)
		for len(rs) < artWidth {
			rs = append(rs, ' ')
		}
		head := style.Gradient2D(string(rs[:eBlockCol]), i, rows)
		eBlock := style.Red(string(rs[eBlockCol:]))
		b.WriteString("  " + head + eBlock + "\n")
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

// loginBanner renders the full hero wordmark + a "connect a provider" frame for
// the `cliche login` wizard. The gradient rules give the provider list a clean
// panel feel without needing to know the terminal width.
func loginBanner() string {
	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString(heroLogo())
	b.WriteByte('\n')
	b.WriteString("  " + style.GradientRule(artWidth) + "\n")
	b.WriteString("  " + style.White("connect a model provider") + "\n")
	b.WriteString("  " + style.Gray("BYO-key · stored locally (0600) · never sent anywhere but the provider") + "\n")
	b.WriteString("  " + style.GradientRule(artWidth) + "\n")
	return b.String()
}

// compactHeader returns a three-line framed identity panel for the chat session
// start: a gradient rule, the wordmark + a mode badge + session metadata, and a
// closing rule. The mode rides in a colored badge (not gray text) because it is
// the active guardrail and should be the most scannable thing in the strip. The
// full ASCII hero is reserved for the first-run splash / bare `cliche`.
func compactHeader(provider, model, mode, keySrc string) string {
	var b strings.Builder
	badge := style.Badge(strings.ToUpper(mode), style.InkRGB, modeColor(mode))
	meta := style.Gray(provider+" · "+model) + style.Dim(" · key "+keySrc)
	rule := "  " + style.GradientRule(artWidth)
	b.WriteString(rule + "\n")
	b.WriteString("  " + gradientWordmark() + "   " + badge + "  " + meta + "\n")
	b.WriteString(rule)
	return b.String()
}

// modeColor escalates the mode badge's fill with how much rope the mode grants:
// plan (gray, read-only) → suggest (white, asks first) → auto-edit (coral) →
// full (red, auto-approves). The same ladder tints the prompt chevron.
func modeColor(mode string) style.RGB {
	switch mode {
	case modePlan:
		return style.GrayRGB
	case modeAutoEdit:
		return style.Sample(0.5)
	case modeFull:
		return style.RedRGB
	default: // suggest
		return style.WhiteRGB
	}
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
