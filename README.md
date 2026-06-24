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

**Cliche** is an open-source, zero-dependency coding agent for your terminal. Bring your own key for any model; Cliche wraps the agent loop in a deterministic **Trust Kernel** — a hard token cap, a runaway circuit-breaker, a reward-hack verifier, and a signed audit ledger. **All on by default.**

[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Dependencies](https://img.shields.io/badge/dependencies-zero-success)](go.mod)
[![Releases](https://img.shields.io/badge/releases-signed%20%2B%20SBOM-8957e5)](https://github.com/mholovetskyi/cliche/releases)
[![Platforms](https://img.shields.io/badge/platforms-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-555)](https://github.com/mholovetskyi/cliche/releases)

[Quickstart](#quickstart) · [Why](#why-it-exists) · [Trust Kernel](#the-trust-kernel) · [Usage](#usage) · [Commands](#command-reference) · [Config](#configuration) · [Models](#bring-any-model) · [Extend](#extend-it) · [For teams](#for-teams)

</div>

---

> Every other coding CLI competes on capability. Cliche competes on the thing none of them ship: **guardrails the model cannot argue its way past** — because they're code wrapped around the loop, not a prompt the model can ignore. A `--yolo` flag may skip approvals, but it can **never** bypass the budget cap, the governor, a deny rule, plan mode, the egress allowlist, or a pre-tool-use hook.

## Why it exists

The category is racing toward agents you start and walk away from — async, in CI, many at once. The harness wasn't built for that:

- A documented runaway ran **809 turns / ~$438** with no circuit breaker.
- A single review spiraled **$0.10 → $7.59** in an 8.5M-token loop.
- Agents quietly **delete tests** and **swallow errors** to make the bar go green — and you find out three tasks later.

None of these are model problems. They're *harness* problems. **Cliche is the harness for the part where you're not watching.**

The guarantees are **deterministic code, not prompts**: no LLM is ever in the limit, accounting, permission, or verdict path. The agent literally cannot talk its way past them.

---

## Table of contents

- [Quickstart](#quickstart)
- [The Trust Kernel](#the-trust-kernel)
- [See it fire](#see-it-fire)
- [Usage](#usage)
- [The session cockpit](#the-session-cockpit)
- [Command reference](#command-reference)
- [Configuration](#configuration)
- [Bring any model](#bring-any-model)
- [Extend it](#extend-it)
- [Trust, honestly](#trust-honestly)
- [For teams](#for-teams)
- [Install](#install)
- [Develop](#develop)
- [License](#license)

---

## Quickstart

```sh
# 1. Install (pick one)
brew install mholovetskyi/tap/cliche                              # macOS / Linux
go install github.com/mholovetskyi/cliche/cmd/cliche@latest       # any platform with Go 1.23+

# 2. See the Trust Kernel fire — offline, no key, no network
cliche demo

# 3. Connect a model and go
cliche login        # pick a provider, paste your key (hidden) — verified & saved 0600, never in the repo
cliche
```

`cliche` with no arguments drops you straight into an interactive session. Type a task; it reads, edits, and runs commands end to end, streaming every step live — and stops the moment a guardrail trips. That's the whole pitch: **start it, and you don't have to hover.**

---

## The Trust Kernel

Four guardrails wrap every run. They are **on by default** and enforced in deterministic Go — the model never sees the levers.

```
              ┌──────────────────────────────────────────────┐
  prompt ───► │                  agent loop                   │
              │                                               │
              │  Governor.BeginTurn ─ max turns / wall clock  │
              │  Budget.Preflight  ─ estimate before the turn │
              │        │                                      │
              │        ▼                                      │
              │   provider.Complete  (your model, streamed)   │
              │        │                                      │
              │        ▼                                      │
              │  Budget.Record     ─ ACTUAL usage, post-turn  │
              │  Governor.RecordToolCall ─ repetition guard   │
              │  Governor.RecordEdit ─ failed-edit breaker    │
              │  Ledger.Append     ─ append-only audit trail  │
              └──────────────────────────────────────────────┘
```

### 1. Budget Kernel — a cap that actually caps
A **hard token ceiling** (the provider-independent guarantee) plus an estimated **dollar ceiling** on top. Checked *before* every turn against an estimate, and again against **actual usage** the moment the turn returns — so the one fat completion that blows the estimate is caught before the next turn fires, not on your invoice. Budgets **nest**: a subagent enforces its own sub-budget *and* every charge bubbles up to the session-wide cap, so a fleet can't outspend the whole.

### 2. Governor — the runaway circuit-breaker
Hard limits on **turns, wall-clock, and consecutive failed edits**, plus **repetition detection** that halts an agent re-issuing the same failing tool call, and a **no-progress** breaker. Every halt is a structured, attributable reason (`turn N: identical tool call repeated 3×`). Strict by default — the runaway that costs other tools $438 stops here in single digits.

### 3. Verifier — catches the agent faking it
Independent reward-hack detection over the diff: it flags **deleted tests**, **swallowed errors** (`except: pass`, empty `catch{}`), and **trivially-true assertions**. As a keystone, it can **re-run your test command itself** and contradict a false "tests pass" claim — it only ever says **verified** when tests were actually re-run and passed on a clean diff. Biased toward "let me check" over false accusations.

### 4. Ledger — auditable to the token
Every turn, tool call, cap event, and verdict is written to an **append-only, SHA-256 hash-chained** log, and the chain head is **ed25519-signed** with a per-user key (stored `0600` in your config dir, never in the repo). `cliche audit` re-verifies the chain and the signature: a ledger **altered, reordered, or with records inserted/removed in the middle** by someone without your signing key is **detected, not just suspected** (exit 5). It's tamper-*evident* — see [Trust, honestly](#trust-honestly) for the precise scope.

> The kernel brand invariant: `--yolo` skips the *approval prompts* and nothing else. It can never raise a cap, silence the governor, override a deny rule, leave plan mode, widen the egress allowlist, or skip a pre-tool-use hook.

---

## See it fire

No key, no network, 30 seconds — every number below is **real program output**, not a mockup:

```sh
cliche demo
```

```
[2] Runaway loop — the agent re-issues the SAME failing edit forever.
    → HALTED at turn 3: identical tool call repeated 3× within the last 3 calls
    → spent ~$0.0738 (15000 tokens) and stopped.
      A documented runaway elsewhere ran 809 turns / ~$438 with no breaker.

[3] Budget blowout — token-heavy turns; the dollar cap is $0.50.
    → HALTED at turn 2: estimated dollar cap reached ~$0.60/$0.50
    → preflight passed, but ACTUAL usage crossed the cap and was caught
      the moment the turn returned — before the next turn could fire.

[4] Reward-hack check — the agent deletes a test to 'pass'.
    → verdict: flagged
      • [deleted_test] a test was removed: func TestChargesCustomer...
```

---

## Usage

### Interactive — `cliche` / `cliche chat`
A session opens on a framed input bar with your trust state always in view:

```
  ╭─ suggest · claude-sonnet-4-6 · $0.0000 · ctx 0% ──────────────╮
  │ ❯ refactor the auth handler and @internal/auth/session.go
```

Type and it works end to end — reads, edits, runs commands, streams every step live with **Go syntax-highlighted** code, then waits for you. Writes and commands prompt `y / N / always` with a colored diff preview, unless you raise the [permission mode](#configuration). Switch models or providers mid-chat (`/model`, `/provider`), connect a tool (`/connect github`) — all without leaving.

### One-shot & scriptable — `cliche run`
```sh
cliche run --max-usd 0.50 --mode auto-edit --branch --verify "fix the failing test in ./api"
```
Works on a fresh git branch, auto-edits, and runs the verifier when done.

### Headless in CI — `cliche exec`
JSON on stdout, **clean exit codes**, fails *loudly* on limits instead of hanging:

```sh
git diff | cliche exec -p "review this change" --max-usd 0.10
```

| Code | Meaning |  | Code | Meaning |
|--|--|--|--|--|
| `0` | completed |  | `3` | budget cap reached |
| `1` | error |  | `4` | a governor breaker tripped |
| `2` | bad usage |  | `5` | completed but **verifier flagged** |

### Fan out — `cliche swarm`
A planner splits the task, executors work it in parallel, a synthesizer combines the results — and the **whole fleet runs under one shared budget cap, one ledger, one permission gate**. The Trust Kernel wraps the swarm, not just one agent:

```sh
cliche swarm --max-usd 1.00 --mode auto-edit "add table-driven tests to every package in ./internal"
```

### Any project — auth once, run anywhere
```sh
cliche login                                # key saved globally, once, in your user config dir
cd ~/work/some-repo && cliche               # operates on THIS repo
cliche chat --dir ~/work/other-repo         # …or target one from anywhere
cliche new my-app                           # scaffold + register a fresh project
cliche projects                             # every project you've used Cliche in, most-recent first
```

Each project keeps its own `.cliche/` (config, ledger, sessions, memory) — like `.git`, so its audit trail and budget history stay local. Credentials are the only thing that's global.

---

## The session cockpit

A modern terminal UI, all zero-dependency Go:

| | |
|--|--|
| **Framed input bar** | a bordered prompt carrying live mode · model · spend · context; the chevron reddens as you near a cap |
| **`@file` includes** | `@path/to/file.go` inlines a file for the model — confined to the project root, no read round-trip |
| **Permission modes** | `plan` (read-only) · `suggest` (asks) · `auto-edit` (auto edits, asks commands) · `full` (auto) — a colored badge, `Shift+Tab` cycles |
| **Live activity feed** | every tool call named with its target; quiet ✓ / loud ✗ results; a phase-narrating spinner |
| **Arrow-key everything** | pick models, themes, sessions, and approvals with ↑/↓ — no typing ids |
| **Full-screen views** | `/tui` live multi-pane chat · `/changes` clickable diff + revert · `/browse` sessions · `/dash` dashboard (mouse + wheel) |
| **Multi-session** | `/sessions`, `/new`, `/fork`, `/resume`, `/kill` — persisted to disk and resumable |
| **Trust at a glance** | `/status` (budget, context, governor caps, grants) · `/rules` (allow/deny, egress, hooks) |
| **Plan surface** | `/plan`, `/tasks`, `/done` — a lightweight to-do list that survives resume |
| **Review & undo** | `/diff`, `/undo`, `/rewind` with a rollback preview, `/commit` to git |

Unambiguous abbreviations just work (`/s` → `/status`); clickable hyperlinks for URLs; end a line with `\` to compose a multi-line prompt.

---

## Command reference

<details>
<summary><b>Commands</b> — <code>cliche &lt;command&gt;</code> (click to expand)</summary>

**Get going**
| Command | What it does |
|---|---|
| `cliche` / `chat` | Interactive agentic session (`--continue` / `--resume <id>` to pick up a prior one) |
| `run "<prompt>"` | One-shot agent run (multi-turn tools) |
| `exec` | Headless: prompt via `-p` or stdin, JSON out, clean exit codes |
| `swarm "<task>"` | Multi-agent run (planner → parallel executors → synthesizer), one shared budget |
| `login` | Interactive provider + key setup (verified, saved `0600`) |
| `auth <provider>` | Save a key non-interactively (scripts/CI); no arg shows status |

**Projects**
| Command | What it does |
|---|---|
| `init` | Scaffold `.cliche/config.json` + an `AGENTS.md` template (never overwrites) |
| `new "<name>"` | Scaffold + register a new project folder |
| `projects` | List projects you've used Cliche in (`add` / `rm` / `workspace`) |

**Trust & audit**
| Command | What it does |
|---|---|
| `verify` | Independently re-run tests + reward-hack detectors → a verdict (`--claim-pass`) |
| `audit` | Verify the ledger's tamper-evident hash chain **and** signature |
| `cost` | Summarize the cost ledger for this project |
| `report` | Export the ledger as a Markdown verdict (`--out <file>`, or `--pr <n>` to post to a GitHub PR) |
| `insights` | Usage & spend report from the ledger + saved sessions |
| `demo` | Run the Trust Kernel offline against four scenarios |
| `map` | Print the repo map the agent starts with (`--full`) |
| `models` | Show the maintained price table behind dollar estimates |

**Connect & extend**
| Command | What it does |
|---|---|
| `mcp` | List MCP servers; `mcp install <name>` builds + wires one in (e.g. `github`) |
| `connect "<name>"` | Connect an OAuth MCP connector via in-terminal device login (e.g. `github`) |
| `connectors` | List connectors (`connectors rm <name>`) |
| `org` | Connect a control plane (Team tier): `org login` pins a signed tighten-only policy; `org show` / `logout` |
| `commands` | Custom saved-prompt slash commands (`commands new <name>`) |
| `skills` | Skills the agent uses automatically (`skills new <name>`) |
| `plugins` | Installable bundles: skills + commands + hooks + MCP (`plugins new <name>`) |
| `themes` | List UI palettes |
| `memory` | Show/edit cross-session project memory (`add` / `clear`) |
| `sessions` | List saved chat sessions |

**Meta:** `config` (print + validate), `bug`, `version` (`-v`), `help` (`-h`).

</details>

<details>
<summary><b>Flags</b> — most apply to <code>run</code> / <code>exec</code> / <code>chat</code> / <code>swarm</code></summary>

| Flag | Purpose |
|---|---|
| `--model <id>` | Model id (default from config) |
| `--provider <name>` | `anthropic` / `openrouter` / `openai` / … (default: auto-detect from your keys) |
| `--base-url <url>` | Override the endpoint — any OpenAI-compatible API |
| `--max-usd <n>` | Estimated dollar cap (a hard token cap is also enforced) |
| `--max-tokens <n>` | Hard token cap |
| `--max-turns <n>` | Governor turn limit |
| `--mode <m>` | `plan` / `suggest` / `auto-edit` / `full` permission mode |
| `--allow-write` / `--allow-run` / `--allow-web` / `--allow-mcp` | Pre-authorize that capability without asking |
| `--allow-outside-root` | Permit file access outside the project root (off by default) |
| `--sandbox` | Strict posture: confine to root, deny non-allowlisted egress, scrub keys from shell env |
| `--yolo` | Skip approvals — never the budget cap or the governor |
| `--verify` | Re-run tests + report a verdict after completion |
| `--dir <path>` | Project root (default `.`); file tools are confined to it |
| `--branch` / `--commit` | Work on a fresh `cliche/<id>` branch / commit on success |
| `-p <prompt>` | Prompt for headless `exec` (else stdin) |
| `--resume <id>` / `--continue` | Resume a saved session / the most recent |

</details>

<details>
<summary><b>Slash commands</b> — inside a chat session</summary>

`/status` `/cost` `/context` `/insights` `/rules` `/permissions` · `/plan` `/tasks` `/done` · `/sessions` `/new` `/fork` `/resume` `/kill` · `/browse` `/dash` `/tui` `/changes` · `/diff` `/undo` `/rewind` `/commit` `/verify` · `/model` `/models` `/provider` · `/connect` `/mcp` · `/memory` `/theme` `/mode` `/clear` `/bug` `/help`

</details>

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
  "egress": { "allow": ["api.github.com", "*.openai.com"] },
  "hooks": { "pre_tool_use": "./scripts/policy.sh" }
}
```

Config is **validated on load** — a 0/negative cap, or a repetition window smaller than its threshold, fails *loudly* rather than silently disarming a guardrail. Cliche also reads `AGENTS.md` (falling back to `CLAUDE.md` / `GEMINI.md`) for project context, including a `## verify` / `test:` line that sets the Verifier's command. Full shape: [docs/config.example.json](docs/config.example.json).

---

## Bring any model

Cliche is provider-neutral and **auto-detects the backend** from whichever key you have. **24 providers** ship as built-in presets — `cliche login` lists them, or pass `--provider <name>` (the matching `*_API_KEY` env var works too, and always overrides a saved key):

| Tier | Providers |
|---|---|
| **Native** | Anthropic (Messages API + prompt caching) |
| **Aggregator** | OpenRouter (one key, hundreds of models) |
| **First-party (OpenAI-compatible)** | OpenAI · Google (Gemini) · xAI (Grok) · DeepSeek · Mistral · Cohere · Perplexity · Moonshot (Kimi) · Zhipu (GLM) · GitHub Models |
| **Inference clouds** | Groq · Cerebras · Together · Fireworks · DeepInfra · NVIDIA NIM · SambaNova · Hyperbolic · Novita |
| **Local — no key needed** | Ollama · LM Studio · vLLM |
| **Anything else** | any OpenAI-compatible endpoint via `providers` in config or `--base-url` |

Route delegated subtasks to a cheaper model with `subagents.model`. Reach gated backends (Bedrock/Vertex/corporate proxies) via custom auth headers in `providers[].headers`.

---

## Extend it

- **MCP** — stdio *and* Streamable-HTTP Model Context Protocol servers. Their tools are permission-gated and governed by the same caps, governor, and ledger as built-ins. `cliche mcp install github` builds and wires one in.
- **Connectors** — seamless OAuth: `cliche connect github` runs an in-terminal device login (or reuses your `gh` CLI token), saved globally and live in every chat.
- **Skills** — `.cliche/skills/<name>/SKILL.md` is injected into the agent automatically. `cliche skills new <name>`.
- **Custom commands** — `.cliche/commands/<name>.md` with `$ARGUMENTS` / `$1` substitution becomes `/<name>`. `cliche commands new <name>`.
- **Hooks** — a `pre_tool_use` command can **block any tool call** (non-zero exit fails *closed*); a `post_tool_use` / `stop` hook observes. Policy you write, enforced by a program.
- **Plugins** — a `.cliche/plugins/<name>/` bundle packages skills + commands + hooks + MCP behind one manifest.

---

## Trust, honestly

A trust tool that oversells its guarantees isn't one. Here's the **precise scope** of each — the protections are real, and so are their edges:

- **Budget** — the **token cap is the hard guarantee**; the **dollar cap is an estimate** from a maintained price table (an unknown model can price at zero). There is no mid-completion abort yet, so a *single* turn can overshoot — it's caught at the next turn boundary, not token-by-token.
- **Governor** — bounds the **orchestration loop** via deterministic checks; it does not hard-kill compute, network, or filesystem activity already running inside a single tool call. Repetition detection relies on a stable tool-call signature.
- **Verifier** — catches a fixed set of **documented** reward-hack patterns and can contradict a false "tests pass" claim. It is **not** a security boundary against an adversary who knows the rules; rename/comment/hardcode evasions can beat the static detectors.
- **Sandbox** — **user-space** confinement (root path confinement, network-deny-by-default, credential scrubbing from shell env). It is deliberately *not* a kernel jail (no seccomp/landlock/Job Objects) — that's the price of staying zero-dependency. A subprocess can still reach the network.
- **Egress allowlist** — gates the **built-in `web_fetch` tool** (re-checked on every redirect hop). It does **not** constrain `run_command` subprocesses or MCP servers. An empty allowlist means allow-all.
- **Ledger** — **tamper-evident, not tamper-proof.** It's an append-only file with normal permissions; the protection is *detection* (hash chain + signature), not write-prevention. A missing seal prints `unsealed` and exits 0; pure **tail-truncation** of the newest entries is only caught if a prior seal covered the longer head. Forgery is detectable only by someone *without* your (plaintext-on-disk) signing key.

We'd rather state the limits than oversell them. Full threat model: [SECURITY.md](SECURITY.md).

---

## For teams

The CLI is free and Apache-2.0, forever. Once more than one person runs agents against a shared codebase — and someone is accountable for what they do — the same guardrails go org-wide: **push a signed policy** every developer's kernel enforces (tighten-only, `--yolo` still can't bypass it), **aggregate every signed ledger** into one tamper-evident audit, and **govern spend** across the team. That's the commercial layer; the kernel underneath stays open.

See [COMMERCIAL.md](COMMERCIAL.md) for tiers, or run `cliche org` to connect.

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

# Go (any platform, 1.23+)
go install github.com/mholovetskyi/cliche/cmd/cliche@latest
```

Prefer a **signed binary**? Grab one from [Releases](https://github.com/mholovetskyi/cliche/releases) — one static binary per platform, with `checksums.txt`, an SBOM, and a keyless **cosign** signature so you can verify the supply chain end to end ([how](SECURITY.md#verify-your-download)). Or build from source:

```sh
git clone https://github.com/mholovetskyi/cliche && cd cliche
go build -o cliche ./cmd/cliche
```

---

## Develop

```sh
go vet ./...
go test ./...                  # every package — zero third-party deps
go build -o cliche ./cmd/cliche
```

The whole kernel is stdlib-only Go (`go.mod` has no `require` block), cross-compiles to macOS/Linux/Windows, and every guardrail is unit-tested. Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

---

## License

[Apache-2.0](LICENSE). The kernel and CLI are fully open — **a trust tool you can't read isn't one.**
