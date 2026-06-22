# Cliche roadmap

Cliche competes on the **harness and governance**, not model capability. The
through-line from v0 to v2: *trust matters most when you're not watching, so
build toward asynchronous, fleet-orchestrated, unattended runs.*

Legend: ✅ done in v0 · 🟡 partial · ⬜ planned

---

## v0 — "Trustworthy single-session autonomy" (this release)

The point of v0 is to prove **one** thing: a single agent run you can leave
unattended without a runaway, a blown budget, or a silently faked result.

**AI-first agentic experience**
- ✅ Interactive session (`cliche chat`): persistent conversation + session-wide
  budget, fresh per-task governor, live activity stream, slash commands.
- ✅ Ask-before-acting permissions: interactive `y/N/always` in a TTY, showing a
  **diff preview** of the exact change before you authorize it; `--yolo` and
  allow-flags still pre-authorize (never bypassing caps/governor).
- ✅ Robust `edit_file` tool: exact match → whitespace-tolerant line-block
  fallback (targeted edits, not full-file overwrites).
- ✅ Confined code-search tools — `search_files` (regex grep), `find_files`
  (glob, `**`-aware), `list_files` — so the agent finds code natively instead of
  shelling out to grep/find (project-root-confined, like `read_file`).
- ✅ Budget-protecting I/O bounds: `read_file` pages large files (offset/limit,
  2000-line default cap) and `run_command` output is capped, so a single tool
  call can't flood the context window or the token budget.
- ✅ Session edit journal: `/diff` shows everything changed this session, `/undo`
  reverts the last edit, and `run` ends with a change summary — in-memory only
  (never persisted; it holds file contents).
- ✅ `cliche init` scaffolds `.cliche/config.json` + an `AGENTS.md` template;
  `cliche models` shows the price table behind dollar estimates.
- ✅ Auto-verify wired into `run`/`exec`/`chat` (`--verify`, `/verify`).

**Extensibility**
- ✅ Provider-neutral (BYO-key): Anthropic Messages API + an OpenAI-compatible
  backend (OpenRouter, OpenAI, local servers) selectable via `--provider` /
  config, both with multi-turn tool calling and retry/backoff.
- ✅ MCP client (stdio): connect external Model Context Protocol servers via the
  `mcp` config array; their tools are namespaced (`mcp__<server>__<tool>`),
  permission-gated (`--allow-mcp`/approval), and governed by the same
  caps/governor/ledger. Still ⬜: HTTP-MCP transport and per-subagent scoped MCP.

**Hardening (audit pass)**
- ✅ Project-root confinement for all file tools (+ `--allow-outside-root` hatch),
  symlink-aware (an in-root symlink pointing outside is rejected).
- ✅ Pre-write syntax validation for Go (go/parser) and JSON (encoding/json).
- ✅ `cliche config` prints and validates the effective configuration.
- ✅ POSIX-shell-preferring command execution with correct exit-code propagation
  on Windows (no more false `verified` from a mis-read pipeline).
- ✅ CRLF-preserving edits; config validation; AGENTS.md parser hardening.
- ✅ Provider retry/backoff (429/5xx) honoring Retry-After; bounded response body.
- ✅ SIGINT cancellation with a structured `cancelled` outcome; wall-clock
  overrun now reports a structured `max_wallclock` halt.
- ✅ Ledger fsync + surfaced write errors; attributable tool target in the audit
  trail; `version` reports build metadata; NO_COLOR ASCII fallback.

**Differentiated core**
- ✅ Budget Kernel — token-hard cap + estimated dollar cap, enforced
  pre-flight *and* mid-stream (catches the fat-completion blowout).
- ✅ Governor — max turns, wall-clock, consecutive-failed-edits, repetition
  detection, no-progress halt. On by default.
- ✅ Cost Ledger — append-only JSONL, `cliche cost` summary, no secrets/code.
- ✅ Verifier — deterministic detectors (deleted test, swallowed error,
  weakened assertion); biases to "unverified" over false "flagged".
- ✅ Headless `exec` — JSON output, clean exit codes, fails loudly on caps.
- ✅ Offline `demo` — four scenarios, real output, no key/network.
- ✅ Zero-dependency single static binary.

**Table-stakes (adequate, deliberately not excellent yet)**
- ✅ Graduated permissions; `--yolo` skips approvals but never caps/governor.
- ✅ `.cliche/config.json` over safe defaults; `AGENTS.md` detection.
- ✅ Provider abstraction — Anthropic backend does **multi-turn tool use**
  (read/write/run, advertised tool schemas, tool_use/tool_result loop); offline
  Mock for tests/demo.
- ✅ Verifier independent test re-run (the keystone): `cliche verify` re-runs the
  project's tests (auto-detected or from `AGENTS.md`), and `verified` is only
  returned when a real re-run passes. False "tests pass" claims are flagged.
- 🟡 Reliable diff/edit engine: `edit_file` does exact → whitespace-tolerant →
  confidence-scored anchored fuzzy matching (refuses single-line / anchor-less
  matches), plus **Go AST syntax validation** that rejects edits which would
  break the file. Still ⬜: per-language AST anchoring beyond Go's parser.
- ✅ Context Ledger — bounded, recoverable, never-silent compaction. Compacts
  only at safe task boundaries; `/context` shows usage, `/recover` undoes it.
- ⬜ Secrets in OS keychain; signed/reproducible releases (Sigstore + SBOM).
- ⬜ Network egress denied-by-default with allowlist.
- ⬜ Team budget + verdict dashboard (the first paid wedge).

**Explicit v0 non-goals:** mid-session model switching, session fork, HTTP-MCP,
Windows tier-1 TUI, OS sandbox, subagents, skills/hooks, local-model hardening.

---

## v1 — "Trustworthy fleets and the async frontier"

- ⬜ Fleet budgets — org-level ceilings across concurrent agents/CI jobs
  (+ SSO, audit logs, self-hosted control plane, SOC2 path).
- 🟡 Subagents: `spawn_subagent` delegates one isolated subtask, and
  `spawn_subagents` runs several CONCURRENTLY — each with a FRESH context and a
  budget scoped under (and bubbling into) the session cap, depth-limited. The
  Budget Kernel is concurrency-safe (one lock at a time; charges still bubble to
  the authoritative root). Still ⬜: per-subagent scoped MCP.
- ⬜ Ticket-to-PR with the Verifier verdict as the first PR comment.
- ⬜ Verifier v2 — model-assisted critic on a *separate cheap model*.
- ⬜ OS sandbox (Seatbelt / Landlock+seccomp / Windows job objects);
  `--yolo` becomes "earned by isolation".
- ⬜ WASM plugin SDK + signed registry (no config-from-URLs).
- ⬜ Skills (`SKILL.md`) + hooks (pre-tool, post-edit, on-halt, on-budget).
- ⬜ ACP server mode — make the governance travel into other editors/hosts.
- ⬜ Hardened local-model layer (tool-call repair shim).

---

## v2 — destination

The architecture is deliberately the control plane for **asynchronous, hosted,
unattended cloud runs** — the Trust Kernel governing background agents that open
PRs while you sleep. v0 and v1 are built toward this, not bolted onto it.

---

## How we measure success (and what would falsify the bet)

- v0 success = demonstrable "runaways prevented / dollars saved" in real CI use.
- Business success = paying teams for the budget+verdict dashboard within two
  quarters of v0.
- Kill signal: installs climb while paid conversion flatlines → the
  bottom-up-to-paid bridge is wrong; pivot to top-down design-partner sales.
