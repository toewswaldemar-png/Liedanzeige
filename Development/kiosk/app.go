package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

// monitorRect gibt den Rect des konfigurierten Monitors zurück.
// Fällt auf Monitor 0 zurück wenn der Index nicht verfügbar ist.
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
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		log.Println("Warte auf Server...")
		time.Sleep(2 * time.Second)
	}

	// Dieses Fenster navigieren
	targetURL := a.screenTargetURL(a.screenIdx)
	log.Printf("[screen %d] Navigiere zu: %s", a.screenIdx, targetURL)
	runtime.WindowExecJS(a.ctx, fmt.Sprintf("window.location.href = %q", targetURL))

	if !a.cfg.Dev {
		time.Sleep(300 * time.Millisecond)
		a.goFullscreen()
	}

	if a.screenIdx == 0 && a.cfg.Dev {
		// Im Dev-Modus: weitere Screens als Browser-Tab öffnen
		for i := 1; i < len(a.cfg.Screens); i++ {
			runtime.BrowserOpenURL(a.ctx, a.screenTargetURL(i))
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

// goFullscreen setzt Vollbild und meldet den State.
func (a *App) goFullscreen() {
	stopTitleBarHook()
	runtime.WindowExecJS(a.ctx, `var d=document.getElementById('__kiosk_tb__');if(d)d.remove();`)
	if r, ok := a.monitorRect(); ok {
		setWindowPos(r, true)
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, a.cfg.Kiosk.AlwaysOnTop)
	select {
	case a.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": true}:
	default:
	}
}

// moveWithBlackout blendet kurz ab, wechselt Monitor und blendet wieder ein.
func (a *App) moveWithBlackout(r monitorRect) {
	runtime.WindowExecJS(a.ctx, `window.__kioskBlackout&&window.__kioskBlackout(true)`)
	time.Sleep(50 * time.Millisecond)
	setWindowPos(r, true)
	time.Sleep(80 * time.Millisecond)
	runtime.WindowExecJS(a.ctx, `window.__kioskBlackout&&window.__kioskBlackout(false)`)
}

func (a *App) handleKioskCommand(msg map[string]any) {
	cmd, _ := msg["command"].(string)
	switch cmd {
	case "fullscreen":
		go a.goFullscreen()
	case "windowed":
		select {
		case a.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": false}:
		default:
		}
		go func() {
			runtime.WindowSetAlwaysOnTop(a.ctx, false)
			time.Sleep(150 * time.Millisecond)
			if r, ok := a.monitorRect(); ok {
				cascade := a.screenIdx * 40
				setWindowPos(monitorRect{X: r.X + cascade, Y: r.Y + cascade, W: r.W / 2, H: r.H / 2}, false)
			}
			startTitleBarHook()
			runtime.WindowExecJS(a.ctx, `
				(function(){
					if(document.getElementById('__kiosk_tb__')) return;
					var d=document.createElement('div');
					d.id='__kiosk_tb__';
					d.style.cssText='position:fixed;top:0;left:0;right:0;height:28px;background:rgba(20,20,20,0.92);z-index:2147483647;display:flex;align-items:center;box-sizing:border-box;backdrop-filter:blur(6px);user-select:none;-webkit-user-select:none;';
					var title=document.createElement('span');
					title.style.cssText='flex:1;padding:0 12px;color:rgba(255,255,255,0.6);font-size:11px;font-family:system-ui,sans-serif;letter-spacing:0.06em;pointer-events:none;';
					title.textContent='Liedanzeige';
					d.appendChild(title);
					[['–',false],['□',false],['×',true]].forEach(function(b){
						var btn=document.createElement('div');
						btn.style.cssText='width:46px;height:28px;display:flex;align-items:center;justify-content:center;color:rgba(255,255,255,0.7);font-size:13px;pointer-events:none;';
						btn.textContent=b[0];
						d.appendChild(btn);
					});
					document.body.appendChild(d);
				})()
			`)
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
		r := rects[monitorIdx]
		go a.moveWithBlackout(r)
	case "next_monitor":
		rects := getMonitorRects()
		if len(rects) <= 1 {
			return
		}
		a.currentMonitorIdx = (a.currentMonitorIdx + 1) % len(rects)
		r := rects[a.currentMonitorIdx]
		log.Printf("[screen %d] Monitor gewechselt → %d (%d,%d)", a.screenIdx, a.currentMonitorIdx, r.X, r.Y)
		go a.moveWithBlackout(r)
	}
}

// ServerURL gibt die Basis-URL zurück (wird im Frontend verwendet)
func (a *App) ServerURL() string {
	return fmt.Sprintf("http://%s:%d", a.cfg.ServerHost, a.cfg.Port)
}
