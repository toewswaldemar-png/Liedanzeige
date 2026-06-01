# Uhr v2 — Go + React/TS/Tailwind/shadcn

## Voraussetzungen

1. **Go** installieren: https://go.dev/dl/ (v1.22+)
2. **Wails** installieren (nach Go): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
3. **Node.js** v18+ (bereits vorhanden)

## Starten

### Server (Go)
```bash
cd server
go mod tidy          # einmalig: Dependencies herunterladen
go run .
```
Server läuft auf http://localhost:1980

### Frontend (Dev-Modus)
```bash
cd frontend
npm run dev
```
Dev-Server auf http://localhost:5173 (Proxy zu :1980)

### Frontend (Produktions-Build)
```bash
cd frontend
npm run build        # Output → ../static/
```
Danach wird das Frontend vom Go-Server unter http://localhost:1980 serviert.

## URLs
- Steuerung:   http://localhost:1980/steuerung
- Liedanzeige: http://localhost:1980/liedanzeige?kanal=lied
- Choranzeige: http://localhost:1980/liedanzeige?kanal=chor
- Health:      http://localhost:1980/health

## Projektstruktur
```
Uhr-v2/
├── config.yaml       # Server-Konfiguration
├── server/           # Go HTTP/WebSocket-Server
├── kiosk/            # Wails Kiosk-App (TODO nach Go-Installation)
├── frontend/         # React + TS + Tailwind + shadcn/ui
└── static/           # Vite Build-Output (von Go serviert)
```
