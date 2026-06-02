package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	isFullscreen      bool
}

// windowStatePath gibt den Pfad der Zustandsdatei für diesen Screen zurück.
func windowStatePath(screenIdx int) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("liedanzeige-screen-%d-state.json", screenIdx))
}

func saveWindowState(screenIdx int, windowed bool) {
	data, _ := json.Marshal(map[string]bool{"windowed": windowed})
	_ = os.WriteFile(windowStatePath(screenIdx), data, 0644)
}

func loadWindowState(screenIdx int) (windowed bool) {
	data, err := os.ReadFile(windowStatePath(screenIdx))
	if err != nil {
		return false
	}
	var m map[string]bool
	if json.Unmarshal(data, &m) == nil {
		return m["windowed"]
	}
	return false
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
	if !a.cfg.Dev {
		if r, ok := a.monitorRect(); ok {
			cascade := a.screenIdx * 40
			runtime.WindowSetSize(ctx, r.W/2, r.H/2)
			runtime.WindowSetPosition(ctx, r.X+cascade, r.Y+cascade)
		}
		runtime.WindowShow(ctx)
	}
}

func (a *App) monitorRect() (monitorRect, bool) {
	rects := getMonitorRects()
	if len(rects) == 0 {
		return monitorRect{}, false
	}
	idx := a.currentMonitorIdx
	if idx >= len(rects) {
		log.Printf("[screen %d] Monitor %d nicht verfügbar (%d erkannt) — verwende Monitor 0", a.screenIdx, idx, len(rects))
		idx = 0
	}
	return rects[idx], true
}

func (a *App) domReady(ctx context.Context) {
	if a.cfg.Dev {
		if r, ok := a.monitorRect(); ok {
			runtime.WindowSetPosition(a.ctx, r.X, r.Y)
		}
	}
	a.loadOnce.Do(func() {
		go a.waitForServerThenLoad()
	})
}

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

func (a *App) waitForServerThenLoad() {
	runtime.WindowExecJS(a.ctx, loadingOverlayJS)

	healthURL := fmt.Sprintf("http://%s:%d/health", a.cfg.ServerHost, a.cfg.Port)
	for {
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		log.Println("Warte auf Server...")
		time.Sleep(2 * time.Second)
	}

	targetURL := a.screenTargetURL(a.screenIdx)
	log.Printf("[screen %d] Navigiere zu: %s", a.screenIdx, targetURL)
	runtime.WindowExecJS(a.ctx, fmt.Sprintf("window.location.href = %q", targetURL))

	if !a.cfg.Dev {
		time.Sleep(300 * time.Millisecond)
		if loadWindowState(a.screenIdx) {
			if r, ok := a.monitorRect(); ok {
				cascade := a.screenIdx * 40
				setWindowPos(monitorRect{X: r.X + cascade, Y: r.Y + cascade, W: r.W / 2, H: r.H / 2}, false)
			}
		} else {
			a.goFullscreen()
		}
	}

	if a.screenIdx == 0 && a.cfg.Dev {
		for i := 1; i < len(a.cfg.Screens); i++ {
			runtime.BrowserOpenURL(a.ctx, a.screenTargetURL(i))
		}
	}

	if a.isChorScreen() {
		a.startNumpadHook()
	}

	startQuitShortcut(func() {
		select {
		case a.kioskSend <- map[string]any{"action": "kiosk", "command": "quit"}:
		default:
			runtime.Quit(a.ctx)
		}
	})
	go a.connectKioskWS()
}

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

// goFullscreen entfernt den Fensterrahmen per Win32 und positioniert das Fenster vollbildfüllend.
func (a *App) goFullscreen() {
	a.isFullscreen = true
	saveWindowState(a.screenIdx, false)
	setWindowFrame(false)
	if r, ok := a.monitorRect(); ok {
		setWindowPos(r, a.cfg.Kiosk.AlwaysOnTop)
	}
	select {
	case a.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": true}:
	default:
	}
}

func (a *App) moveWithBlackout(r monitorRect) {
	runtime.WindowExecJS(a.ctx, `window.__kioskBlackout&&window.__kioskBlackout(true)`)
	time.Sleep(50 * time.Millisecond)
	if a.isFullscreen {
		setWindowPos(r, a.cfg.Kiosk.AlwaysOnTop)
	} else {
		// Fenstermodus: halbe Monitorgröße, kein Topmost, Rahmen bleibt
		cascade := a.screenIdx * 40
		wr := monitorRect{X: r.X + cascade, Y: r.Y + cascade, W: r.W / 2, H: r.H / 2}
		setWindowPos(wr, false)
	}
	time.Sleep(80 * time.Millisecond)
	runtime.WindowExecJS(a.ctx, `window.__kioskBlackout&&window.__kioskBlackout(false)`)
}

func (a *App) handleKioskCommand(msg map[string]any) {
	cmd, _ := msg["command"].(string)
	switch cmd {
	case "fullscreen":
		go a.goFullscreen()

	case "windowed":
		a.isFullscreen = false
		saveWindowState(a.screenIdx, true)
		select {
		case a.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": false}:
		default:
		}
		go func() {
			runtime.WindowSetAlwaysOnTop(a.ctx, false)
			// Rahmen wiederherstellen → echte Windows-Titelleiste erscheint
			setWindowFrame(true)
			time.Sleep(100 * time.Millisecond)
			if r, ok := a.monitorRect(); ok {
				cascade := a.screenIdx * 40
				setWindowPos(monitorRect{X: r.X + cascade, Y: r.Y + cascade, W: r.W / 2, H: r.H / 2}, false)
			}
		}()

	case "move_to":
		screenF, _ := msg["screen"].(float64)
		if int(screenF) != a.screenIdx {
			return
		}
		monitorF, _ := msg["monitor"].(float64)
		monitorIdx := int(monitorF)
		rects := getMonitorRects()
		if monitorIdx >= len(rects) {
			return
		}
		a.currentMonitorIdx = monitorIdx
		go a.moveWithBlackout(rects[monitorIdx])

	case "next_monitor":
		rects := getMonitorRects()
		if len(rects) <= 1 {
			return
		}
		a.currentMonitorIdx = (a.currentMonitorIdx + 1) % len(rects)
		r := rects[a.currentMonitorIdx]
		log.Printf("[screen %d] Monitor gewechselt → %d (%d,%d)", a.screenIdx, a.currentMonitorIdx, r.X, r.Y)
		go a.moveWithBlackout(r)

	case "quit":
		log.Printf("[screen %d] Beende Prozess", a.screenIdx)
		runtime.Quit(a.ctx)
	}
}

// beforeClose wird von Wails beim Klick auf X aufgerufen.
// Exit-Code 100 signalisiert dem Supervisor: bewusstes Beenden, kein Neustart.
func (a *App) beforeClose(ctx context.Context) bool {
	os.Exit(100)
	return false
}

// ServerURL gibt die Basis-URL zurück (wird im Frontend verwendet)
func (a *App) ServerURL() string {
	return fmt.Sprintf("http://%s:%d", a.cfg.ServerHost, a.cfg.Port)
}
