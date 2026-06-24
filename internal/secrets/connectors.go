package secrets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Connector tokens (from the OAuth device flow) live in connectors.json in the
// user config dir, 0600 — never in a project, so they're not committed and are
// available across every project. Same posture as the credentials file.

// ConnectorToken is a stored connector grant.
type ConnectorToken struct {
	Token string `json:"token"`
	Type  string `json:"type,omitempty"` // e.g. "bearer"
}

func connectorsPath() (string, error) {
	d, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "connectors.json"), nil
}

func loadConnectors() (map[string]ConnectorToken, error) {
	m := map[string]ConnectorToken{}
	p, err := connectorsPath()
	if err != nil {
		return m, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return m, err
	}
	_ = json.Unmarshal(data, &m)
	return m, nil
}

// SaveConnector persists (or replaces) a connector's token.
func SaveConnector(name string, t ConnectorToken) error {
	m, _ := loadConnectors()
	m[name] = t
	d, err := baseDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	p, _ := connectorsPath()
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// Connector returns a stored connector token (ok=false if not connected).
func Connector(name string) (ConnectorToken, bool) {
	m, _ := loadConnectors()
	t, ok := m[name]
	return t, ok
}

// ConnectedNames lists the connectors that have a stored token, sorted.
func ConnectedNames() []string {
	m, _ := loadConnectors()
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// DeleteConnector removes a stored connector (disconnect). A missing one is fine.
func DeleteConnector(name string) error {
	m, _ := loadConnectors()
	if _, ok := m[name]; !ok {
		return nil
	}
	delete(m, name)
	data, _ := json.MarshalIndent(m, "", "  ")
	p, err := connectorsPath()
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o600)
}
