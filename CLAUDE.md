# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Uhr** is a church/choir display system that shows hymn numbers and a clock on presentation screens. Four components work together:

- `Development/server/` — Go HTTP + WebSocket server (port 1980); embeds `server/static/` at build time
- `Development/frontend/` — React 19 + TypeScript + Tailwind + shadcn/ui SPA (Vite); builds to `server/static/`
- `Development/kiosk/` — Wails desktop app; ohne `--screen`-Flag läuft es als Supervisor (startet und überwacht alle Screens), mit `--screen=N` als Kiosk-Fenster

Source lives entirely in `Development/`. There are no tests in this codebase.

## Commands

### Produktions-Build (alle Komponenten)
```bat
build.bat          :: alles
build-server.bat   :: nur Frontend + Server
build-kiosk.bat    :: nur Kiosk
```
`npm install` wird automatisch nachgeholt wenn `frontend/node_modules/` fehlt. Alle Skripte pausieren bei Fehler.

> **Kiosk-Build:** `build-kiosk.bat` ruft `wails build -skipbindings` auf. Der Wails-Binding-Generator hängt sich auf diesem System auf; `-skipbindings` überspringt ihn sicher, da sich die gebundenen Go-Typen selten ändern. Wails legt die EXE zunächst in `Development/kiosk/build/bin/` ab — das Skript kopiert sie nach `_build/Kiosk/liedanzeige-kiosk.exe` und löscht `build/bin/` danach automatisch. Das Verzeichnis `Development/kiosk/build/` (Icons, Manifests) bleibt erhalten.

> **Wichtig:** Der Go-Server bettet die Frontend-Dateien per `//go:embed static` zur Compile-Zeit ein. Nach jeder Frontend-Änderung muss deshalb **auch der Go-Server neu gebaut werden** — `npm run build` allein reicht nicht. Schnellster Weg: `build-server.bat` (baut Frontend + Server in einem Schritt).

Ausgabe in `_build/` (Repo-Root):

```
_build/
├── Server/
│   └── liedanzeige-server.exe
└── Kiosk/
    └── liedanzeige-kiosk.exe   (Supervisor + Kiosk in einer Binary)
```

`config.json` jeweils im entsprechenden Unterordner ablegen — Vorlage: `config.example.json` im Repo-Root. `settings.json` wird vom Server automatisch in `_build/Server/` angelegt — nicht manuell erstellen oder löschen.

### Entwicklung (einzelne Komponenten)

```bash
cd Development/server && go run .
cd Development/frontend && npm run dev   # :5173, proxies /ws → :1980
cd Development/kiosk && wails dev
cd Development/watchdog && go run .
```

## Architecture

### WebSocket channels & message flow

The server (`Development/server/hub.go`) maintains a `Hub` with five named channels: `lied`, `chor`, `steuerung`, `kiosk`, `log`.

- **steuerung** clients send: `input`, `backspace`, `reset`, `settings`, `kiosk`. Server echoes back so all operator tabs stay in sync — `Steuerung.tsx` never updates local state directly.
- `target: "lied"` broadcasts to both `lied` and `chor`; `target: "chor"` controls only `chor` independently.
- On connect, steuerung clients receive a `sync` message with `liedState`, `chorState`, `steuerungState` and current `settings`.
- **`steuerungState`** is a third Hub state (separate from `liedState`/`chorState`) updated by ANY input regardless of channel — this is the authoritative display value for all Steuerung tabs. Both Lied- and Chor-Steuerung always show the same number.
- On display client connect, hub pushes current state (`display` action) + current settings.
- **kiosk** channel carries: `fullscreen`, `windowed`, `reload`, `move_to`; `swap_monitors` remaps screen indices.
- **log** channel: server pushes `{ action: "log", level, message, ts }` entries; history (last 100) is replayed on connect. Clients send `{ action: "clear_log" }` to reset history.

### Frontend routes

| URL | Page | Purpose |
|-----|------|---------|
| `/` | Landing page (server HTML) | Links to all available URLs |
| `/steuerung/lied` | `Steuerung.tsx` | Operator control — congregation |
| `/steuerung/chor` | `Steuerung.tsx` | Operator control — choir |
| `/lied` | `Liedanzeige.tsx` | Congregation display |
| `/chor` | `Liedanzeige.tsx` | Choir display |

`/steuerung` redirects to `/steuerung/lied`. The landing page at `/` is served directly by the Go server (not the SPA).

`Liedanzeige` has two modes: **clock mode** (no input) and **number mode** (hymn number entered). Settings applied as CSS custom properties on `<html>`. Auto-reset timer returns to clock mode after `resetDelay` minutes.

### Steuerung display logic

- Display is a single shared state across all Steuerung tabs, initialised from `steuerungState` in the `sync` message.
- All input echoes (lied AND chor) update the display on ALL Steuerung tabs — max 4 digits, `>= 4` guard on both send and receive.
- Workflow: LÖSCHEN always clears both channels before entering a new number. LÖSCHEN on Chor-Steuerung sends `reset target: "lied"` which resets both `liedState` and `chorState`.
- Terminal button (only on `/steuerung/lied`) opens a log panel showing server events in real-time.

### Key types (`Development/frontend/src/lib/types.ts`)

`WsMessage` is the discriminated union of all WebSocket message shapes. `DisplaySettings` and `DEFAULTS` define display configuration. **`DEFAULTS` in `types.ts` must stay in sync with the defaults in `Development/server/config.go`.**

### Settings persistence — three layers

1. **`localStorage`** (key `liedanzeige-settings`) — fallback initial value only
2. **Server Hub** — authoritative at runtime; pushed via `sync` message to new Steuerung clients and via `settings` action to display clients
3. **`server/settings.json`** — written async on each `settings` WS message; read on restart

### Kiosk (Wails)

`Development/kiosk/app.go` — on `domReady` (guarded by `sync.Once`), polls `/health` until server responds, then navigates to screen URL. Screen 0 spawns screens 1+ as subprocesses (`--screen N` flag) in production or browser tabs in dev mode. Connects to `/ws/kiosk` for window-control commands.

`Development/kiosk/monitor.go` (Windows-only) — Win32 APIs (`EnumDisplayMonitors`, `SetWindowPos`) to move windows fullscreen. Calls `window.__kioskBlackout(true/false)` on the WebView to fade during repositioning.

`Development/kiosk/numpad.go` (Windows-only, choir screen only) — low-level keyboard hook (`WH_KEYBOARD_LL`) captures numpad globally and forwards to `/ws/steuerung`. Handles NumLock-on and NumLock-off states.

The kiosk has a minimal embedded frontend in `Development/kiosk/frontend/dist/index.html` — eine einzelne statische HTML-Datei (kein npm, kein Build-Schritt). Wails bettet sie zur Compile-Zeit ein; die eigentliche Anzeige kommt vom Haupt-Frontend nach Navigation.

### Watchdog

`Development/watchdog/main.go` — `startMu` prevents concurrent restarts; `mu` protects `proc` and crash counters. Logs a warning after 5 rapid crashes (<30s runtime). Subscribes to `/ws/kiosk` and restarts kiosk on `"reload"` command. Writes `watchdog.log` next to the exe.

### Server extras

- **Firewall** (`server/firewall_windows.go`): on first start, checks for rule `"Liedanzeige-Server"` via `netsh`; if missing, triggers UAC dialog to add it.
- **File logging**: `setupLogging()` writes to `server.log` (next to exe) + stdout via `io.MultiWriter`.
- **Landing page** (`server/landing.go`): `/` serves a static HTML page listing all URLs; does not load the SPA.
- **WebSocket origin check**: allows empty Origin (Go clients), localhost, and same-host requests only.

### Path alias

Frontend uses `@/` → `frontend/src/` (configured in `vite.config.ts` and `tsconfig.app.json`).

## Configuration

All Go components read **`config.json`** from their working directory or parent, creating it with defaults on first run.

`server/settings.json` stores display settings and is managed exclusively by the server.

Key `config.json` fields:

```json
{
  "server_host": "192.168.1.100",
  "port": 1980,
  "dev": false,
  "screens": [
    { "name": "liedanzeige", "url": "/lied", "monitor": 1 },
    { "name": "choranzeige", "url": "/chor", "monitor": 0 }
  ],
  "kiosk": { "always_on_top": true }
}
```

`dev: false` → kiosk runs fullscreen and always-on-top. `server_host` must be the LAN IP for multi-machine setups. `config.example.json` in `Development/` is the reference template.
