# Cliché Studio — the iOS / Android app

A thin native client. It wraps the **same React UI** that ships in the Go binary
(`internal/web/static`) with [Capacitor](https://capacitorjs.com), and points
itself at a **remote Cliché backend** — because a phone can't run the agent (no
shell on iOS). The agent runs in your cloud sandbox; the app talks to it over
HTTPS + SSE with a token. See `../sandbox/README.md` for the backend.

```
📱 this app  ──HTTPS/SSE + token──▶  🌐 gateway  ──▶  📦 your cliche serve sandbox
```

## How it connects

On first launch the app shows a **Connect** screen (server URL + access token).
Everything then routes through `studio/src/lib/api.ts`:

- `api()` prepends the backend base + the `Authorization: Bearer` token.
- SSE/downloads use `?token=` (EventSource can't set headers).

In the browser/desktop there's no configured backend → same-origin, no token →
identical to today. The app-only bits (Connect screen, the native "build
finished" notification) activate only when `window.Capacitor` is present, so the
shared web build pulls in **zero** Capacitor dependencies.

## Build & run

Prereqs: **Node 18+**, the **Capacitor CLI** (installed below). iOS also needs a
**Mac + Xcode**; Android needs **Android Studio**.

```sh
cd mobile
npm install

# generate the native projects (one-time; iOS only on a Mac)
npx cap add ios
npx cap add android

# build the web UI and copy it into the native projects
npm run sync

# open the native IDE to run / sign / archive
npm run ios       # Xcode  (Mac only)
npm run android   # Android Studio
```

Re-run `npm run sync` after any change to the Studio UI.

## Before you publish

- Set a real `appId` (reverse-DNS) in `capacitor.config.ts` that matches your
  Apple / Play account.
- App icon + splash: drop `assets/logo.png` (1024²) and run
  `npx @capacitor/assets generate` (the brand mark lives in `../assets/logo.svg`).
- Accounts: **Apple Developer** ($99/yr), **Google Play** ($25 once).
- Apple review: the app is a client to a server-side agent (it does **not**
  execute downloaded code on-device), the same posture as GitHub Mobile / Replit
  — that's what keeps it compliant.
- **Push** ("build finished" while the app is closed) needs APNs/FCM + the gateway
  sending the push. The bundled `@capacitor/local-notifications` already fires a
  local notification when a run ends while the app is backgrounded (no server push
  required) — `notifyDone()` in `studio/src/App.tsx`.

## What still needs the private control plane

The gateway, the sandbox provider (E2B / Fly), and Stripe billing live outside
this repo (`/private/`). This folder is just the open-source app shell.
