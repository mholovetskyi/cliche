package cli

import (
	"regexp"
	"strings"

	"github.com/mholovetskyi/cliche/internal/style"
)

// Minimal, dependency-free Markdown rendering for assistant responses in the
// terminal: fenced code blocks get a gradient left-border, headings/bold/inline
// code are styled, and bullets get a coral glyph. Terminals soft-wrap long
// lines, so no manual wrapping is done (which would mismeasure width once ANSI
// escapes are inserted). Plain text is returned unchanged when styling is off.

var (
	mdInlineCode = regexp.MustCompile("`([^`]+)`")
	mdBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
)

// renderMarkdown styles a markdown string for the terminal.
func renderMarkdown(s string) string {
	if !style.Enabled {
		return s
	}
	var b strings.Builder
	inFence := false
	bar := style.Color(gl("▌", "|"), style.Sample(0.5))
	for _, ln := range strings.Split(s, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue // hide the ``` marker; the border delineates the block
		}
		if inFence {
			b.WriteString("  " + bar + " " + style.White(ln) + "\n")
			continue
		}
		switch {
		case strings.HasPrefix(t, "### "):
			b.WriteString(style.Bold(strings.TrimPrefix(t, "### ")) + "\n")
		case strings.HasPrefix(t, "## "):
			b.WriteString(style.BoldWhite(strings.TrimPrefix(t, "## ")) + "\n")
		case strings.HasPrefix(t, "# "):
			b.WriteString(style.GradientBold(strings.TrimPrefix(t, "# ")) + "\n")
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			indent := ln[:len(ln)-len(strings.TrimLeft(ln, " "))]
			b.WriteString(indent + style.Red(gl("•", "-")) + " " + inlineMarkdown(t[2:]) + "\n")
		default:
			b.WriteString(inlineMarkdown(ln) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// inlineMarkdown styles inline spans: `code` in coral, **bold** in bold white.
// Code is resolved first so ** inside a code span isn't treated as bold.
func inlineMarkdown(s string) string {
	s = mdInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		return style.Red(strings.Trim(m, "`"))
	})
	s = mdBold.ReplaceAllStringFunc(s, func(m string) string {
		return style.BoldWhite(strings.Trim(m, "*"))
	})
	return s
}
