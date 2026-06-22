package tools

import (
	"os"
	"strings"
)

// Sandbox mode (Policy.Sandbox) is Cliche's application-level isolation posture:
// a single switch that composes the hard boundaries into a locked-down default,
// for running a task you don't fully trust. It enforces, regardless of the other
// flags:
//
//   - Confinement: file access stays within the project root (it overrides
//     --allow-outside-root).
//   - Network deny-by-default: web_fetch is blocked unless an egress allowlist
//     names the host (no allowlist ⇒ no network at all).
//   - Secret scrubbing: shell commands run with the operator's model-provider
//     credentials removed from their environment, so a model-authored command
//     can't read or exfiltrate the API key Cliche itself uses.
//
// This is enforced in user space (the same layer as confinement and the
// permission gate), not by the kernel. A kernel-level sandbox (landlock/seccomp
// on Linux, sandbox-exec on macOS, Job Objects on Windows) is the intended
// defense-in-depth layer beneath this; it is deliberately NOT claimed here,
// because doing it correctly is per-platform and would either pull in a
// non-stdlib dependency (breaking Cliche's zero-dependency guarantee) or ship
// untested syscall code. Honesty about the boundary is part of the trust model.

// scrubbedEnv returns the process environment with model-provider credentials
// removed — used for shell commands under sandbox mode.
func scrubbedEnv() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		if isSecretEnvKey(kv[:eq]) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// isSecretEnvKey reports whether an environment key holds a credential Cliche
// should not hand to a model-authored shell command. Sandbox is an untrusted
// posture, so this errs toward over-scrubbing: anything that looks like a key,
// token, secret, password, or credential is stripped (covers OPENAI_KEY,
// *_API_KEY, AWS_ACCESS_KEY_ID, *_TOKEN, *SECRET*, etc.).
func isSecretEnvKey(key string) bool {
	k := strings.ToUpper(key)
	switch {
	case strings.HasPrefix(k, "CLICHE_"):
		return true
	case strings.HasSuffix(k, "_KEY") || strings.Contains(k, "_API_KEY") || strings.Contains(k, "ACCESS_KEY"):
		return true
	case strings.Contains(k, "TOKEN"):
		return true
	case strings.Contains(k, "SECRET"):
		return true
	case strings.Contains(k, "PASSWORD") || strings.Contains(k, "PASSWD"):
		return true
	case strings.Contains(k, "CREDENTIAL"):
		return true
	default:
		return false
	}
}
