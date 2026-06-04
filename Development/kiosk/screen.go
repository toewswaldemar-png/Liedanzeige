//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	webview2 "github.com/jchv/go-webview2"
)

const loadingOverlayJS = `(function(){
  var s=document.createElement('style');
  s.textContent='@keyframes spin{to{transform:rotate(360deg)}}@keyframes fade{0%,100%{opacity:.4}50%{opacity:1}}';
  document.head.appendChild(s);
  var d=document.createElement('div');
  d.id='kiosk-loading';
  d.style.cssText='position:fixed;inset:0;background:#fff;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:24px;z-index:9999;';
  d.innerHTML='<div style="width:56px;height:56px;border:4px solid #ddd;border-top-color:#555;border-radius:50%;animation:spin 1s linear infinite"></div><span id="kiosk-status" style="font-family:sans-serif;font-size:16px;color:#000;animation:fade 2s ease-in-out infinite">Verbinde mit Server…</span>';
  document.body.appendChild(d);
  window.__kioskSetStatus=function(m){var e=document.getElementById('kiosk-status');if(e)e.textContent=m;};
  window.__kioskBlackout=function(on){d.style.display=on?'flex':'none';};
})()`

type screenState struct {
	cfg          *Config
	idx          int
	w            webview2.WebView
	hwnd         uintptr
	isFullscreen bool
	monitorIdx   int
	kioskSend    chan map[string]any
	closeAll     func()
	quit         chan struct{}
}

func runScreen(cfg *Config, idx int, closeAll func(), quit chan struct{}) {
	defer closeAll() // wenn dieser Screen endet, alle anderen ebenfalls beenden

	dataPath := filepath.Join(os.TempDir(), fmt.Sprintf("liedanzeige-screen-%d", idx))
	_ = os.RemoveAll(dataPath)

	w := webview2.New(cfg.Dev)
	if w == nil {
		log.Fatalf("[screen %d] WebView2 nicht verfügbar — Runtime installiert?", idx)
	}
	defer w.Destroy()

	monitorIdx := 0
	if idx < len(cfg.Screens) {
		monitorIdx = cfg.Screens[idx].Monitor
	}

	hwnd := uintptr(w.Window())

	title := "Liedanzeige"
	if idx < len(cfg.Screens) {
		title = cfg.Screens[idx].Name
	}
	w.SetTitle(title)

	s := &screenState{
		cfg:        cfg,
		idx:        idx,
		w:          w,
		hwnd:       hwnd,
		monitorIdx: monitorIdx,
		kioskSend:  make(chan map[string]any, 4),
		closeAll:   closeAll,
		quit:       quit,
	}

	if !cfg.Dev {
		if r, ok := s.monitorRect(); ok {
			cascade := idx * 40
			setWindowPosHWND(hwnd, monitorRect{X: r.X + cascade, Y: r.Y + cascade, W: r.W / 2, H: r.H / 2}, false)
		}
	}

	subclassClose(hwnd, closeAll)

	w.Init(loadingOverlayJS)
	w.Navigate("about:blank")

	go s.waitAndLoad()

	go func() {
		<-quit
		w.Dispatch(w.Terminate)
	}()

	if s.isChorScreen() {
		s.startNumpadHook()
	}

	go s.connectKioskWS()

	w.Run()
}

func (s *screenState) monitorRect() (monitorRect, bool) {
	rects := getMonitorRects()
	if len(rects) == 0 {
		return monitorRect{}, false
	}
	idx := s.monitorIdx
	if idx >= len(rects) {
		log.Printf("[screen %d] Monitor %d nicht verfügbar (%d erkannt) — verwende Monitor 0", s.idx, idx, len(rects))
		idx = 0
	}
	return rects[idx], true
}

func (s *screenState) screenURL() string {
	path := "/lied"
	if s.idx < len(s.cfg.Screens) {
		path = s.cfg.Screens[s.idx].URL
	}
	if s.cfg.Dev {
		return fmt.Sprintf("http://localhost:5173%s", path)
	}
	return fmt.Sprintf("http://%s:%d%s", s.cfg.ServerHost, s.cfg.Port, path)
}

func (s *screenState) waitAndLoad() {
	s.w.Dispatch(func() {
		s.w.Eval(fmt.Sprintf("window.__kioskSetStatus&&window.__kioskSetStatus(%q)", "Verbinde mit Server…"))
	})

	for {
		select {
		case <-s.quit:
			return
		default:
		}
		if quickHealthCheck(s.cfg.ServerHost, s.cfg.Port, 2*time.Second) {
			break
		}
		s.w.Dispatch(func() {
			s.w.Eval(fmt.Sprintf("window.__kioskSetStatus&&window.__kioskSetStatus(%q)", "Suche Server…"))
		})
		log.Printf("[screen %d] %s:%d nicht erreichbar — UDP-Broadcast...", s.idx, s.cfg.ServerHost, s.cfg.Port)
		if host, port, ok := discoverServer(3 * time.Second); ok {
			log.Printf("[screen %d] Server gefunden — %s:%d", s.idx, host, port)
			s.cfg.ServerHost = host
			s.cfg.Port = port
			saveConfig(s.cfg)
		} else {
			log.Printf("[screen %d] Warte auf Server...", s.idx)
			time.Sleep(2 * time.Second)
		}
		s.w.Dispatch(func() {
			s.w.Eval(fmt.Sprintf("window.__kioskSetStatus&&window.__kioskSetStatus(%q)", "Verbinde mit Server…"))
		})
	}

	url := s.screenURL()
	log.Printf("[screen %d] Navigiere zu: %s", s.idx, url)
	s.w.Dispatch(func() { s.w.Navigate(url) })

	if !s.cfg.Dev {
		time.Sleep(300 * time.Millisecond)
		if loadWindowState(s.idx) {
			if r, ok := s.monitorRect(); ok {
				cascade := s.idx * 40
				setWindowPosHWND(s.hwnd, monitorRect{X: r.X + cascade, Y: r.Y + cascade, W: r.W / 2, H: r.H / 2}, false)
			}
		} else {
			s.goFullscreen()
		}
	}
}

func (s *screenState) goFullscreen() {
	s.isFullscreen = true
	saveWindowState(s.idx, false)
	setWindowFrameHWND(s.hwnd, false)
	if r, ok := s.monitorRect(); ok {
		setWindowPosHWND(s.hwnd, r, true) // immer HWND_TOPMOST im Vollbild — Taskleiste liegt darunter
	}
	select {
	case s.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": true}:
	default:
	}
}

func (s *screenState) goWindowed() {
	s.isFullscreen = false
	saveWindowState(s.idx, true)
	setWindowFrameHWND(s.hwnd, true)
	go func() {
		time.Sleep(100 * time.Millisecond) // kurz warten bis Windows den Rahmen neu gezeichnet hat
		if r, ok := s.monitorRect(); ok {
			cascade := s.idx * 40
			setWindowPosHWND(s.hwnd, monitorRect{X: r.X + cascade, Y: r.Y + cascade, W: r.W / 2, H: r.H / 2}, false)
		}
	}()
	select {
	case s.kioskSend <- map[string]any{"action": "kiosk_state", "fullscreen": false}:
	default:
	}
}

func (s *screenState) moveWithBlackout(r monitorRect) {
	s.w.Dispatch(func() {
		s.w.Eval(`window.__kioskBlackout&&window.__kioskBlackout(true)`)
	})
	time.Sleep(50 * time.Millisecond)
	if s.isFullscreen {
		setWindowPosHWND(s.hwnd, r, true)
	} else {
		cascade := s.idx * 40
		setWindowPosHWND(s.hwnd, monitorRect{X: r.X + cascade, Y: r.Y + cascade, W: r.W / 2, H: r.H / 2}, false)
	}
	time.Sleep(80 * time.Millisecond)
	s.w.Dispatch(func() {
		s.w.Eval(`window.__kioskBlackout&&window.__kioskBlackout(false)`)
	})
}

// handleKioskCommand wird direkt aus dem WS-Goroutine aufgerufen.
// Win32-Ops sind thread-sicher; WebView2-Ops nutzen w.Dispatch.
func (s *screenState) handleKioskCommand(msg map[string]any) {
	cmd, _ := msg["command"].(string)
	switch cmd {
	case "fullscreen":
		s.goFullscreen()

	case "windowed":
		s.goWindowed()

	case "reload":
		url := s.screenURL()
		s.w.Dispatch(func() { s.w.Navigate(url) })

	case "move_to":
		screenF, _ := msg["screen"].(float64)
		if int(screenF) != s.idx {
			return
		}
		monitorF, _ := msg["monitor"].(float64)
		monitorIdx := int(monitorF)
		rects := getMonitorRects()
		if monitorIdx >= len(rects) {
			return
		}
		s.monitorIdx = monitorIdx
		go s.moveWithBlackout(rects[monitorIdx])

	case "next_monitor":
		rects := getMonitorRects()
		if len(rects) <= 1 {
			return
		}
		s.monitorIdx = (s.monitorIdx + 1) % len(rects)
		r := rects[s.monitorIdx]
		log.Printf("[screen %d] Monitor gewechselt → %d (%d,%d)", s.idx, s.monitorIdx, r.X, r.Y)
		go s.moveWithBlackout(r)

	case "quit":
		log.Printf("[screen %d] quit empfangen", s.idx)
		s.closeAll()
	}
}

func (s *screenState) connectKioskWS() {
	url := fmt.Sprintf("ws://%s:%d/ws/kiosk", s.cfg.ServerHost, s.cfg.Port)
	for {
		select {
		case <-s.quit:
			return
		default:
		}
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Printf("[screen %d] kiosk ws: %v — retry in 2s", s.idx, err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("[screen %d] kiosk ws: verbunden", s.idx)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					log.Printf("[screen %d] kiosk ws read: %v", s.idx, err)
					return
				}
				var msg map[string]any
				if json.Unmarshal(data, &msg) == nil {
					s.handleKioskCommand(msg)
				}
			}
		}()

	send:
		for {
			select {
			case msg := <-s.kioskSend:
				data, _ := json.Marshal(msg)
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					select {
					case s.kioskSend <- msg:
					default:
					}
					break send
				}
			case <-done:
				break send
			case <-s.quit:
				conn.Close()
				return
			}
		}

		conn.Close()
		<-done
		time.Sleep(2 * time.Second)
	}
}

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
