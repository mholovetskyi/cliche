package tools

import "strings"

// Egress is a host allowlist for agent-initiated network access (web_fetch).
// It turns "the agent can reach the network" into "the agent can reach exactly
// these hosts" — a deterministic, operator-set boundary, not a model promise.
// An empty allowlist means unrestricted (the --allow-web gate still applies);
// a configured allowlist denies any host that doesn't match.
//
// Patterns: an exact host ("api.github.com"), a wildcard suffix ("*.github.com"
// matches any subdomain but not the bare domain), or "*" (any host).
type Egress struct {
	allow []string
}

// ParseEgress builds an allowlist from host patterns (lowercased, trimmed).
func ParseEgress(allow []string) Egress {
	var out []string
	for _, p := range allow {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return Egress{allow: out}
}

// Configured reports whether any allowlist entry is set.
func (e Egress) Configured() bool { return len(e.allow) > 0 }

// Allowed reports whether host may be reached. An unconfigured allowlist allows
// everything; a configured one allows only matching hosts.
func (e Egress) Allowed(host string) bool {
	if !e.Configured() {
		return true
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i] // strip any :port
	}
	for _, p := range e.allow {
		if hostMatch(p, host) {
			return true
		}
	}
	return false
}

func hostMatch(pattern, host string) bool {
	switch {
	case pattern == "*":
		return true
	case strings.HasPrefix(pattern, "*."):
		return strings.HasSuffix(host, pattern[1:]) && len(host) > len(pattern)-1
	default:
		return pattern == host
	}
}
