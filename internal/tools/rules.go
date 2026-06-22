package tools

import (
	"fmt"
	"strings"
)

// Permission rules are deterministic, code-evaluated allow/deny patterns that
// pre-authorize safe actions and hard-block dangerous ones — the practical
// bridge between "ask about everything" and "leave it running", and policy as
// CODE (not a prompt the model can argue past). Rule syntax is `Tool(pattern)`:
//
//	Read(pattern)  Write(pattern)  Edit(pattern)   — pattern is a path glob (** spans dirs)
//	Bash(pattern)  (alias: Run)                    — pattern is a command glob (* = any run)
//
// Evaluation: DENY wins over ALLOW, and a deny overrides even --yolo (a deny
// rule is a guarantee). A matching allow pre-authorizes (skips the prompt). No
// match falls through to the mode/approver/Policy flow.

type ruleAction int

const (
	ruleNone ruleAction = iota
	ruleAllow
	ruleDeny
)

type rule struct {
	tool    string // "read" | "write" | "edit" | "run"
	pattern string
}

// Rules is a parsed allow/deny rule set.
type Rules struct {
	allow []rule
	deny  []rule
}

// Empty reports whether no rules are configured.
func (r Rules) Empty() bool { return len(r.allow) == 0 && len(r.deny) == 0 }

func normalizeRuleTool(t string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "read":
		return "read", true
	case "write":
		return "write", true
	case "edit":
		return "edit", true
	case "bash", "run":
		return "run", true
	}
	return "", false
}

func parseRule(s string) (rule, error) {
	s = strings.TrimSpace(s)
	open := strings.Index(s, "(")
	if open < 0 || !strings.HasSuffix(s, ")") {
		return rule{}, fmt.Errorf("rule %q must be Tool(pattern), e.g. Bash(go test *) or Read(**/.env)", s)
	}
	tool, ok := normalizeRuleTool(s[:open])
	if !ok {
		return rule{}, fmt.Errorf("rule %q: unknown tool (want Read|Write|Edit|Bash)", s)
	}
	pattern := s[open+1 : len(s)-1]
	if pattern == "" {
		return rule{}, fmt.Errorf("rule %q: empty pattern", s)
	}
	return rule{tool: tool, pattern: pattern}, nil
}

// ParseRules parses allow/deny rule strings, returning the first parse error.
func ParseRules(allow, deny []string) (Rules, error) {
	var r Rules
	for _, s := range allow {
		ru, err := parseRule(s)
		if err != nil {
			return Rules{}, err
		}
		r.allow = append(r.allow, ru)
	}
	for _, s := range deny {
		ru, err := parseRule(s)
		if err != nil {
			return Rules{}, err
		}
		r.deny = append(r.deny, ru)
	}
	return r, nil
}

// Decision evaluates the rules for a tool category ("read"/"write"/"edit"/"run")
// against a target (a file path or a command). Deny wins over allow.
func (r Rules) Decision(category, target string) ruleAction {
	if matchAny(r.deny, category, target) {
		return ruleDeny
	}
	if matchAny(r.allow, category, target) {
		return ruleAllow
	}
	return ruleNone
}

func matchAny(rules []rule, category, target string) bool {
	for _, ru := range rules {
		if ru.tool != category {
			continue
		}
		if category == "run" {
			if wildcardMatch(ru.pattern, strings.TrimSpace(target)) {
				return true
			}
		} else if pathMatchesGlob(ru.pattern, normalizePath(target)) {
			return true
		}
	}
	return false
}

func normalizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.TrimPrefix(p, "./")
}

// wildcardMatch matches a command against a glob where '*' is any run of
// characters (segments must appear in order; a leading/trailing '*' anchors
// loosely).
func wildcardMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(s, p)
		if i < 0 {
			return false
		}
		s = s[i+len(p):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}
