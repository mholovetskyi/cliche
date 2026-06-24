// Package projects is a global registry of the project directories Cliche has
// been used in, so they're discoverable across sessions (`cliche projects`) and
// new ones can be scaffolded (`cliche new`). It lives in the user config dir
// alongside credentials. Work is still per-folder — this is only the index;
// every project keeps its own .cliche/ (config, ledger, sessions).
package projects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Project is one tracked project root.
type Project struct {
	Path     string    `json:"path"` // absolute path; the registry key
	Name     string    `json:"name"`
	LastUsed time.Time `json:"last_used"`
}

// Registry is the cross-project index plus an optional workspace root that
// `cliche new` scaffolds into by default.
type Registry struct {
	Workspace string    `json:"workspace,omitempty"`
	Projects  []Project `json:"projects"`
}

func file(dir string) string { return filepath.Join(dir, "registry.json") }

// Load reads the registry from dir. A missing file is not an error.
func Load(dir string) (*Registry, error) {
	r := &Registry{}
	data, err := os.ReadFile(file(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}
	if err := json.Unmarshal(data, r); err != nil {
		return r, err
	}
	return r, nil
}

// Save writes the registry to dir (creating it if needed).
func (r *Registry) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file(dir), append(data, '\n'), 0o644)
}

// Upsert records a project by absolute path, bumping LastUsed (and Name if given).
func (r *Registry) Upsert(path, name string, now time.Time) {
	for i := range r.Projects {
		if r.Projects[i].Path == path {
			r.Projects[i].LastUsed = now
			if name != "" {
				r.Projects[i].Name = name
			}
			return
		}
	}
	r.Projects = append(r.Projects, Project{Path: path, Name: name, LastUsed: now})
}

// Remove forgets a project by absolute path or name. Returns true if one matched.
func (r *Registry) Remove(key string) bool {
	for i := range r.Projects {
		if r.Projects[i].Path == key || r.Projects[i].Name == key {
			r.Projects = append(r.Projects[:i], r.Projects[i+1:]...)
			return true
		}
	}
	return false
}

// Recent returns the projects sorted most-recently-used first.
func (r *Registry) Recent() []Project {
	out := append([]Project(nil), r.Projects...)
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsed.After(out[j].LastUsed) })
	return out
}
