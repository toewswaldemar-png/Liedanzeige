package main

import (
	"encoding/json"
	"log"
	"sync"

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

type Hub struct {
	mu                sync.RWMutex
	clients           map[string]map[*Client]bool
	liedState         string
	chorState         string
	settings          DisplaySettings
	cfg               *AppConfig
	settingsPath      string
	settingsMu        sync.Mutex // serialisiert Schreibzugriffe auf settings.json
	monitorsSwapped   bool
	kioskFullscreen   bool
	kioskStateKnown   bool
}

func NewHub(cfg *AppConfig, settings *DisplaySettings, settingsPath string) *Hub {
	return &Hub{
		clients: map[string]map[*Client]bool{
			"lied":      {},
			"chor":      {},
			"steuerung": {},
			"kiosk":     {},
		},
		settings:     *settings,
		cfg:          cfg,
		settingsPath: settingsPath,
	}
}

func (h *Hub) Register(channel string, client *Client) {
	h.mu.Lock()
	if h.clients[channel] == nil {
		h.clients[channel] = make(map[*Client]bool)
	}
	h.clients[channel][client] = true
	h.mu.Unlock()

	if channel == "steuerung" {
		h.mu.RLock()
		known, fullscreen := h.kioskStateKnown, h.kioskFullscreen
		h.mu.RUnlock()
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

func (h *Hub) HandleKioskMessage(msg Message) {
	action, _ := msg["action"].(string)
	if action == "kiosk_state" {
		fullscreen, _ := msg["fullscreen"].(bool)
		h.mu.Lock()
		h.kioskFullscreen = fullscreen
		h.kioskStateKnown = true
		h.mu.Unlock()
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
		h.mu.Unlock()
		for _, ch := range targets {
			h.broadcast(ch, Message{"action": "input", "key": key})
		}
		h.broadcast("steuerung", Message{"action": "input", "key": key, "target": target})

	case "backspace":
		h.mu.Lock()
		if target == "chor" {
			if len(h.chorState) > 0 {
				h.chorState = h.chorState[:len(h.chorState)-1]
			}
		} else {
			if len(h.liedState) > 0 {
				h.liedState = h.liedState[:len(h.liedState)-1]
				h.chorState = h.liedState
			}
		}
		h.mu.Unlock()
		for _, ch := range targets {
			h.broadcast(ch, Message{"action": "backspace"})
		}
		h.broadcast("steuerung", Message{"action": "backspace", "target": target})

	case "reset":
		h.mu.Lock()
		if target == "chor" {
			h.chorState = ""
		} else {
			h.liedState = ""
			h.chorState = ""
		}
		h.mu.Unlock()
		for _, ch := range targets {
			h.broadcast(ch, Message{"action": "reset"})
		}
		h.broadcast("steuerung", Message{"action": "reset", "target": target})

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
							log.Printf("settings speichern: %v", err)
						}
					}()
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
		} else {
			h.broadcast("kiosk", msg)
		}
	}
}
