# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Uhr** is a church/choir display system that shows hymn numbers and a clock on presentation screens. Four components work together:

- `Development/server/` — Go HTTP + WebSocket server (port 1980); embeds `server/static/` at build time
- `Development/frontend/` — React 19 + TypeScript + Tailwind + shadcn/ui SPA (Vite); builds to `server/static/`
- `Development/kiosk/` — Wails desktop app that wraps the frontend in native windows (one per screen)
- `Development/watchdog/` — Go process monitor that auto-restarts `kiosk.exe` on crash

`Deployment/` holds pre-built binaries and config for production use; source lives entirely in `Development/`.

There are no tests in this codebase.

## Commands

### Produktions-Build (alle Komponenten)
```bat
cd Development
build.bat          :: alles
build-server.bat   :: nur Frontend + Server
build-kiosk.bat    :: nur Kiosk + Watchdog
```
`npm install` wird automatisch nachgeholt wenn `frontend/node_modules/` fehlt.

Baut Frontend → Server/Watchdog/Kiosk und legt die EXEs in `Development/_build/` ab:

```
_build/
├── server/
│   └── liedanzeige-server.exe
└── kiosk/
    ├── liedanzeige-kiosk.exe
    └── liedanzeige-watchdog.exe
```

`config.json` jeweils im entsprechenden Unterordner ablegen — Vorlage: `Development/config.example.json`. `settings.json` wird vom Server automatisch in `_build/server/` angelegt — nicht manuell erstellen oder löschen. Keine Laufzeit-Dateien in `_build/` selbst (nur `.gitkeep`).

### Entwicklung (einzelne Komponenten)

```bash
cd Development/server
go mod tidy       # first time only
go run .
```

```bash
cd Development/frontend
npm install       # first time only
npm run dev       # dev server on :5173, proxies /ws to :1980
npm run lint      # ESLint
```

```bash
cd Development/kiosk
wails dev         # dev mode (800×600, windowed)
```

```bash
cd Development/watchdog
go run .
```

## Architecture

### WebSocket message flow

The server (`Development/server/hub.go`) maintains a `Hub` with four named channels: `lied`, `chor`, `steuerung`, `kiosk`.

- **steuerung** clients send commands: `input`, `backspace`, `reset`, `settings`, `kiosk`; the server echoes back so all operator tabs stay in sync — `Steuerung.tsx` never updates local state directly
- Sending to target `"lied"` broadcasts to both `lied` and `chor` (they mirror each other); target `"chor"` controls only `chor` independently
- When a display client connects, the hub immediately pushes current state (`display` action) + current settings
- **kiosk** channel carries remote window-control commands (`fullscreen`, `windowed`, `toggle_fullscreen`, `reload`, `move_to`)
- `swap_monitors` command toggles a `monitorsSwapped` flag in the hub and remaps all screen indices before broadcasting `move_to` commands

### Frontend routes

| URL | Page | Purpose |
|-----|------|---------|
| `/steuerung/lied` | `Steuerung.tsx` | Operator control for congregation screen |
| `/steuerung/chor` | `Steuerung.tsx` | Operator control for choir screen |
| `/lied` | `Liedanzeige.tsx` | Congregation display screen |
| `/chor` | `Liedanzeige.tsx` | Choir display screen |

`/` and `/steuerung` redirect to `/steuerung/lied`. The channel is read from the URL path (not a query param).

`Liedanzeige` has two modes: **clock mode** (no input) and **number mode** (hymn number entered). Display settings are applied as CSS custom properties on `<html>`. An auto-reset timer returns to clock mode after `resetDelay` minutes.

### Key types (`Development/frontend/src/lib/types.ts`)

`WsMessage` is the discriminated union of all WebSocket message shapes. `DisplaySettings` and `DEFAULTS` define the display configuration shared between `Steuerung` and `Liedanzeige`. **`DEFAULTS` in `types.ts` must stay in sync with the defaults in `Development/server/config.go`.**

### Settings persistence — three layers

Display settings exist in three places that must stay consistent:

1. **`localStorage`** (key `liedanzeige-settings`) — loaded by `frontend/src/lib/settings.ts` on mount, merged with `DEFAULTS`
2. **Server in-memory `Hub`** — authoritative at runtime; pushed to display clients on connect
3. **`server/settings.json`** — written asynchronously by the server on each `settings` WS message; read on server restart to restore state

`server/config.json` holds server/kiosk/screen configuration and is separate from `settings.json`.

### Kiosk (Wails)

`Development/kiosk/app.go` — on `domReady` (guarded by `sync.Once`), polls `/health` until the server responds, then navigates the Wails window to its screen URL from `config.json`. Screen 0 spawns windows for screens 1+ as subprocesses (production, `--screen N` flag) or browser tabs (dev mode). Connects to `/ws/kiosk` for remote window-control commands.

`Development/kiosk/monitor.go` (Windows-only) uses Win32 APIs (`EnumDisplayMonitors`, `SetWindowPos`) to move windows fullscreen to a specific monitor. Before moving, it calls `window.__kioskBlackout(true)` on the Wails webview to fade to black, then fades back in after repositioning.

`Development/kiosk/numpad.go` (Windows-only, choir screen only) installs a low-level keyboard hook (`WH_KEYBOARD_LL`) that captures numpad input globally — even when the window is unfocused — and forwards digits directly to `/ws/steuerung`. Handles both NumLock-on (VK_NUMPAD0–9) and NumLock-off (navigation keys mapped to digits) states.

The kiosk has its own embedded frontend in `Development/kiosk/frontend/` (separate from `Development/frontend/`).

### Watchdog

`Development/watchdog/main.go` monitors `kiosk.exe`, restarting it after a 3s delay on crash. Also subscribes to `/ws/kiosk` and restarts the kiosk on a `"reload"` command. Reads `config.json` from its own directory or parent.

### Path alias

The frontend uses `@/` as an alias for `frontend/src/` (configured in `vite.config.ts` and `tsconfig.app.json`).

## Configuration

All Go components read **`config.json`** (JSON format) from their working directory or parent directory, creating it with defaults on first run. `Development/config.yaml` is a human-readable reference only — it is **not parsed by any component**.

`server/settings.json` stores display settings separately and is managed exclusively by the server.

Key `config.json` fields:

```json
{
  "server_host": "localhost",
  "port": 1980,
  "dev": true,
  "screens": [
    { "name": "liedanzeige", "url": "/lied", "monitor": 0 },
    { "name": "choranzeige", "url": "/chor", "monitor": 1 }
  ],
  "kiosk": { "always_on_top": true }
}
```

`dev: false` makes kiosk run fullscreen and always-on-top. `server_host` must be the server's LAN IP for multi-machine setups.
