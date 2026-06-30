# Changelog

All notable changes to Cliché are documented here. The format is loosely based on
[Keep a Changelog](https://keepachangelog.com/); dates are when the change landed
on `main`.

## Unreleased

### Cliché Studio (the web app)
- **Live dev-server preview** — `cliche serve` can run a built app's real dev
  server (`npm run dev`, Vite/Next/CRA) with hot reload; the preview auto-opens on
  the first pass and live-reloads on every edit. A **Build ⇄ Live** toggle picks
  between a static build and a running dev server so a leftover server can't hijack
  the preview.
- **Projects & Apps** — open project folders (each with its own chats and apps) and
  run/preview the apps inside them. Switching a project re-roots the whole studio.
- **`clone_site`** — paste a URL and Cliché screenshots + reads the original to
  rebuild it as a clean, responsive app, then screenshot-critiques and iterates.
- **`update_plan`** — the agent maintains a live progress checklist you watch tick
  off as it builds.
- **Hermes-style nav** — Skills & Tools · Messaging · Artifacts pages, a serif
  "Cliché" identity, light/dark themes, and a sectioned Settings panel
  (provider/model · personality · permissions · **live + per-session Trust-Kernel
  limits** · appearance).
- **Personalities** — built-in presets + your own `PERSONA.md`, applied everywhere
  (tone only, never permissions).

### Trust Kernel & safety
- **Untrusted-input boundary** — every tool result (built-in, MCP, or subagent) is
  now size-bounded and secret-redacted before it reaches the model or the ledger.
- **MCP SSRF guard** — MCP HTTP servers can't be pointed at link-local/cloud-
  metadata addresses to steal their OAuth Bearer token.
- **Agent-curated memory is approval-gated** — `remember`/`remember_user` go
  through the approver, so the agent can't silently rewrite its own memory.
- **Panic recovery** — a panic in a tool/provider/MCP server is recovered into a
  structured error instead of crashing a long-running `serve`/cron/telegram.

### Reliability
- CI now collects coverage (`go test -cover`); added panic-recovery and wall-clock
  bounding tests.
