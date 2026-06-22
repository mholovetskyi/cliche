# AGENTS.md

Project context for AI agents (and humans) working on Cliche itself. Cliche
reads this file as its own project-context standard.

## What this is

Cliche is a trust-first AI coding CLI written in **Go**. It wraps any model in a
deterministic **Trust Kernel** (budget caps + loop breakers + ledger + verifier).
The core principle: **no LLM is ever in the limit, accounting, or verdict path.**
Guardrails are code, not prompts.

## Ground rules for changes

- **Keep the core dependency-free.** The `internal/*` packages and the CLI use
  the Go standard library only. A single static binary with no third-party
  supply-chain surface is a product feature, not an accident. Do not add
  dependencies to the kernel without a very good reason.
- **The kernel must stay deterministic and testable without a network.** Any new
  guardrail needs unit tests that prove it trips (and does not false-trip).
- **`--yolo` may skip approvals but must NEVER bypass the Budget Kernel or the
  Governor.** This invariant is the brand.
- **Be honest in user-facing copy.** The token cap is a hard guarantee; the
  dollar cap is an estimate; the Verifier catches documented patterns, not
  adversaries. Don't let marketing language outrun what the code guarantees.

## Build / test

```sh
go vet ./...
go test ./...
go build -o cliche ./cmd/cliche
./cliche demo   # end-to-end smoke test, offline
```

## Layout

- `cmd/cliche` — entrypoint (thin).
- `internal/budget`, `internal/governor`, `internal/ledger`, `internal/verifier`
  — the deterministic Trust Kernel. Start here.
- `internal/provider` — model backends (`anthropic.go`, `mock.go`).
- `internal/agent` — the loop that wires the kernel around a provider.
- `internal/cli` — command dispatch (`run`, `exec`, `demo`, `cost`).

## Verify-rules extension (forward-looking)

Cliche intends to standardize honesty rules as an `AGENTS.md` extension rather
than inventing a new dotfile. A `## verify` section here is the planned home for
project-specific reward-hacking rules and the test command the Verifier should
re-run. Not wired up in v0 — see ROADMAP.md.
