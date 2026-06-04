package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
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

var version = "dev"

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // direkte Go-Verbindungen (Kiosk, Watchdog) haben keinen Origin-Header
		}
		// Localhost immer erlauben (Dev-Modus, Vite-Proxy)
		if strings.HasPrefix(origin, "http://localhost") ||
			strings.HasPrefix(origin, "http://127.0.0.1") {
			return true
		}
		// Production: Origin muss mit dem Server-Host übereinstimmen
		return strings.HasPrefix(origin, "http://"+r.Host) ||
			strings.HasPrefix(origin, "https://"+r.Host)
	},
}

// timestampWriter hängt "2006-01-02 15:04:05 " vor jede Log-Zeile.
type timestampWriter struct{ w io.Writer }

func (t timestampWriter) Write(p []byte) (int, error) {
	prefix := []byte(time.Now().Format("2006-01-02 15:04:05 "))
	_, _ = t.w.Write(prefix)
	return t.w.Write(p)
}

func setupLogging() {
	// "server.log" liegt im Arbeitsverzeichnis – konsistent mit config.json/settings.json.
	// Auf Windows: CWD = Verzeichnis der exe (Explorer-Start / .bat).
	// In Docker:  CWD = /data  (Volume-Mount).
	f, err := os.OpenFile("server.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("log-datei konnte nicht geoeffnet werden: %v", err)
		return
	}
	w := timestampWriter{w: io.MultiWriter(os.Stdout, f)}
	log.SetOutput(w)
	log.SetFlags(log.Lshortfile) // Datum/Zeit kommt vom timestampWriter
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
	ensureDiscoveryFirewallRule()

	hub := NewHub(cfg, settings, "settings.json")
	mux := http.NewServeMux()

	// Health-Check
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	// Version
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`, version)
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

		addr := conn.RemoteAddr().String()
		if host, _, err := net.SplitHostPort(addr); err == nil {
			addr = host
		}
		if addr == "::1" || addr == "127.0.0.1" {
			addr = "localhost"
		}
		client := &Client{conn: conn, addr: addr}

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
			} else if channel == "chor" {
				var msg Message
				if json.Unmarshal(data, &msg) == nil {
					// Numpad-Fallback vom Chor-Display: input/reset weiterleiten
					if action, _ := msg["action"].(string); action == "input" || action == "reset" {
						hub.HandleSteuerung(msg)
					}
				}
			} else if channel == "kiosk" {
				var msg Message
				if json.Unmarshal(data, &msg) == nil {
					hub.HandleKioskMessage(msg)
				}
			} else if channel == "log" {
				var msg Message
				if json.Unmarshal(data, &msg) == nil {
					hub.HandleLogMessage(msg)
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
			serveLandingPage(w, r, cfg, version)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(staticFS, path); err != nil {
			http.ServeFileFS(w, r, staticFS, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	go startDiscoveryListener(cfg.ServerHost, cfg.Port)

	bindAddr := fmt.Sprintf(":%d", cfg.Port)
	go func() {
		hub.LogEvent("info", fmt.Sprintf("Server bereit: %s:%d", cfg.ServerHost, cfg.Port))
	}()
	log.Fatal(http.ListenAndServe(bindAddr, mux))
}
