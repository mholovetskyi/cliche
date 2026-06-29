# Cliché sandbox — the per-user container for the cloud product

This is the keystone of the cloud/mobile architecture: **one isolated container
per user**, running `cliche serve` in authed, network-bound mode. The phone app
(or web client) talks to a gateway; the gateway authenticates the user and proxies
to *their* sandbox. The heavy lifting — an agent that runs `git`/`node`/`go`/a
headless browser against a real filesystem — happens here, never on the device
(phones can't run a shell; that's why the app is a thin client).

```
📱 app ──HTTPS/SSE + token──▶ 🌐 gateway ──▶ 📦 this container (cliche serve --listen 0.0.0.0:7878)
```

## The security model (non-negotiable)

- **The container is the isolation boundary.** One user per container, always. The
  agent runs arbitrary shell commands; sharing a host = sharing an RCE.
- **No unauthenticated exposure.** `cliche serve` refuses to bind a non-loopback
  address without a token (`internal/web/auth.go`). The orchestrator injects a
  strong, per-session `CLICHE_SERVE_TOKEN`.
- The **Trust Kernel** (budget cap, governor, deny rules, egress allowlist) still
  runs inside — and matters more, since it's now *your* compute being spent.

## Build & run

```sh
# from the repo root
docker build -f sandbox/Dockerfile -t cliche-sandbox .

# run one user's sandbox (the gateway does this per session; token is per-session)
docker run --rm -p 7878:7878 \
  -e CLICHE_SERVE_TOKEN="$(openssl rand -base64 24)" \
  cliche-sandbox
```

The agent's provider key is supplied per session too (via the in-app Setup / the
`/api/provider` call, or a `CLICHE_*` env the gateway injects) — never baked into
the image.

## What still needs building (the private control plane)

This image + the authed serve mode are the **open-core** foundation. The rest is
the paid product and lives outside this repo (`/private/`):

1. **Sandbox provider** — run this image as a per-user microVM/container. Recommended
   starting points: [E2B](https://e2b.dev) (purpose-built for AI-agent code
   execution), [Fly Machines](https://fly.io/docs/machines/) (fast boot, cheap), or
   Firecracker. Map `user/session → container`, idle-stop to control cost.
2. **Gateway** — public HTTPS endpoint: authenticate the app, look up the user's
   sandbox, proxy HTTP + SSE through (forwarding the per-session token), enforce
   quotas. Add CORS for the app origin (the server already emits CORS in token mode).
3. **Billing** — Stripe; meter sandbox-time / tokens against the plan.
4. **The mobile app** — Capacitor wrapper of the Studio UI pointed at the gateway,
   with login + a "build finished" push notification. → TestFlight / Play.

See the repo root discussion for the phased plan.
