# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Uhr** is a church/choir display system that shows hymn numbers and a clock on presentation screens. Four components work together:

- `server/` — Go HTTP + WebSocket server (port 1980); embeds `server/static/` at build time
- `frontend/` — React 19 + TypeScript + Tailwind + shadcn/ui SPA (Vite); builds to `server/static/`
- `kiosk/` — Wails desktop app that wraps the frontend in native windows (one per screen)
- `watchdog/` — Go process monitor that auto-restarts `kiosk.exe` on crash

There are no tests in this codebase.

## Commands

### Produktions-Build (alle Komponenten)
```bat
build.bat          :: alles
build-server.bat   :: nur Frontend + Server
build-kiosk.bat    :: nur Kiosk + Watchdog
```
`npm install` wird automatisch nachgeholt wenn `frontend/node_modules/` fehlt. Alle Skripte pausieren bei Fehler.

Ausgabe in `_build/`:

```
_build/
├── server/
│   └── liedanzeige-server.exe
└── kiosk/
    ├── liedanzeige-kiosk.exe
    └── liedanzeige-watchdog.exe
```

`config.json` jeweils im entsprechenden Unterordner ablegen — Vorlage: `config.example.json`. `settings.json` wird vom Server automatisch in `_build/server/` angelegt — nicht manuell erstellen oder löschen.

### Entwicklung (einzelne Komponenten)

```bash
cd server && go run .
cd frontend && npm run dev   # :5173, proxies /ws → :1980
cd kiosk && wails dev
cd watchdog && go run .
```

## Architecture

### WebSocket channels & message flow

`server/hub.go` maintains a `Hub` with five named channels: `lied`, `chor`, `steuerung`, `kiosk`, `log`.

- **steuerung** clients send: `input`, `backspace`, `reset`, `settings`, `kiosk`. Server echoes back so all operator tabs stay in sync — `Steuerung.tsx` never updates local state directly.
- `target: "lied"` broadcasts to both `lied` and `chor`; `target: "chor"` controls only `chor` independently.
- On steuerung client connect, hub sends a `sync` message with `liedState`, `chorState`, **`steuerungState`** and current `settings`.
- **`steuerungState`** is a third Hub state updated by ANY input regardless of channel — authoritative display value for all Steuerung tabs. Both Lied- and Chor-Steuerung always show the same number simultaneously.
- On display client connect, hub pushes current state (`display` action) + current settings.
- **kiosk** channel: `fullscreen`, `windowed`, `reload`, `move_to`; `swap_monitors` remaps screen indices.
- **log** channel: server pushes `{ action:"log", level, message, ts }`; last 100 entries replayed on connect. Client sends `{ action:"clear_log" }` to reset.

### Frontend routes

| URL | Page | Purpose |
|-----|------|---------|
| `/` | Server HTML | Landing page with links to all URLs |
| `/steuerung/lied` | `Steuerung.tsx` | Operator control — congregation |
| `/steuerung/chor` | `Steuerung.tsx` | Operator control — choir |
| `/lied` | `Liedanzeige.tsx` | Congregation display |
| `/chor` | `Liedanzeige.tsx` | Choir display |

`/steuerung` redirects to `/steuerung/lied`. The landing page at `/` is served by the Go server directly, not the SPA.

`Liedanzeige` has two modes: **clock mode** (no input) and **number mode** (hymn number entered). Settings applied as CSS custom properties on `<html>`. Auto-reset timer returns to clock mode after `resetDelay` minutes.

### Steuerung display logic

- Single `display` state, initialised from `steuerungState` in the `sync` message on connect.
- All input echoes (lied AND chor) update the display on ALL Steuerung tabs — max 4 digits, `>= 4` guard on both send and receive.
- Workflow: LÖSCHEN always clears both channels before entering a new number. LÖSCHEN sends `reset target:"lied"` which resets `liedState`, `chorState` AND `steuerungState`.
- Terminal log panel (only `/steuerung/lied`) shows server events via `/ws/log`.

### Key types (`frontend/src/lib/types.ts`)

`WsMessage` is the discriminated union of all WebSocket message shapes. `DisplaySettings` and `DEFAULTS` define display configuration. **`DEFAULTS` in `types.ts` must stay in sync with defaults in `server/config.go`.**

### Settings persistence — three layers

1. **`localStorage`** (key `liedanzeige-settings`) — fallback initial value only
2. **Server Hub** — authoritative; pushed via `sync` to new Steuerung clients and via `settings` to display clients
3. **`server/settings.json`** — written async on each `settings` WS message; read on restart

### Kiosk (Wails)

`kiosk/app.go` — on `domReady` (guarded by `sync.Once`), polls `/health`, then navigates to screen URL. Screen 0 spawns screens 1+ as subprocesses (`--screen N`) in production or browser tabs in dev. Connects to `/ws/kiosk` for window-control.

`kiosk/monitor.go` (Windows-only) — Win32 APIs for fullscreen positioning. Calls `window.__kioskBlackout(bool)` on WebView to fade during monitor moves.

`kiosk/numpad.go` (Windows-only, choir screen only) — `WH_KEYBOARD_LL` hook captures numpad globally, forwards to `/ws/steuerung`. Handles NumLock-on and NumLock-off.

The kiosk has its own embedded frontend in `kiosk/frontend/` (separate from `frontend/`).

### Watchdog

`watchdog/main.go` — `startMu` prevents concurrent restarts; `mu` protects `proc` and crash counters. Warns after 5 rapid crashes. Subscribes to `/ws/kiosk`, restarts on `"reload"`. Writes `watchdog.log` next to exe.

### Server extras

- **Firewall** (`server/firewall_windows.go`): checks for rule `"Liedanzeige-Server"` on start; triggers UAC to add it if missing.
- **File logging**: `server.log` next to exe via `io.MultiWriter`.
- **Landing page** (`server/landing.go`): `/` serves static HTML, does not load the SPA.
- **WebSocket origin**: allows empty Origin (Go clients), localhost, same-host only.

### Path alias

Frontend uses `@/` → `frontend/src/` (`vite.config.ts` + `tsconfig.app.json`).

## Configuration

All Go components read **`config.json`** from their working directory or parent, creating defaults on first run.

`server/settings.json` is managed exclusively by the server.

Key `config.json` fields (see `config.example.json`):

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

`dev: false` → kiosk runs fullscreen and always-on-top. `server_host` must be the LAN IP for multi-machine setups.
