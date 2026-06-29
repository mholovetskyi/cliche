// Package workspace models Cliché Studio's two organizing concepts:
//
//   - a Project is a physical folder under the workspace root that holds its own
//     chats (.cliche/sessions) and apps — like a Claude Project.
//   - an App is a buildable/previewable folder: a static page (index.html) or a
//     live dev app (package.json with a dev/start/serve script — Vite/Next/CRA).
//     Apps live in a project, or directly in the workspace root.
//
// Everything here is pure filesystem inspection — zero dependencies.
package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Project is a folder under the workspace root.
type Project struct {
	Name  string `json:"name"`
	Path  string `json:"path"` // absolute
	Apps  int    `json:"apps"`
	Chats int    `json:"chats"`
}

// App is a buildable folder.
type App struct {
	Name   string `json:"name"`
	Rel    string `json:"rel"`    // relative to the scope root ("." = the root itself)
	Kind   string `json:"kind"`   // "static" | "dev"
	Script string `json:"script"` // dev command, for dev apps
}

func skip(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == "build"
}

// Projects lists the immediate subfolders of the workspace root, each annotated
// with how many apps and chats it contains. Sorted by name.
func Projects(ws string) []Project {
	entries, err := os.ReadDir(ws)
	if err != nil {
		return nil
	}
	out := []Project{}
	for _, e := range entries {
		if !e.IsDir() || skip(e.Name()) {
			continue
		}
		dir := filepath.Join(ws, e.Name())
		out = append(out, Project{Name: e.Name(), Path: dir, Apps: len(Apps(dir)), Chats: countChats(dir)})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// Apps lists the buildable apps under root: the root itself if it's an app, plus
// any immediate subfolder that is one. Sorted with the root first, then by name.
func Apps(root string) []App {
	out := []App{}
	if a, ok := appAt(root, "."); ok {
		out = append(out, a)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	var subs []App
	for _, e := range entries {
		if !e.IsDir() || skip(e.Name()) {
			continue
		}
		if a, ok := appAt(filepath.Join(root, e.Name()), e.Name()); ok {
			subs = append(subs, a)
		}
	}
	sort.Slice(subs, func(i, j int) bool { return strings.ToLower(subs[i].Name) < strings.ToLower(subs[j].Name) })
	return append(out, subs...)
}

// appAt classifies dir as a dev app (package.json dev script), a static app
// (index.html), or neither. A dev script wins (it's the richer experience).
func appAt(dir, rel string) (App, bool) {
	name := filepath.Base(dir)
	if rel == "." {
		name = "this folder"
	}
	if s, ok := devScript(dir); ok {
		return App{Name: name, Rel: rel, Kind: "dev", Script: s}, true
	}
	if isFile(filepath.Join(dir, "index.html")) {
		return App{Name: name, Rel: rel, Kind: "static"}, true
	}
	return App{}, false
}

func devScript(dir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return "", false
	}
	for _, n := range []string{"dev", "start", "serve"} {
		if _, has := pkg.Scripts[n]; has {
			return "npm run " + n, true
		}
	}
	return "", false
}

// countChats counts saved sessions in a project's .cliche/sessions.
func countChats(dir string) int {
	entries, err := os.ReadDir(filepath.Join(dir, ".cliche", "sessions"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
