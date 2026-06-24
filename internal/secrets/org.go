package secrets

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Org connects the CLI to a Cliche control plane (the commercial Team tier): the
// policy endpoint, the bearer token that proves an active subscription, and the
// pinned ed25519 public key the org's policy is verified against. It lives in
// org.json in the user config dir, 0600 — global, like credentials, so one
// connection governs every project. The token is sensitive; the URL and key
// are not, but they ride along here so a single file fully describes the link.
type OrgConfig struct {
	URL   string `json:"url"`   // policy endpoint, e.g. https://cp.example.com/policy
	Token string `json:"token"` // bearer token (subscription proof)
	Key   string `json:"key"`   // pinned org public key, "ed25519:<base64>"
}

func orgPath() (string, error) {
	d, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "org.json"), nil
}

// SaveOrg persists the control-plane connection.
func SaveOrg(c OrgConfig) error {
	d, err := baseDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	p, _ := orgPath()
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// Org returns the stored control-plane connection (ok=false if not connected).
func Org() (OrgConfig, bool) {
	p, err := orgPath()
	if err != nil {
		return OrgConfig{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return OrgConfig{}, false
	}
	var c OrgConfig
	if json.Unmarshal(data, &c) != nil || c.URL == "" || c.Token == "" || c.Key == "" {
		return OrgConfig{}, false
	}
	return c, true
}

// DeleteOrg disconnects from the control plane.
func DeleteOrg() error {
	p, err := orgPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
