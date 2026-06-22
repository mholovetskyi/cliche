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
- ✅ Ask-before-acting permissions: interactive `y/N/always` in a TTY; `--yolo`
  and allow-flags still pre-authorize (never bypassing caps/governor).
- ✅ Robust `edit_file` tool: exact match → whitespace-tolerant line-block
  fallback (targeted edits, not full-file overwrites).
- ✅ Auto-verify wired into `run`/`exec`/`chat` (`--verify`, `/verify`).

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
- 🟡 Reliable diff/edit engine: `edit_file` does exact + whitespace-tolerant
  line-block matching. Still ⬜: full AST-aware anchoring and confidence-scored
  fuzzy matching.
- ⬜ Context Ledger — bounded, recoverable, never-silent compaction.
- ⬜ Secrets in OS keychain; signed/reproducible releases (Sigstore + SBOM).
- ⬜ Network egress denied-by-default with allowlist.
- ⬜ Team budget + verdict dashboard (the first paid wedge).

**Explicit v0 non-goals:** mid-session model switching, session fork, HTTP-MCP,
Windows tier-1 TUI, OS sandbox, subagents, skills/hooks, local-model hardening.

---

## v1 — "Trustworthy fleets and the async frontier"

- ⬜ Fleet budgets — org-level ceilings across concurrent agents/CI jobs
  (+ SSO, audit logs, self-hosted control plane, SOC2 path).
- ⬜ Subagents with **per-subagent scoped budget + scoped MCP**.
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
