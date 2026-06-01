package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message map[string]any

// Client kapselt eine WebSocket-Verbindung mit Write-Mutex
// um gleichzeitige Schreibzugriffe (Broadcast + Ping) zu verhindern.
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *Client) writeMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(messageType, data)
}

const maxLogHistory = 100

type Hub struct {
	mu              sync.RWMutex
	clients         map[string]map[*Client]bool
	liedState       string
	chorState       string
	steuerungState  string // einheitlicher Anzeigestand für alle Steuerung-Tabs
	settings        DisplaySettings
	cfg             *AppConfig
	settingsPath    string
	settingsMu        sync.Mutex // serialisiert Schreibzugriffe auf settings.json
	settingsLogTimer  *time.Timer
	settingsLogTimeMu sync.Mutex
	monitorsSwapped   bool
	kioskFullscreen bool
	kioskStateKnown bool
	logHistory      []Message
	logMu           sync.Mutex
}

func NewHub(cfg *AppConfig, settings *DisplaySettings, settingsPath string) *Hub {
	return &Hub{
		clients: map[string]map[*Client]bool{
			"lied":      {},
			"chor":      {},
			"steuerung": {},
			"kiosk":     {},
			"log":       {},
		},
		settings:     *settings,
		cfg:          cfg,
		settingsPath: settingsPath,
	}
}

// LogEvent schreibt einen Eintrag ins File-Log, puffert ihn und sendet ihn an alle /ws/log Clients.
func (h *Hub) LogEvent(level, msg string) {
	ts := time.Now().Format("15:04:05")
	log.Printf("[%s] %s", level, msg)
	entry := Message{
		"action":  "log",
		"level":   level,
		"message": msg,
		"ts":      ts,
	}
	h.logMu.Lock()
	h.logHistory = append(h.logHistory, entry)
	if len(h.logHistory) > maxLogHistory {
		h.logHistory = h.logHistory[len(h.logHistory)-maxLogHistory:]
	}
	h.logMu.Unlock()
	h.broadcast("log", entry)
}

func (h *Hub) Register(channel string, client *Client) {
	h.mu.Lock()
	if h.clients[channel] == nil {
		h.clients[channel] = make(map[*Client]bool)
	}
	h.clients[channel][client] = true
	h.mu.Unlock()

	// Log-Kanal: History sofort senden, kein Connect-Event loggen
	if channel == "log" {
		h.logMu.Lock()
		history := make([]Message, len(h.logHistory))
		copy(history, h.logHistory)
		h.logMu.Unlock()
		for _, entry := range history {
			if data, err := json.Marshal(entry); err == nil {
				_ = client.writeMessage(websocket.TextMessage, data)
			}
		}
		return
	}

	h.LogEvent("info", fmt.Sprintf("verbunden: %s", channel))

	if channel == "steuerung" {
		h.mu.RLock()
		known, fullscreen := h.kioskStateKnown, h.kioskFullscreen
		liedState, chorState, settings := h.liedState, h.chorState, h.settings
		steuerungState := h.steuerungState
		h.mu.RUnlock()
		// Aktuellen Stand senden — neuer Tab zeigt sofort korrekten Zustand
		if data, err := json.Marshal(Message{
			"action":         "sync",
			"liedState":      liedState,
			"chorState":      chorState,
			"steuerungState": steuerungState,
			"settings":       settings,
		}); err == nil {
			_ = client.writeMessage(websocket.TextMessage, data)
		}
		if known {
			if data, err := json.Marshal(Message{"action": "kiosk_state", "fullscreen": fullscreen}); err == nil {
				_ = client.writeMessage(websocket.TextMessage, data)
			}
		}
	}

	if channel == "lied" || channel == "chor" {
		h.mu.RLock()
		state := h.liedState
		if channel == "chor" {
			state = h.chorState
		}
		settings := h.settings
		h.mu.RUnlock()

		if data, err := json.Marshal(Message{"action": "display", "value": state}); err == nil {
			_ = client.writeMessage(websocket.TextMessage, data)
		}
		if data, err := json.Marshal(Message{"action": "settings", "settings": settings}); err == nil {
			_ = client.writeMessage(websocket.TextMessage, data)
		}
	}
}

func (h *Hub) Unregister(channel string, client *Client) {
	h.mu.Lock()
	delete(h.clients[channel], client)
	h.mu.Unlock()
	if channel != "log" {
		h.LogEvent("info", fmt.Sprintf("getrennt: %s", channel))
	}
}

func (h *Hub) broadcast(channel string, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients[channel] {
		if err := client.writeMessage(websocket.TextMessage, data); err != nil {
			log.Printf("ws write [%s]: %v", channel, err)
		}
	}
}

func (h *Hub) HandleLogMessage(msg Message) {
	action, _ := msg["action"].(string)
	if action == "clear_log" {
		h.logMu.Lock()
		h.logHistory = h.logHistory[:0]
		h.logMu.Unlock()
	}
}

func (h *Hub) HandleKioskMessage(msg Message) {
	action, _ := msg["action"].(string)
	if action == "kiosk_state" {
		fullscreen, _ := msg["fullscreen"].(bool)
		h.mu.Lock()
		h.kioskFullscreen = fullscreen
		h.kioskStateKnown = true
		h.mu.Unlock()
		state := "Fenster"
		if fullscreen {
			state = "Vollbild"
		}
		h.LogEvent("info", fmt.Sprintf("Kiosk: %s", state))
		h.broadcast("steuerung", msg)
	}
}

func (h *Hub) HandleSteuerung(msg Message) {
	action, _ := msg["action"].(string)
	target, _ := msg["target"].(string)
	if target == "" {
		target = "lied"
	}

	targets := []string{target}
	if target == "lied" {
		targets = []string{"lied", "chor"}
	}

	switch action {
	case "input":
		key, _ := msg["key"].(string)
		h.mu.Lock()
		if target == "chor" {
			if len(h.chorState) < 4 {
				h.chorState += key
			}
		} else {
			if len(h.liedState) < 4 {
				h.liedState += key
				h.chorState = h.liedState
			}
		}
		if len(h.steuerungState) < 4 {
			h.steuerungState += key
		}
		steuerungState := h.steuerungState
		h.mu.Unlock()
		for _, ch := range targets {
			h.broadcast(ch, Message{"action": "input", "key": key})
		}
		h.broadcast("steuerung", Message{"action": "input", "key": key, "target": target, "steuerungState": steuerungState})
		h.LogEvent("info", fmt.Sprintf("Eingabe: '%s' -> %s", key, target))

	case "backspace":
		h.mu.Lock()
		changed := false
		if target == "chor" {
			if len(h.chorState) > 0 {
				h.chorState = h.chorState[:len(h.chorState)-1]
				changed = true
			}
		} else {
			if len(h.liedState) > 0 {
				h.liedState = h.liedState[:len(h.liedState)-1]
				h.chorState = h.liedState
				changed = true
			}
		}
		steuerungChanged := false
		if len(h.steuerungState) > 0 {
			h.steuerungState = h.steuerungState[:len(h.steuerungState)-1]
			steuerungChanged = true
		}
		h.mu.Unlock()
		if changed || steuerungChanged {
			if changed {
				for _, ch := range targets {
					h.broadcast(ch, Message{"action": "backspace"})
				}
			}
			h.broadcast("steuerung", Message{"action": "backspace", "target": target})
			h.LogEvent("info", fmt.Sprintf("Loeschen -> %s", target))
		}

	case "reset":
		h.mu.Lock()
		if target == "chor" {
			h.chorState = ""
		} else {
			h.liedState = ""
			h.chorState = ""
		}
		h.steuerungState = ""
		h.mu.Unlock()
		for _, ch := range targets {
			h.broadcast(ch, Message{"action": "reset"})
		}
		h.broadcast("steuerung", Message{"action": "reset", "target": target})
		h.LogEvent("info", fmt.Sprintf("Reset -> %s", target))

	case "settings":
		if raw, ok := msg["settings"]; ok {
			if b, err := json.Marshal(raw); err == nil {
				var s DisplaySettings
				if err := json.Unmarshal(b, &s); err == nil {
					h.mu.Lock()
					h.settings = s
					h.mu.Unlock()
					go func() {
						h.settingsMu.Lock()
						defer h.settingsMu.Unlock()
						if err := saveSettings(h.settingsPath, s); err != nil {
							h.LogEvent("error", fmt.Sprintf("settings speichern: %v", err))
						}
					}()
					// Debounce: nur bei gültigen Settings und nach Ende der Slider-Bewegung loggen
					h.settingsLogTimeMu.Lock()
					if h.settingsLogTimer != nil {
						h.settingsLogTimer.Stop()
					}
					h.settingsLogTimer = time.AfterFunc(500*time.Millisecond, func() {
						h.LogEvent("info", "Einstellungen aktualisiert")
					})
					h.settingsLogTimeMu.Unlock()
				}
			}
		}
		h.broadcast("lied", msg)
		h.broadcast("chor", msg)

	case "kiosk":
		cmd, _ := msg["command"].(string)
		if cmd == "swap_monitors" && len(h.cfg.Screens) >= 2 {
			h.mu.Lock()
			h.monitorsSwapped = !h.monitorsSwapped
			swapped := h.monitorsSwapped
			h.mu.Unlock()
			for i, screen := range h.cfg.Screens {
				targetMonitor := screen.Monitor
				if swapped {
					j := len(h.cfg.Screens) - 1 - i
					targetMonitor = h.cfg.Screens[j].Monitor
				}
				h.broadcast("kiosk", Message{
					"action":  "kiosk",
					"command": "move_to",
					"screen":  i,
					"monitor": targetMonitor,
				})
			}
			h.LogEvent("info", "Monitore getauscht")
		} else {
			h.broadcast("kiosk", msg)
			h.LogEvent("info", fmt.Sprintf("Kiosk-Befehl: %s", cmd))
		}
	}
}
