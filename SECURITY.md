# Security

Cliche is a trust tool, so we hold ourselves to the bar we sell.

## Reporting a vulnerability

Please report security issues privately via GitHub Security Advisories on this
repository (Security → Report a vulnerability), or by email to the maintainer.
Do not open a public issue for an exploitable vulnerability.

## Design posture

- **BYO-key.** Your model API key is read from the environment
  (`ANTHROPIC_API_KEY`) and sent only to the provider you chose. v0 does not
  persist keys. (Roadmap: OS keychain storage — never plaintext config, never a
  URL.)
- **Local-first.** The cost ledger and verdicts are written under `.cliche/` in
  your project. There is no mandatory cloud upload and no default telemetry.
- **No secrets or raw code in the ledger.** The audit trail records metadata
  only (tokens, cost, event types, verdicts, short truncated details).
- **Zero third-party dependencies in the core.** A single static binary has a
  minimal supply-chain attack surface. (Roadmap: signed, reproducible releases
  with a published SBOM.)

## Honest non-claims

- The **Verifier** catches *documented* reward-hacking patterns and honest
  mistakes in a diff. It is **not** a security boundary: an adversary who knows
  the rules can evade static detectors (rename, comment, hardcode). Treat its
  verdicts as a strong signal, not a guarantee.
- The **dollar cap is an estimate**; the **token cap is the hard guarantee**.
- v0 does **not** sandbox tool execution at the OS level. Shell commands and
  writes are off by default and gated by an explicit flag; an OS sandbox is on
  the v1 roadmap. Until then, run untrusted prompts without `--allow-run` /
  `--allow-write` / `--yolo`.
