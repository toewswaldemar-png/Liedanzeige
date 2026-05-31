package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx               context.Context
	cfg               *Config
	screenIdx         int
	loadOnce          sync.Once
	currentMonitorIdx int
	kioskSend         chan map[string]any
}

func NewApp(cfg *Config, screenIdx int) *App {
	monitorIdx := 0
	if screenIdx < len(cfg.Screens) {
		monitorIdx = cfg.Screens[screenIdx].Monitor
	}
	return &App{
		cfg:               cfg,
		screenIdx:         screenIdx,
		currentMonitorIdx: monitorIdx,
		kioskSend:         make(chan map[string]any, 4),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) domReady(ctx context.Context) {
	a.positionOnConfiguredMonitor()
	// sync.Once stellt sicher, dass Navigation nur beim ersten domReady ausgeführt wird,
	// nicht bei jeder weiteren Seitennavigation.
	a.loadOnce.Do(func() {
		go a.waitForServerThenLoad()
	})
}

// screenTargetURL gibt die vollständige URL für den Screen-Index zurück
func (a *App) screenTargetURL(idx int) string {
	screenURL := "/lied"
	if idx < len(a.cfg.Screens) {
		screenURL = a.cfg.Screens[idx].URL
	}
	if a.cfg.Dev {
		return fmt.Sprintf("http://localhost:5173%s", screenURL)
	}
	return fmt.Sprintf("http://%s:%d%s", a.cfg.ServerHost, a.cfg.Port, screenURL)
}

const loadingOverlayJS = `(function(){
  var s = document.createElement('style');
  s.textContent = '@keyframes spin{to{transform:rotate(360deg)}}@keyframes fade{0%,100%{opacity:.4}50%{opacity:1}}';
  document.head.appendChild(s);
  var d = document.createElement('div');
  d.id = 'kiosk-loading';
  d.style.cssText = 'position:fixed;inset:0;background:#fff;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:24px;z-index:9999;';
  d.innerHTML =
    '<div style="width:56px;height:56px;border:4px solid #ddd;border-top-color:#555;border-radius:50%;animation:spin 1s linear infinite"></div>' +
    '<span style="font-family:sans-serif;font-size:16px;color:#000;animation:fade 2s ease-in-out infinite">Verbinde mit Server…</span>';
  document.body.appendChild(d);
})()`

// Warten bis der Server erreichbar ist, dann zur Anzeige-URL navigieren
func (a *App) waitForServerThenLoad() {
	runtime.WindowExecJS(a.ctx, loadingOverlayJS)

	healthURL := fmt.Sprintf("http://%s:%d/health", a.cfg.ServerHost, a.cfg.Port)
	for {
		resp, err := http.Get(healthURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			break
		}
		log.Println("Warte auf Server...")
		time.Sleep(2 * time.Second)
	}

	// Dieses Fenster navigieren
	targetURL := a.screenTargetURL(a.screenIdx)
	log.Printf("[screen %d] Navigiere zu: %s", a.screenIdx, targetURL)
	runtime.WindowExecJS(a.ctx, fmt.Sprintf("window.location.href = %q", targetURL))

	// Vollbild per Win32 setzen — runtime.WindowFullscreen nutzt die Work-Area
	// (Monitor ohne Taskleiste) und würde die Taskleiste nicht abdecken.
	if !a.cfg.Dev {
		time.Sleep(300 * time.Millisecond)
		rects := getMonitorRects()
		if idx := a.currentMonitorIdx; idx < len(rects) {
			moveWindowFullscreenToMonitor(rects[idx])
		}
		runtime.WindowSetAlwaysOnTop(a.ctx, a.cfg.Kiosk.AlwaysOnTop)
		// State an Steuerung melden — wird über connectKioskWS gesendet sobald verbunden
		select {
		case a.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": true}:
		default:
		}
	}

	if a.screenIdx == 0 {
		if a.cfg.Dev {
			// Im Dev-Modus: weitere Screens als Browser-Tab öffnen
			for i := 1; i < len(a.cfg.Screens); i++ {
				runtime.BrowserOpenURL(a.ctx, a.screenTargetURL(i))
			}
		} else {
			// Im Production-Modus: weitere Fenster als eigene Prozesse spawnen
			exe, err := os.Executable()
			if err != nil {
				log.Printf("os.Executable: %v", err)
			} else {
				for i := 1; i < len(a.cfg.Screens); i++ {
					cmd := exec.Command(exe, fmt.Sprintf("--screen=%d", i))
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					if err := cmd.Start(); err != nil {
						log.Printf("Fenster %d starten: %v", i, err)
					} else {
						log.Printf("Fenster %d gestartet (PID %d)", i, cmd.Process.Pid)
					}
				}
			}
		}
	}

	// Globalen Numpad-Hook nur für den Chor-Bildschirm starten
	if a.isChorScreen() {
		a.startNumpadHook()
	}

	go a.connectKioskWS()
}

// WebSocket-Verbindung zum /ws/kiosk Channel (bidirektional)
func (a *App) connectKioskWS() {
	url := fmt.Sprintf("ws://%s:%d/ws/kiosk", a.cfg.ServerHost, a.cfg.Port)
	for {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Printf("kiosk ws: %v — retry in 2s", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Println("kiosk ws: verbunden")

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					log.Printf("kiosk ws read: %v", err)
					return
				}
				var msg map[string]any
				if json.Unmarshal(data, &msg) == nil {
					a.handleKioskCommand(msg)
				}
			}
		}()

	send:
		for {
			select {
			case msg := <-a.kioskSend:
				data, _ := json.Marshal(msg)
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					select {
					case a.kioskSend <- msg:
					default:
					}
					break send
				}
			case <-done:
				break send
			}
		}

		conn.Close()
		<-done
		time.Sleep(2 * time.Second)
	}
}

func (a *App) handleKioskCommand(msg map[string]any) {
	cmd, _ := msg["command"].(string)
	switch cmd {
	case "fullscreen":
		runtime.WindowSetAlwaysOnTop(a.ctx, a.cfg.Kiosk.AlwaysOnTop)
		go func() {
			rects := getMonitorRects()
			if idx := a.currentMonitorIdx; idx < len(rects) {
				moveWindowFullscreenToMonitor(rects[idx])
			}
		}()
		select {
		case a.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": true}:
		default:
		}
	case "windowed":
		select {
		case a.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": false}:
		default:
		}
		go func() {
			runtime.WindowUnfullscreen(a.ctx)
			runtime.WindowSetAlwaysOnTop(a.ctx, false)
			time.Sleep(150 * time.Millisecond)
			rects := getMonitorRects()
			idx := a.currentMonitorIdx
			if idx >= len(rects) {
				idx = 0
			}
			if len(rects) > 0 {
				r := rects[idx]
				w, h := r.W/2, r.H/2
				cascade := a.screenIdx * 40
				x := r.X + cascade
				y := r.Y + cascade
				// Win32 direkt statt Wails-API: konsistente physikalische Koordinaten
				// mit getMonitorRects() (EnumDisplayMonitors).
				positionWindowWindowed(x, y, w, h)
			}
		}()
	case "toggle_fullscreen":
		go func() {
			rects := getMonitorRects()
			if idx := a.currentMonitorIdx; idx < len(rects) {
				moveWindowFullscreenToMonitor(rects[idx])
			}
		}()
	case "move_to":
		screenF, _ := msg["screen"].(float64)
		if int(screenF) != a.screenIdx {
			return // Befehl gilt für ein anderes Fenster
		}
		monitorF, _ := msg["monitor"].(float64)
		monitorIdx := int(monitorF)
		rects := getMonitorRects()
		if monitorIdx >= len(rects) {
			return
		}
		a.currentMonitorIdx = monitorIdx
		r := rects[monitorIdx]
		go func() {
			runtime.WindowExecJS(a.ctx, `window.__kioskBlackout&&window.__kioskBlackout(true)`)
			time.Sleep(50 * time.Millisecond)
			moveWindowFullscreenToMonitor(r)
			time.Sleep(80 * time.Millisecond)
			runtime.WindowExecJS(a.ctx, `window.__kioskBlackout&&window.__kioskBlackout(false)`)
		}()

	case "next_monitor":
		rects := getMonitorRects()
		if len(rects) <= 1 {
			return
		}
		a.currentMonitorIdx = (a.currentMonitorIdx + 1) % len(rects)
		r := rects[a.currentMonitorIdx]
		log.Printf("[screen %d] Monitor gewechselt → %d (%d,%d)", a.screenIdx, a.currentMonitorIdx, r.X, r.Y)
		go func() {
			runtime.WindowExecJS(a.ctx, `window.__kioskBlackout&&window.__kioskBlackout(true)`)
			time.Sleep(50 * time.Millisecond)
			moveWindowFullscreenToMonitor(r)
			time.Sleep(80 * time.Millisecond)
			runtime.WindowExecJS(a.ctx, `window.__kioskBlackout&&window.__kioskBlackout(false)`)
		}()
	}
}

// ServerURL gibt die Basis-URL zurück (wird im Frontend verwendet)
func (a *App) ServerURL() string {
	return fmt.Sprintf("http://%s:%d", a.cfg.ServerHost, a.cfg.Port)
}
