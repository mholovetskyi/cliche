# Cliche Studio — building the Windows `.exe` installer

Cliche Studio is the desktop app: the **zero-dependency CLI** (`cliche.exe`, with
the React UI embedded) shown in a native **WebView2 window** (`cliche-studio.exe`,
a separate Go module so the webview dependency never touches the CLI core),
packaged as a double-click installer.

```
ClicheStudioSetup.exe
   ├─ cliche.exe          ← zero-dep CLI; `cliche serve` runs the local web app
   └─ cliche-studio.exe   ← WebView2 shell: launches `cliche serve`, shows it in a window
```

## Build it (on Windows)

Prerequisites: **Go 1.23+**, **Node 18+** (only to (re)build the UI), and
**[Inno Setup 6](https://jrsoftware.org/isinfo.php)** (`ISCC` on PATH).

With `make`:

```sh
make installer        # ui → cli → desktop → ISCC → dist\ClicheStudioSetup.exe
```

Or step by step:

```sh
# 1. (re)build the React UI → internal/web/static (committed; skip if unchanged)
cd studio && npm ci && npm run build && cd ..

# 2. the CLI (UI baked in) and the desktop shell
go build -o dist/cliche.exe ./cmd/cliche
cd desktop && go build -o ../dist/cliche-studio.exe . && cd ..

# 3. the installer
ISCC installer/cliche-studio.iss      # → dist/ClicheStudioSetup.exe
```

## Notes

- The **WebView2 runtime** ships with Windows 10/11; on an older machine install it
  from <https://developer.microsoft.com/microsoft-edge/webview2/>.
- `cliche.exe` alone is the full CLI **and** the web app (`cliche serve` opens your
  browser) — the installer/shell just give it a native window and a Start-Menu icon.
- The CLI binary stays a single static, **zero-dependency** executable; only the
  desktop shell (`desktop/go.mod`) and the UI (`studio/`) pull in tooling.
