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

// banner is the interactive-session header, echoing the landing page's
// dictionary-entry motif in red/black/white.
func banner() string {
	var b strings.Builder
	b.WriteString("\n  " + wordmark() + "\n")
	b.WriteString("  " + style.Gray("cli·ché  /ˈklē-ˌshā/  noun · the ") + style.Red("loop breaker") + "\n")
	b.WriteString("  " + style.White("the AI coding agent you can actually leave running.") + "\n")
	b.WriteString("  " + style.Gray("trust kernel · on by default · auditable to the token") + "\n")
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
