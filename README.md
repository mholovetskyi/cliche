<div align="center">

```
 ██████╗██╗     ██╗ ██████╗██╗  ██╗███████╗
██╔════╝██║     ██║██╔════╝██║  ██║██╔════╝
██║     ██║     ██║██║     ███████║█████╗
██║     ██║     ██║██║     ██╔══██║██╔══╝
╚██████╗███████╗██║╚██████╗██║  ██║███████╗
 ╚═════╝╚══════╝╚═╝ ╚═════╝╚═╝  ╚═╝╚══════╝
```

### the AI coding agent you can actually leave running

Cliche rides the same frontier models you already use — **bring your own key** — and wraps the agent loop in a deterministic **Trust Kernel**: hard spend caps, a runaway breaker, an append-only audit ledger, and a verifier that re-runs your tests. All on by default.

[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Dependencies](https://img.shields.io/badge/dependencies-zero-success)](go.mod)
[![Releases](https://img.shields.io/badge/releases-signed%20%2B%20SBOM-8957e5)](https://github.com/mholovetskyi/cliche/releases)

</div>

---

> Every other coding CLI competes on capability. Cliche competes on the thing none of them ship: **guardrails the model cannot argue its way past** — because they're code wrapped around the loop, not a prompt the model can ignore. A `--yolo` flag may skip approvals, but it can **never** bypass the budget cap, the governor, a deny rule, plan mode, the egress allowlist, or a pre-tool-use hook.

## Why it exists

The category is racing toward agents you start and walk away from — async, in CI, many at once. The harness wasn't built for that:

- A documented runaway ran **809 turns / ~$438** with no circuit breaker.
- A single review spiraled **$0.10 → $7.59** in an 8.5M-token loop.
- Agents quietly **delete tests** and **swallow errors** to make the bar go green.

None of these are model problems. They're harness problems. **Cliche is the harness for the part where you're not watching.**

---

## Install

```sh
# Homebrew (macOS / Linux)
brew install mholovetskyi/tap/cliche

# Scoop (Windows)
scoop bucket add mholovetskyi https://github.com/mholovetskyi/scoop-bucket
scoop install cliche

# winget (Windows)
winget install mholovetskyi.cliche

# Go
go install github.com/mholovetskyi/cliche/cmd/cliche@latest
```

Or grab a signed binary from [Releases](https://github.com/mholovetskyi/cliche/releases) — one static binary per platform, plus `checksums.txt`, an SBOM, and a keyless cosign signature so you can verify the supply chain end to end ([how](SECURITY.md#verify-your-download)). Or build from source (Go 1.23+):

```sh
git clone https://github.com/mholovetskyi/cliche && cd cliche
go build -o cliche ./cmd/cliche
```

---

## 60-second tour

**See the Trust Kernel fire — offline, no key:**

```sh
cliche demo
```

Four deterministic scenarios: a clean task, a runaway the Governor halts, a budget blowout caught mid-stream, and a diff where the agent deleted a test (flagged). Every number printed is real program output.

**Connect a provider and start cooking:**

```sh
cliche login      # pick a provider, paste your key — verified & saved (0600, never in the repo)
cliche chat
```

A session opens on a framed input bar with your trust state always in view:

```
  ╭─ suggest · gpt-4o-mini · $0.0000 · ctx 0% ───────────────────╮
  │ ❯ refactor the auth handler and @internal/auth/session.go
```

Type and it works end-to-end — reads, edits, runs commands, streams every step live (with **Go syntax-highlighted** code), then waits for your next message. Writes and commands prompt `y/N/always` with a colored diff preview, unless you raise the permission mode.

**One-shot, scriptable:**

```sh
cliche run --max-usd 0.50 --mode auto-edit --branch --verify "fix the failing test in ./api"
```

**Fan out a swarm** — a planner splits the task, executors work it in parallel, a synthesizer combines the results. The whole fleet runs under **one shared budget cap, one ledger, one permission gate** — the Trust Kernel wraps the swarm, not just one agent:

```sh
cliche swarm --max-usd 1.00 --mode auto-edit "add table-driven tests to every package in ./internal"
```

**Works on any project — auth once, run anywhere:**

```sh
cliche login                      # key saved globally (once, in your user config dir)
cd ~/work/some-existing-repo && cliche chat      # operates on THIS repo
cliche chat --dir ~/work/other-repo              # …or target one from anywhere
cliche new my-app                 # scaffold + register a fresh project
cliche projects                   # every project you've used Cliche in, most-recent first
```

Each project keeps its own `.cliche/` (config, ledger, sessions) — like `.git`, so its audit trail and budget history stay local. Credentials are the only thing that's global.

**Headless in CI** (JSON out, clean exit codes):

```sh
git diff | cliche exec -p "review this change" --max-usd 0.10
```

| Code | Meaning |  | Code | Meaning |
|--|--|--|--|--|
| `0` | completed |  | `3` | budget cap reached |
| `1` | error |  | `4` | a governor breaker tripped |
| `2` | bad usage |  | `5` | completed but **verifier flagged** |

---

## What's in the session

A modern terminal cockpit, all zero-dependency Go:

| | |
|--|--|
| **Framed input bar** | a bordered prompt carrying live mode · model · spend · context; the chevron turns red as you near a cap |
| **`@file` includes** | `@path/to/file.go` inlines the file for the model — confined to the project root, no read round-trip |
| **Permission modes** | `plan` (read-only) · `suggest` (asks) · `auto-edit` (auto edits, asks commands) · `full` (auto) — shown as a colored badge |
| **Live activity feed** | every tool call named with its target; quiet ✓/loud ✗ results; a phase-narrating spinner |
| **Multi-session** | `/sessions` to list, `/new` to branch off, `/resume` to pick up where you left — persisted to disk |
| **Trust at a glance** | `/status` (budget, context, governor caps, standing grants) · `/rules` (allow/deny, egress, hooks) |
| **Plan surface** | `/plan`, `/tasks`, `/done` — a lightweight to-do list that survives resume |
| **Review & undo** | `/diff`, `/undo` and `/rewind` with a rollback preview, `/commit` to git |
| **Models** | `/models` lists the priced catalog; `/model <id>` hops models mid-chat |

Unambiguous abbreviations just work (`/s` → `/status`, `/di` → `/diff`); end a line with `\` to compose a multi-line prompt.

---

## Bring any model

Cliche is provider-neutral and **auto-detects the backend** from whichever key you have. Built-in presets:

| Provider | Notes |
|---|---|
| **Anthropic** | native Messages API, prompt caching |
| **OpenRouter** | one key, hundreds of models |
| **OpenAI · Google (Gemini) · xAI · DeepSeek · Mistral · Cohere · Perplexity · Moonshot · Zhipu · GitHub Models** | first-party, OpenAI-compatible |
| **Groq · Cerebras · Together · Fireworks · DeepInfra · NVIDIA · SambaNova · Hyperbolic · Novita** | fast open-weight inference clouds |
| **Ollama · LM Studio · vLLM** | local servers — **no API key required**, built-in presets |
| **Anything else** | any OpenAI-compatible endpoint — add under `providers` in config or pass `--base-url` |

~25 providers are built in: `cliche login` lists them, or just `--provider <name>` (the matching `*_API_KEY` env var works too). OAuth connectors like GitHub: `cliche connect github`.

`cliche login` walks you through it; the matching `*_API_KEY` env var always overrides a saved key. Route delegated subtasks to a cheaper model with `subagents.model`.

---

## How it works

```
              ┌──────────────────────────────────────────────┐
  prompt ───► │                  agent loop                   │
              │                                               │
              │  Governor.BeginTurn ─ max turns / wall clock  │
              │  Budget.Preflight  ─ estimate before the turn │
              │        │                                      │
              │        ▼                                      │
              │   provider.Complete  (BYO model, streamed)    │
              │        │                                      │
              │        ▼                                      │
              │  Budget.Record     ─ ACTUAL usage, mid-stream │
              │  Governor.RecordToolCall ─ repetition guard   │
              │  Governor.RecordEdit ─ failed-edit breaker    │
              │  Ledger.Append     ─ append-only audit trail  │
              └──────────────────────────────────────────────┘
```

The Trust Kernel is deterministic: **no LLM is ever in the limit, accounting, or verdict path.**

| Package | Role |
|---------|------|
| `internal/budget` | token-hard + dollar-estimated spend caps (two-gate enforcement) |
| `internal/governor` | runaway breakers (turns, wall-clock, repetition, failed edits, no-progress) |
| `internal/ledger` | append-only JSONL audit trail + summary, **hash-chained & verifiable** (`cliche audit`) |
| `internal/verifier` | deterministic reward-hacking detectors + independent test re-run |
| `internal/provider` | model-agnostic interface — Anthropic, OpenAI-compatible, offline Mock |
| `internal/tools` | tool execution behind a graduated permission gate + path confinement |
| `internal/agent` | the loop wiring it all together |
| `internal/style` | the Width-aware terminal UI toolkit (gradients, gauges, boxes) |

---

## The moat: sandbox, egress, hooks

Beyond approvals, Cliche can enforce policy the model can't reach:

- **`--sandbox`** — confines file access to the project root, **denies network** unless allowlisted, and **scrubs secrets** (`*_KEY`, `*TOKEN`, …) from the environment of shell commands.
- **Egress allowlist** — `egress.allow` host patterns are re-checked on **every redirect hop** (no SSRF past the allowlist).
- **Allow/deny rules** — `Tool(pattern)` policy-as-code; **deny wins** over allow and over `--yolo`.
- **Hooks** — a `pre_tool_use` command can block any tool call (non-zero exit fails **closed**); a `stop` hook observes completion. Policy you write, enforced by a program.
- **MCP** — stdio and HTTP Model Context Protocol servers; their tools are permission-gated and governed by the same caps, governor, and ledger as built-ins.
- **Tamper-evident, signed ledger** — every audit entry is SHA-256 hash-chained to the one before it, and the chain head is **ed25519-signed** with a per-user key (stored 0600 in your config dir, never in the project). `cliche audit` re-verifies the chain (flagging any **altered, deleted, reordered, or inserted** record) *and* the signature — so an attacker who can edit a project's ledger but can't read your key can be detected (exit 5), not just suspected. The audit trail is verifiable, not just append-only.

---

## Configuration

Drop a `.cliche/config.json` in your project root (flags override per-run):

```json
{
  "model": "claude-sonnet-4-6",
  "budget": { "max_tokens": 2000000, "max_usd": 5.0 },
  "governor": {
    "max_turns": 50,
    "max_wallclock_seconds": 1800,
    "max_consecutive_failed_edits": 5,
    "repetition_window": 8,
    "repetition_threshold": 3,
    "no_progress_turns": 12
  },
  "permissions": { "allow": ["Read(**)"], "deny": ["Bash(rm -rf *)"] },
  "egress": { "allow": ["api.github.com", "*.openai.com"] }
}
```

Config is validated on load — a 0/negative cap, or a window smaller than the repetition threshold, fails loudly rather than silently disarming a guardrail. Cliche also reads `AGENTS.md` (falling back to `CLAUDE.md` / `GEMINI.md`) for project context, including a `## verify` / `test:` line that sets the Verifier's command. Full shape: [docs/config.example.json](docs/config.example.json).

---

## The keystone: independent verification

`verify` re-runs your project's tests **itself** and combines the result with reward-hacking detectors over the diff — so a "tests pass" claim is checked, not trusted:

```sh
git diff | cliche verify --claim-pass
```

It auto-detects the test command (`go test ./...`, `pytest`, `npm test`, `cargo test`) or reads it from `AGENTS.md`. Exit codes: `0` verified, `5` flagged, `0`/`2` unverified (with `--strict`).

---

## Honest non-claims

- The **token cap is the hard guarantee**; the **dollar cap is an estimate** from a maintained price table (rounded conservatively, with a high unknown-model fallback).
- The **Verifier catches documented patterns** and honest mistakes and can independently re-run your tests. It is **not** a security boundary against an adversary who knows the rules.
- The **sandbox is app-level** confinement (root confinement, network-deny, credential scrubbing), not a kernel jail — deliberately, to stay zero-dependency.
- The **Context Ledger** keeps the transcript bounded and recoverable; it compacts only at safe task boundaries, never silently — every compaction is logged and undoable.

We'd rather state the limits than oversell them. See [SECURITY.md](SECURITY.md).

---

## Development

```sh
go vet ./...
go test ./...          # 18 packages, zero third-party deps
go build -o cliche ./cmd/cliche
```

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

---

## For teams

The CLI is free and Apache-2.0, forever. Once more than one person is running
agents against a shared codebase — and someone is accountable for what they do
— the same guardrails go org-wide: **push a signed policy** every developer's
kernel enforces (tighten-only, `--yolo` still can't bypass it), **aggregate
every signed ledger** into one tamper-evident audit, and **govern spend** across
the team. That's the commercial layer; the kernel underneath stays open.

See [COMMERCIAL.md](COMMERCIAL.md) for tiers, or run `cliche org` to connect.
Sponsor the open-source core via the Sponsor button.

---

## License

[Apache-2.0](LICENSE). The kernel and CLI are fully open — a trust tool you can't read isn't one.
