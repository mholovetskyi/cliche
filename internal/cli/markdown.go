package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/mholovetskyi/cliche/internal/style"
)

// Minimal, dependency-free Markdown rendering for assistant responses in the
// terminal: fenced code blocks get a gradient left-border and a language label,
// headings/bold/inline code are styled, and bullets get a coral glyph. The
// per-line renderer carries fence state, so the SAME logic renders a whole
// buffered reply (renderMarkdown) or a live-streamed one line-at-a-time
// (mdStreamer) — the latter never moves the cursor backward, so it is safe even
// when a line soft-wraps. Plain text is returned unchanged when styling is off.

var (
	mdInlineCode = regexp.MustCompile("`([^`]+)`")
	mdBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
)

// renderMarkdown styles a whole markdown string for the terminal.
func renderMarkdown(s string) string {
	if !style.Enabled {
		return s
	}
	var b strings.Builder
	inFence := false
	for _, ln := range strings.Split(s, "\n") {
		if rendered, emit := renderMdLine(ln, &inFence); emit {
			b.WriteString(rendered + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderMdLine renders one markdown line, toggling *inFence across calls. It
// returns the styled line (no trailing newline) and whether to emit it at all —
// fence ``` markers are suppressed (the border delineates the block instead).
func renderMdLine(ln string, inFence *bool) (string, bool) {
	if !style.Enabled {
		return ln, true
	}
	bar := style.Color(gl("▌", "|"), style.Sample(0.5))
	t := strings.TrimSpace(ln)
	if strings.HasPrefix(t, "```") {
		*inFence = !*inFence
		if *inFence {
			if lang := strings.TrimSpace(strings.TrimPrefix(t, "```")); lang != "" {
				return bar + " " + style.Dim(lang), true // surface the language label
			}
		}
		return "", false // hide the bare marker
	}
	if *inFence {
		return bar + " " + style.White(ln), true
	}
	switch {
	case strings.HasPrefix(t, "### "):
		return style.Bold(strings.TrimPrefix(t, "### ")), true
	case strings.HasPrefix(t, "## "):
		return style.BoldWhite(strings.TrimPrefix(t, "## ")), true
	case strings.HasPrefix(t, "# "):
		return style.GradientBold(strings.TrimPrefix(t, "# ")), true
	case strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "):
		indent := ln[:len(ln)-len(strings.TrimLeft(ln, " "))]
		return indent + style.Red(gl("•", "-")) + " " + inlineMarkdown(t[2:]), true
	default:
		return inlineMarkdown(ln), true
	}
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

// mdStreamer renders streamed assistant text incrementally: it buffers the
// current line and, when the line completes, prints it (gutter-indented) through
// renderMdLine. A trailing partial line is rendered on flush(). It never erases,
// so it can't corrupt scrollback when a line wraps — the safe streaming path.
type mdStreamer struct {
	out     io.Writer
	inFence bool
	line    strings.Builder
}

func newMdStreamer(out io.Writer) *mdStreamer { return &mdStreamer{out: out} }

func (m *mdStreamer) write(text string) {
	for {
		nl := strings.IndexByte(text, '\n')
		if nl < 0 {
			m.line.WriteString(text)
			return
		}
		m.line.WriteString(text[:nl])
		m.emit()
		text = text[nl+1:]
	}
}

func (m *mdStreamer) flush() {
	if m.line.Len() > 0 {
		m.emit()
	}
}

func (m *mdStreamer) emit() {
	line := m.line.String()
	m.line.Reset()
	if rendered, ok := renderMdLine(line, &m.inFence); ok {
		fmt.Fprintln(m.out, "  "+rendered) // gutter indent, aligned with the feed
	}
}
