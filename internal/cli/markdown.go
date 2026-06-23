package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

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

// mdState carries the cross-line markdown state: whether we're inside a fenced
// code block and, if so, its language (so a ```go block can be syntax-lit).
type mdState struct {
	inFence bool
	lang    string
}

// renderMarkdown styles a whole markdown string for the terminal.
func renderMarkdown(s string) string {
	if !style.Enabled {
		return s
	}
	var b strings.Builder
	var st mdState
	for _, ln := range strings.Split(s, "\n") {
		if rendered, emit := renderMdLine(ln, &st); emit {
			b.WriteString(rendered + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderMdLine renders one markdown line, advancing *st across calls. It returns
// the styled line (no trailing newline) and whether to emit it at all — fence
// ``` markers are suppressed (the border delineates the block instead).
func renderMdLine(ln string, st *mdState) (string, bool) {
	if !style.Enabled {
		return ln, true
	}
	bar := style.Color(gl("▌", "|"), style.Sample(0.5))
	t := strings.TrimSpace(ln)
	if strings.HasPrefix(t, "```") {
		st.inFence = !st.inFence
		if st.inFence {
			st.lang = strings.TrimSpace(strings.TrimPrefix(t, "```"))
			if st.lang != "" {
				return bar + " " + style.Dim(st.lang), true // surface the language label
			}
		} else {
			st.lang = ""
		}
		return "", false // hide the bare marker
	}
	if st.inFence {
		body := style.White(ln)
		if st.lang == "go" {
			body = highlightGo(ln) // light syntax color for Go (the project's language)
		}
		return bar + " " + body, true
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
	out  io.Writer
	st   mdState
	line strings.Builder
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
	if rendered, ok := renderMdLine(line, &m.st); ok {
		fmt.Fprintln(m.out, "  "+rendered) // gutter indent, aligned with the feed
	}
}

// goKeywords are the reserved words (plus a few canonical builtins/values) the
// Go highlighter tints. A fixed set keeps highlightGo a parser-free, line-local
// tokenizer rather than dragging in go/scanner.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
	"nil": true, "true": true, "false": true, "iota": true,
}

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// highlightGo applies a minimal, single-line Go tokenizer to a code-fence body:
// keywords bold, string/rune literals coral, // and (single-line) /* */ comments
// gray, everything else white. It coalesces adjacent same-color runs so plain
// stretches stay contiguous, and only adds color — never changing display width.
// Deliberately line-local (no cross-line block-comment state) and called only
// when styling is on.
func highlightGo(line string) string {
	const (
		plain = iota
		keyword
		literal
		comment
	)
	type seg struct {
		text string
		kind int
	}
	var segs []seg
	add := func(text string, kind int) {
		if text == "" {
			return
		}
		if n := len(segs); n > 0 && segs[n-1].kind == kind {
			segs[n-1].text += text // coalesce same-color runs
		} else {
			segs = append(segs, seg{text, kind})
		}
	}
	var word strings.Builder
	flushWord := func() {
		w := word.String()
		if w == "" {
			return
		}
		if goKeywords[w] {
			add(w, keyword)
		} else {
			add(w, plain)
		}
		word.Reset()
	}

	rs := []rune(line)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == '/' && i+1 < len(rs) && rs[i+1] == '/': // line comment to EOL
			flushWord()
			add(string(rs[i:]), comment)
			i = len(rs)
		case r == '/' && i+1 < len(rs) && rs[i+1] == '*': // block comment (line-local)
			flushWord()
			j := i + 2
			for j+1 < len(rs) && !(rs[j] == '*' && rs[j+1] == '/') {
				j++
			}
			end := j + 2
			if end > len(rs) {
				end = len(rs)
			}
			add(string(rs[i:end]), comment)
			i = end
		case r == '"' || r == '`' || r == '\'': // string / rune literal
			flushWord()
			quote := r
			j := i + 1
			for j < len(rs) {
				if rs[j] == '\\' && quote != '`' && j+1 < len(rs) {
					j += 2 // skip an escape (raw `…` strings don't escape)
					continue
				}
				if rs[j] == quote {
					j++
					break
				}
				j++
			}
			add(string(rs[i:j]), literal)
			i = j
		case isWordRune(r):
			word.WriteRune(r)
			i++
		default:
			flushWord()
			add(string(r), plain)
			i++
		}
	}
	flushWord()

	var b strings.Builder
	for _, s := range segs {
		switch s.kind {
		case keyword:
			b.WriteString(style.Bold(s.text))
		case literal:
			b.WriteString(style.Red(s.text))
		case comment:
			b.WriteString(style.Gray(s.text))
		default:
			b.WriteString(style.White(s.text))
		}
	}
	return b.String()
}
