package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/tools"
)

// maxIncludeBytes caps how much of a single @-referenced file is inlined into the
// prompt, so an accidental @huge.log can't blow the context window.
const maxIncludeBytes = 50 * 1024

// maxAttachmentBytes caps a single attached image/document, so @huge.pdf can't
// bloat a request.
const maxAttachmentBytes = 10 << 20 // 10 MiB

// attachmentMediaType returns the IANA type for an attachable file's extension
// (images + PDF), or "" if it is not a recognized attachment (so it falls
// through to text inlining).
func attachmentMediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	}
	return ""
}

// fileRefRe matches an @-prefixed path token (e.g. @internal/cli/session.go). It
// requires at least one non-space, non-@ char after the @, so a bare "@" or an
// email's "@host" only *matches* — actual inclusion still requires the path to
// resolve to a real file inside the project, so prose and addresses are left as
// literal text.
var fileRefRe = regexp.MustCompile(`@([^\s@]+)`)

// expandFileRefs inlines the contents of @-referenced files into the prompt sent
// to the model — so "@main.go what's wrong here?" skips a read_file round-trip
// and matches the muscle memory of peer CLIs. Each token is resolved with the
// executor's confinement rules (no escaping the project root); tokens that don't
// resolve to a readable file are left untouched (treated as literal text). The
// user's typed line is preserved verbatim as the message body (and remains the
// session title), so only the model sees the attached bodies. Image references
// (@photo.png) are attached as vision images instead of inlined text. Returns the
// prompt to send, styled notes, and any attached images.
func (s *session) expandFileRefs(line string) (string, []string, []provider.Image) {
	matches := fileRefRe.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return line, nil, nil
	}
	var preamble strings.Builder
	var notes []string
	var images []provider.Image
	seen := map[string]bool{}
	for _, m := range matches {
		ref := m[1]
		if seen[ref] {
			continue
		}
		seen[ref] = true

		abs, err := tools.ResolveWithin(s.dir, ref)
		if err != nil {
			notes = append(notes, style.Red(gl("⚠", "!")+" @"+ref)+style.Gray(" — outside the project root; left as text"))
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue // not a real file → an @mention, not a path; leave it literal
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			notes = append(notes, style.Red(gl("⚠", "!")+" @"+ref)+style.Gray(" — unreadable; left as text"))
			continue
		}
		// An image or PDF attaches for a vision/document model rather than inlining.
		if mt := attachmentMediaType(ref); mt != "" {
			if len(data) > maxAttachmentBytes {
				notes = append(notes, style.Red(gl("⚠", "!")+" @"+ref)+style.Gray(" — over 10 MiB; skipped"))
				continue
			}
			images = append(images, provider.Image{MediaType: mt, Data: data})
			kind := "image"
			if mt == "application/pdf" {
				kind = "document"
			}
			notes = append(notes, style.Gray(fmt.Sprintf("%s @%s · %s attached (needs a vision/document model)", gl("🖼", "+"), ref, kind)))
			continue
		}
		truncated := false
		if len(data) > maxIncludeBytes {
			data = data[:maxIncludeBytes]
			truncated = true
		}
		preamble.WriteString("--- " + ref + " ---\n")
		preamble.Write(data)
		if len(data) == 0 || data[len(data)-1] != '\n' {
			preamble.WriteByte('\n')
		}
		if truncated {
			preamble.WriteString("… [truncated]\n")
		}
		preamble.WriteByte('\n')

		lines := strings.Count(string(data), "\n") + 1
		note := style.Gray(fmt.Sprintf("%s @%s · %d line(s)", gl("⊕", "+"), ref, lines))
		if truncated {
			note += style.Gray(" (truncated)")
		}
		notes = append(notes, note)
	}
	if preamble.Len() == 0 {
		return line, notes, images // nothing inlined (maybe images were attached)
	}
	prompt := "The user attached these files with @ (their message follows):\n\n" + preamble.String() + line
	return prompt, notes, images
}
