# Cliche build targets.
#
# The CLI core is zero-dependency Go (one static binary). On top of it:
#   • the Studio UI  — React/Vite, built into internal/web/static (embedded)
#   • the desktop shell — a separate Go module (desktop/) using WebView2
#   • the .exe installer — Inno Setup, bundling both binaries
#
# Plain `go build ./cmd/cliche` always works with NO Node — the UI is committed
# pre-built. The targets below are for (re)building the UI and the desktop app.

VERSION ?= 0.1.0
DIST    := dist

.PHONY: all ui cli desktop installer test clean

## ui: rebuild the React Studio UI into the embedded dir
ui:
	cd studio && npm ci && npm run build

## cli: build the cliche CLI (UI baked in) into dist/
cli:
	mkdir -p $(DIST)
	go build -o $(DIST)/cliche.exe ./cmd/cliche

## desktop: build the WebView2 desktop shell (Windows) into dist/
desktop:
	mkdir -p $(DIST)
	cd desktop && GOOS=windows GOARCH=amd64 go build -o ../$(DIST)/cliche-studio.exe .

## installer: build the Windows .exe installer (requires Inno Setup's ISCC on PATH)
installer: ui cli desktop
	ISCC installer/cliche-studio.iss

## test: vet + the full Go test suite (CLI core)
test:
	go vet ./...
	go test ./...

## all: rebuild everything into a shippable installer
all: installer

clean:
	rm -rf $(DIST) studio/dist
