package cli

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/web"
)

// toMsgs flattens an agent transcript into the conversation rows the web UI
// renders: user prompts, assistant replies, and a compact "ran X" line for tool
// calls. Pure tool-result turns are dropped — the live feed already showed them.
func toMsgs(msgs []provider.Message) []web.Msg {
	var out []web.Msg
	for _, m := range msgs {
		switch m.Role {
		case "user":
			if strings.TrimSpace(m.Text) != "" {
				out = append(out, web.Msg{Role: "user", Text: m.Text})
			}
		case "assistant":
			if strings.TrimSpace(m.Text) != "" {
				out = append(out, web.Msg{Role: "assistant", Text: m.Text})
			}
			if len(m.ToolCalls) > 0 {
				names := make([]string, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					names = append(names, tc.Name)
				}
				out = append(out, web.Msg{Role: "tool", Text: strings.Join(names, ", ")})
			}
		}
	}
	return out
}

// titleFrom turns a prompt into a short, single-line session title.
func titleFrom(s string) string {
	s = strings.TrimSpace(firstLine(s))
	if r := []rune(s); len(r) > 60 {
		s = strings.TrimSpace(string(r[:60])) + "…"
	}
	if s == "" {
		s = "New chat"
	}
	return s
}

// deriveTitle picks a session title from its first user message.
func deriveTitle(msgs []provider.Message) string {
	for _, m := range msgs {
		if m.Role == "user" && strings.TrimSpace(m.Text) != "" {
			return titleFrom(m.Text)
		}
	}
	return "New chat"
}

// fileTreeSkip are directories left out of the workspace tree — internal state,
// VCS, and dependency/build noise.
var fileTreeSkip = map[string]bool{".git": true, ".cliche": true, "node_modules": true, "dist": true, ".idea": true, ".vscode": true}

// fileTree builds the project file tree shown in the workspace, directories
// first then files, each alphabetical. Depth-bounded so a deep tree can't hang.
func fileTree(root string) []web.FileNode {
	return readDirNodes(root, "", 0)
}

func readDirNodes(root, rel string, depth int) []web.FileNode {
	if depth > 8 {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	var dirs, files []web.FileNode
	for _, e := range entries {
		name := e.Name()
		if fileTreeSkip[name] {
			continue
		}
		if !e.IsDir() && web.IsSensitiveFile(name) {
			continue // keep secrets out of the workspace tree
		}
		childRel := path.Join(rel, name)
		if e.IsDir() {
			dirs = append(dirs, web.FileNode{Name: name, Path: childRel, Dir: true, Children: readDirNodes(root, childRel, depth+1)})
		} else {
			files = append(files, web.FileNode{Name: name, Path: childRel, Dir: false})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return append(dirs, files...)
}

// readProjectFile returns a file's contents for the viewer, strictly confined to
// the project root (no traversal, no absolute paths) and capped in size.
func readProjectFile(root, rel string) (string, bool) {
	if rel == "" {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", false
	}
	if web.IsSensitiveFile(filepath.Base(clean)) {
		return "", false // never read a secret file into the viewer
	}
	full := filepath.Join(root, clean)
	if rp, err := filepath.Rel(root, full); err != nil || rp == ".." || strings.HasPrefix(rp, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() || info.Size() > 512*1024 {
		return "", false
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", false
	}
	return string(data), true
}
