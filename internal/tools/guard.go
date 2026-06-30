package tools

import (
	"os"
	"regexp"
	"strings"
)

// maxToolOutputBytes bounds any single tool result before it is fed back to the
// model and recorded. Untrusted sources — an MCP server, a fetched page, a cloned
// site — could otherwise return megabytes that flood the context window and
// inflate the next request's cost. A coding tool's legitimate output (a file, a
// command result) fits well under this; web_fetch/clone already cap lower.
const maxToolOutputBytes = 100_000

// secretPatterns match common credential shapes so a tool can't echo a leaked key
// straight into the model's context (and the ledger). Deliberately broad — this is
// an untrusted-input boundary, so it errs toward over-redaction.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),                                            // OpenAI-style keys
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),                                       // GitHub tokens
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                                                 // AWS access key id
	regexp.MustCompile(`AIza[0-9A-Za-z_\-]{30,}`),                                          // Google API keys
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),                                     // Slack tokens
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{20,}`),                                // Authorization: Bearer …
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),                               // PEM private keys
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`), // JWTs
}

// SanitizeOutput is the untrusted-input boundary of the Trust Kernel: every tool
// result — built-in, MCP, or subagent — passes through here before it reaches the
// model or the ledger. It redacts credential-shaped strings (and the operator's
// own provider keys verbatim) and bounds the size so a hostile or buggy source
// can't exfiltrate a secret in plain sight or flood the context.
func SanitizeOutput(s string) string {
	if s == "" {
		return s
	}
	s = redactSecrets(s)
	if len(s) > maxToolOutputBytes {
		s = s[:maxToolOutputBytes] + "\n…[tool output truncated by the Trust Kernel to bound the context]"
	}
	return s
}

func redactSecrets(s string) string {
	// The operator's actual provider keys are the highest-value target: redact any
	// that appear verbatim (a tool reading the env, a misconfigured echo, etc.).
	for _, v := range operatorSecrets() {
		if len(v) >= 12 {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// operatorSecrets returns the VALUES of credential-shaped environment variables,
// so the agent's own keys can be scrubbed out of any tool output verbatim.
func operatorSecrets() []string {
	var out []string
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		if isSecretEnvKey(kv[:eq]) {
			if v := kv[eq+1:]; len(v) >= 12 {
				out = append(out, v)
			}
		}
	}
	return out
}
