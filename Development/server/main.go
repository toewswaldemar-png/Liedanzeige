package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed static
var staticFiles embed.FS

const (
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func setupLogging() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	logPath := filepath.Join(filepath.Dir(exePath), "server.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("log-datei konnte nicht geoeffnet werden: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.Ltime | log.Lshortfile)
}

func main() {
	setupLogging()

	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	settings, err := loadSettings("settings.json")
	if err != nil {
		log.Fatalf("settings: %v", err)
	}

	ensureFirewallRule(cfg.Port)

	hub := NewHub(cfg, settings, "settings.json")
	mux := http.NewServeMux()

	// Health-Check
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	// WebSocket /ws/{channel}
	mux.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/ws/"), "/")
		channel := parts[0]
		if channel == "" {
			http.Error(w, "channel required", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		client := &Client{conn: conn}

		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		pingStop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(pingPeriod)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := client.writeMessage(websocket.PingMessage, nil); err != nil {
						return
					}
				case <-pingStop:
					return
				}
			}
		}()

		hub.Register(channel, client)
		defer func() {
			close(pingStop)
			hub.Unregister(channel, client)
		}()

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if channel == "steuerung" {
				var msg Message
				if json.Unmarshal(data, &msg) == nil {
					hub.HandleSteuerung(msg)
				}
			} else if channel == "kiosk" {
				var msg Message
				if json.Unmarshal(data, &msg) == nil {
					hub.HandleKioskMessage(msg)
				}
			}
		}
	})

	// SPA-Fallback mit eingebettetem static/-Ordner
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	fileServer := http.FileServer(http.FS(staticFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			serveLandingPage(w, r, cfg)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(staticFS, path); err != nil {
			http.ServeFileFS(w, r, staticFS, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	bindAddr := fmt.Sprintf(":%d", cfg.Port)
	hub.LogEvent("info", fmt.Sprintf("Server gestartet: %s:%d", cfg.ServerHost, cfg.Port))
	log.Fatal(http.ListenAndServe(bindAddr, mux))
}
