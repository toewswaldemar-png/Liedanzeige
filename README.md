# Liedanzeige — Go + React/TS/Tailwind/shadcn

Kirchliches Anzeigesystem für Lied- und Chornummern sowie eine Uhr auf Präsentationsbildschirmen.

## Komponenten

| Verzeichnis | Beschreibung |
|-------------|--------------|
| `Development/server/` | Go HTTP + WebSocket Server (Port 1980); bettet Frontend zur Compile-Zeit ein |
| `Development/frontend/` | React 19 + TypeScript + Tailwind + shadcn/ui SPA (Vite) |
| `Development/kiosk/` | Go Kiosk-App (kein Wails); Win32 + WebView2 direkt via `go-webview2` |

## Voraussetzungen

- **Go** v1.22+: https://go.dev/dl/
- **Node.js** v18+
- **goversioninfo** (für Windows-Icon-Einbettung): `go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest`

## Build

### Alles auf einmal
```bat
build.bat
```

### Nur Server (Frontend + Go)
```bat
build-server.bat
```

### Nur Kiosk
```bat
build-kiosk.bat
```

Ausgabe in `_build/`:
```
_build/
├── Server/
│   └── Liedanzeige.exe
└── Kiosk/
    └── Kiosk.exe
```

> **Hinweis:** Der Go-Server bettet das Frontend per `//go:embed static` ein. Nach Frontend-Änderungen muss der Server neu gebaut werden — `npm run build` allein reicht nicht. `build-server.bat` erledigt beides in einem Schritt.

## Entwicklung (einzelne Komponenten)

```bash
cd Development/server && go run .
cd Development/frontend && npm run dev   # :5173, Proxy → :1980
cd Development/kiosk && go run .         # Windows only; Server muss laufen
```

## URLs

| URL | Zweck |
|-----|-------|
| `http://localhost:1980/` | Startseite mit allen Links |
| `http://localhost:1980/steuerung/lied` | Steuerung Liedanzeige |
| `http://localhost:1980/steuerung/chor` | Steuerung Choranzeige |
| `http://localhost:1980/lied` | Liedanzeige (Beamer) |
| `http://localhost:1980/chor` | Choranzeige (Beamer) |
| `http://localhost:1980/health` | Health-Check |

## Konfiguration

`config.json` im jeweiligen `_build/`-Unterordner ablegen. Vorlage: `config.example.json` im Repo-Root.

Wichtige Felder:
```json
{
  "server_host": "192.168.1.100",
  "port": 1980,
  "screens": [
    { "name": "liedanzeige", "url": "/lied",  "monitor": 1 },
    { "name": "choranzeige", "url": "/chor",  "monitor": 0 }
  ]
}
```

`server_host` muss die LAN-IP sein bei Mehrrechnerbetrieb. Ohne `config.json` startet der Kiosk mit Defaults (`localhost:1980`).
