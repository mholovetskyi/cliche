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
| **OpenAI** · **Groq** · **DeepSeek** · **Mistral** · **Together** · **xAI** | OpenAI-compatible |
| **Any OpenAI-compatible / local** | Ollama, LM Studio, vLLM — add under `providers` in config |

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
| `internal/ledger` | append-only JSONL audit trail + summary |
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

## License

[Apache-2.0](LICENSE). The kernel and CLI are fully open — a trust tool you can't read isn't one.
