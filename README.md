# Cliche

**The AI coding agent you can actually leave running.**

Cliche wraps any model in a deterministic **Trust Kernel**: a hard token cap
(with an estimated dollar cap on top), a loop circuit-breaker, an append-only
cost ledger, and a reward-hacking verifier. All on by default. Open source.
Auditable to the token.

> Every other coding CLI competes on capability. Cliche rides the same frontier
> models you already use — bring your own key — and competes on the thing none
> of them ship: guardrails the model **cannot** argue its way past, because
> they're code wrapped around the loop, not a prompt the model can ignore.

This is **v0**: the deterministic core is real, tested, and runnable today. The
real-model path does **multi-turn tool use** (read/write files, run commands)
via Anthropic, and the **Verifier independently re-runs your tests**. See
[ROADMAP.md](ROADMAP.md) for what's next and [why it exists](docs/landing.md).

---

## Why

The category is racing toward agents you start and walk away from — async, in
CI, many at once. The harness wasn't built for that:

- A documented runaway ran **809 turns / ~$438** with no circuit breaker.
- A single review spiraled **$0.10 → $7.59** in an 8.5M-token loop.
- Agents quietly **delete tests** and **swallow errors** to make the bar go green.

None of these are model problems. They're harness problems. Cliche is a harness
built for the part where you're not watching.

---

## Install

```sh
go install github.com/mholovetskyi/cliche/cmd/cliche@latest
```

Or build from source (Go 1.23+):

```sh
git clone https://github.com/mholovetskyi/cliche
cd cliche
go build -o cliche ./cmd/cliche
```

---

## Quickstart

**See the Trust Kernel work, offline, in 30 seconds:**

```sh
cliche demo
```

It runs four deterministic scenarios — a healthy task that completes cleanly, a
runaway loop the Governor halts, a budget blowout caught mid-stream, and a diff
where the agent deleted a test (flagged). The numbers printed are real program
output.

**Cook interactively (AI-first session, BYO key):**

```sh
export ANTHROPIC_API_KEY=sk-...
cliche chat
```

Type a task and Cliche works it end-to-end — reading, editing (`edit_file`),
running commands, and delegating isolated subtasks to **budget-scoped
subagents** (one at a time, or several in **parallel**) — streaming each step
live, then waits for your next message (the conversation and the session-wide
budget persist; every subagent's spend is drawn from, and bounded by, that same
session cap). In a terminal,
writes and commands prompt `y/N/always` unless you pass `--allow-write` /
`--allow-run` / `--yolo`. Slash commands: `/cost`, `/clear`, `/verify`,
`/help`, `/exit`.

**One-shot run (scriptable):**

```sh
cliche run --max-usd 0.50 --allow-write --verify "fix the failing test in ./api"
```

`--verify` re-runs your tests after the agent finishes and attaches a verdict.

**Use it headless in CI:**

```sh
git diff | cliche exec -p "review this change" --max-usd 0.10
```

`exec` emits JSON and returns clean exit codes:

| Code | Meaning |
|------|---------|
| `0`  | completed |
| `1`  | error (e.g. missing key) |
| `2`  | bad usage |
| `3`  | budget cap reached |
| `4`  | a governor breaker tripped |

**Independently verify a change (the keystone):**

`verify` re-runs your project's tests itself and combines the result with
reward-hacking detectors over the diff — so a "tests pass" claim is checked, not
trusted:

```sh
git diff | cliche verify --claim-pass
```

It auto-detects the test command (`go test ./...`, `pytest`, `npm test`,
`cargo test`) or reads a `## verify` / `test:` line from `AGENTS.md`. Exit
codes: `0` verified, `5` flagged, `0`/`2` unverified (with `--strict`).

**See where the money went:**

```sh
cliche cost
```

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
              │   provider.Complete  (BYO model)              │
              │        │                                      │
              │        ▼                                      │
              │  Budget.Record     ─ ACTUAL usage, mid-stream │
              │  Governor.RecordToolCall ─ repetition guard   │
              │  Governor.RecordEdit ─ failed-edit breaker    │
              │  Ledger.Append     ─ append-only audit trail  │
              └──────────────────────────────────────────────┘
```

The Trust Kernel is deterministic: **no LLM is ever in the limit, accounting,
or verdict path.** A `--yolo` flag can skip approvals, but it can **never**
bypass the Budget Kernel or the Governor. That is the brand.

Package layout:

| Package | Role |
|---------|------|
| `internal/budget`   | token-hard + dollar-estimated spend caps (two-gate enforcement) |
| `internal/governor` | loop / runaway breakers (turns, wall-clock, repetition, failed edits, no-progress) |
| `internal/ledger`   | append-only JSONL audit trail + summary |
| `internal/verifier` | deterministic reward-hacking detectors over a diff |
| `internal/provider` | model-agnostic interface + Anthropic + offline Mock |
| `internal/tools`    | tool execution behind a graduated permission gate |
| `internal/agent`    | the loop wiring it all together |
| `internal/pricing`  | maintained per-model price table (conservative, high fallback) |
| `internal/config`   | `.cliche/config.json` over safe defaults + `AGENTS.md` detection |
| `internal/cli`      | zero-dependency command dispatcher |

---

## Configuration

Drop a `.cliche/config.json` in your project root to set defaults (flags
override per-run):

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
  }
}
```

Config is validated on load — a 0/negative cap or a window smaller than the
repetition threshold fails loudly rather than silently disarming a guardrail.
See [docs/config.example.json](docs/config.example.json) for the full shape.

Cliche reads `AGENTS.md` (and falls back to `CLAUDE.md` / `GEMINI.md`) for
project context — the cross-tool standard, including an optional `## verify` /
`test:` line that sets the Verifier's test command.

**MCP servers:** list Model Context Protocol servers under the `mcp` config
array (`{name, command, args}`); their tools appear to the agent as
`mcp__<server>__<tool>`, are permission-gated (`--allow-mcp` or approval), and
are governed by the same caps, governor, and ledger as built-in tools.

**Safety defaults:** file tools are confined to the `--dir` project root (no
reading/writing outside it without `--allow-outside-root`); writes, commands,
and MCP calls are off until allowed or approved; transient API errors (429/5xx)
are retried with backoff; a stalled MCP server can't hang a run (calls respect
cancellation); and `Ctrl-C` cancels the current task with a structured outcome
rather than a hard kill.

---

## Honest non-claims

- The **token cap is the hard guarantee**; the **dollar cap is an estimate**
  derived from a maintained price table (rounded conservatively, with a high
  unknown-model fallback).
- The **Verifier catches documented patterns** and honest mistakes, and can
  independently re-run your tests. It is **not** a security boundary against an
  adversary who knows the rules.
- The real-model loop does **multi-turn tool use** (read/edit/write/run, gated
  by permissions). `edit_file` matches exact → whitespace-tolerant → a
  confidence-scored fuzzy anchor (which refuses single-line or anchor-less
  matches, because false edits are worse than no edit), and edits are
  **syntax-validated before writing** (Go via go/parser, JSON via encoding/json;
  other languages: no-op for now).
- The **Context Ledger** keeps the transcript bounded and recoverable; it
  compacts only at safe task boundaries (within one long task the budget cap
  governs) and never silently — every compaction is logged and undoable.

We'd rather state the limits than oversell them. See [SECURITY.md](SECURITY.md).

---

## Development

```sh
go vet ./...
go test ./...
go build -o cliche ./cmd/cliche
```

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

---

## License

[Apache-2.0](LICENSE). The kernel and CLI are fully open: a trust tool you
can't read isn't one.
