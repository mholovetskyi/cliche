# Contributing to Cliche

Thanks for considering a contribution. Cliche is open core (Apache-2.0) and the
kernel is meant to be read and trusted.

## Before you start

- Read [AGENTS.md](AGENTS.md) for the non-negotiable invariants (dependency-free
  core, deterministic kernel, `--yolo` never bypasses caps/governor, honest
  copy).
- Open an issue to discuss anything larger than a bug fix.

## Workflow

1. Fork and branch from `main`.
2. Make your change. Add or update tests — the kernel must prove it trips and
   does not false-trip.
3. Run the full local check:
   ```sh
   gofmt -l .        # should print nothing
   go vet ./...
   go test ./...
   go build ./...
   ./cliche demo     # offline end-to-end smoke test
   ```
4. Open a PR with a clear description of the behavior change.

## Style

- Standard library only in `internal/*` and the CLI unless discussed first.
- Keep it `gofmt`-clean and `go vet`-clean.
- Match the surrounding code: small packages, clear names, comments that explain
  *why*, not *what*.

## Good first issues

The biggest open v0 gaps are in [ROADMAP.md](ROADMAP.md): the reliable diff
engine, the Verifier's independent test re-run, and the Context Ledger. Each is
self-contained and high-impact.
