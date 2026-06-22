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
- **Project-root confinement.** File tools (`read_file`/`write_file`/
  `edit_file`) are confined to the `--dir` project root: absolute paths and `..`
  escapes are rejected, so a prompt-injected agent cannot read `~/.ssh/id_rsa`
  or write outside your project. `run_command` executes with the root as its
  working directory. The escape hatch is an explicit `--allow-outside-root`.
  (Confinement is path-based and does not resolve symlinks, so an in-root
  symlink pointing outside the root is not blocked; OS-sandbox-based
  containment is on the v1 roadmap.)
- **No secrets or raw code in the ledger.** The audit trail records metadata
  only (tokens, cost, event types, verdicts, the tool name + target path or a
  truncated command — never file contents, `old_string`, or keys).
- **Zero third-party dependencies in the core.** A single static binary has a
  minimal supply-chain attack surface. (Roadmap: signed, reproducible releases
  with a published SBOM.)

## Honest non-claims

- The **Verifier** catches *documented* reward-hacking patterns and honest
  mistakes in a diff. It is **not** a security boundary: an adversary who knows
  the rules can evade static detectors (rename, comment, hardcode). Treat its
  verdicts as a strong signal, not a guarantee.
- The **dollar cap is an estimate**; the **token cap is the hard guarantee**.
- v0 confines file *paths* to the project root but does **not** sandbox tool
  execution at the OS level — a permitted `run_command` runs with your full user
  privileges and can reach the network and files outside the root. Shell
  commands and writes are off by default and gated by an explicit flag/approval;
  an OS sandbox (and default-deny egress) is on the v1 roadmap. Until then, run
  untrusted prompts without `--allow-run` / `--allow-write` / `--yolo`.
