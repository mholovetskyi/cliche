package cli

import (
	"strings"

	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

// wordmark renders "cliché" with the brand's white "clich" + red "é".
func wordmark() string {
	return style.Red(gl("⬡", "*")) + " " + style.BoldWhite("clich") + style.BoldRed("é")
}

// gradientWordmark renders the hexagon + "cliché" with the brand gradient sweep.
func gradientWordmark() string {
	return style.Color(gl("⬡", "*"), style.Sample(0)) + " " + style.GradientBold("cliché")
}

// banner is the interactive-session header: a gradient wordmark and rule over
// the dictionary-entry motif, with a coral gradient ribbon down the left.
func banner() string {
	lines := []string{
		gradientWordmark(),
		style.GradientRule(44),
		style.Gray("cli·ché  /ˈklē-ˌshā/  noun · the ") + style.Red("loop breaker"),
		style.White("the AI coding agent you can actually leave running."),
		style.Gray("trust kernel · on by default · auditable to the token"),
	}
	var b strings.Builder
	b.WriteByte('\n')
	last := len(lines) - 1
	for i, ln := range lines {
		ribbon := style.Color(gl("▌", " "), style.Sample(float64(i)/float64(last)))
		b.WriteString("  " + ribbon + "  " + ln + "\n")
	}
	return b.String()
}

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
