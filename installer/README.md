# Cliché Studio — building the Windows `.exe` installer

Cliché Studio is the desktop app: the **zero-dependency CLI** (`cliche.exe`, with
the React UI embedded) shown in a native **WebView2 window** (`cliche-studio.exe`,
a separate Go module so the webview dependency never touches the CLI core),
packaged as a per-user double-click installer.

```
ClicheStudioSetup.exe
   ├─ cliche.exe          ← zero-dep CLI; `cliche serve` runs the local web app
   ├─ cliche-studio.exe   ← WebView2 shell: native splash → launches `cliche serve` → window
   └─ MicrosoftEdgeWebview2Setup.exe  ← Evergreen bootstrapper, run only if the runtime is missing
```

The installer is **per-user** (`PrivilegesRequired=lowest`, installs to
`%LOCALAPPDATA%\Programs\Cliche`) — **no admin / UAC prompt**, the way VS Code and
Discord install. The app icon (`assets/logo.ico`) is embedded in `cliche-studio.exe`
and used for the installer + shortcuts.

## Build it (on Windows)

Prerequisites: **Go 1.23+**, **Node 18+** (only to (re)build the UI), and
**[Inno Setup 6](https://jrsoftware.org/isinfo.php)** (`ISCC` on PATH).

```sh
# 1. (re)build the React UI → internal/web/static (committed; skip if unchanged)
cd studio && npm ci && npm run build && cd ..

# 2. the CLI (UI baked in) and the desktop shell (the logo .syso is committed,
#    so `go build` embeds the icon automatically)
go build -o dist/cliche.exe ./cmd/cliche
cd desktop && go build -o ../dist/cliche-studio.exe . && cd ..

# 3. the Evergreen WebView2 bootstrapper (gitignored — fetch it before ISCC)
curl -L -o installer/MicrosoftEdgeWebview2Setup.exe "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

# 4. the installer
ISCC installer/cliche-studio.iss      # → dist/ClicheStudioSetup.exe
```

## ⚠️ Signing — required before public distribution

**The installer and both bundled binaries are UNSIGNED.** Downloaded from a website
they carry the Mark-of-the-Web, so Windows **SmartScreen shows a full-screen
"Windows protected your PC — Unknown publisher"** dialog; the user has to click
**More info → Run anyway**. A large share of non-technical users abandon there, and
some Defender/corporate policies block it outright.

To ship to the public properly, sign all three with an **OV** (or, to skip the
SmartScreen reputation ramp entirely, **EV**) **code-signing certificate** and add a
`SignTool` directive to the `.iss` `[Setup]` section. Until then, the download page
must tell users to expect the warning and how to proceed. (The repo's cosign /
sigstore signing covers the CLI *release archives'* supply-chain provenance — it is
**not** Windows Authenticode and does not touch this installer.)

## Notes

- **WebView2 runtime**: the installer bundles the Evergreen bootstrapper and runs it
  silently only when the runtime is absent (checked via the `EdgeUpdate` registry
  key), so the app works on Windows 10 SKUs that don't ship it. If the runtime is
  still missing at launch, the shell shows a native dialog pointing at the download.
- `cliche.exe` alone is the full CLI **and** the web app (`cliche serve` opens your
  browser) — the installer/shell just give it a native window and a Start-Menu icon.
  Tick **"Add the cliche command to PATH"** during install to use `cliche` in a terminal.
- Uninstall removes the app + the regenerable config (`%APPDATA%\cliche`) but
  intentionally **keeps your projects** in `~/Cliche Projects`.
- The CLI binary stays a single static, **zero-dependency** executable; only the
  desktop shell (`desktop/go.mod`) and the UI (`studio/`) pull in tooling.
