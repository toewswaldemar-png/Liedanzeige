package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ── Config ────────────────────────────────────────────────────────────────────

type Config struct {
	ServerHost string `json:"server_host"`
	Port       int    `json:"port"`
}

func loadConfig(dir string) (*Config, error) {
	for _, p := range []string{
		filepath.Join(dir, "config.json"),
		filepath.Join(dir, "..", "config.json"),
	} {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		cfg := Config{ServerHost: "localhost", Port: 1980}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	cfg := Config{ServerHost: "localhost", Port: 1980}
	log.Printf("Keine config.json gefunden — verwende Standard (%s:%d)", cfg.ServerHost, cfg.Port)
	return &cfg, nil
}

// ── Watchdog ──────────────────────────────────────────────────────────────────

type Watchdog struct {
	cfg         *Config
	mu          sync.Mutex // schuetzt proc, crashCount, lastCrashAt
	startMu     sync.Mutex // verhindert gleichzeitige Neustarts
	proc        *exec.Cmd
	exeDir      string
	crashCount  int
	lastCrashAt time.Time
}

func (w *Watchdog) kioskPath() string {
	return filepath.Join(w.exeDir, "liedanzeige-kiosk.exe")
}

func (w *Watchdog) killAll() {
	name := filepath.Base(w.kioskPath())
	if err := exec.Command("taskkill", "/F", "/IM", name).Run(); err != nil {
		log.Printf("taskkill: %v", err)
	}
}

func (w *Watchdog) start() {
	// Nur ein Start gleichzeitig erlaubt
	w.startMu.Lock()
	defer w.startMu.Unlock()

	w.killAll()
	time.Sleep(500 * time.Millisecond)

	cmd := exec.Command(w.kioskPath())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("kiosk start: %v", err)
		return
	}

	w.mu.Lock()
	w.proc = cmd
	w.mu.Unlock()

	log.Printf("kiosk gestartet (PID %d)", cmd.Process.Pid)
	go w.monitor(cmd)
}

// monitor wartet auf Prozessende und startet automatisch neu
func (w *Watchdog) monitor(cmd *exec.Cmd) {
	startTime := time.Now()
	err := cmd.Wait()
	runDuration := time.Since(startTime)

	w.mu.Lock()
	isCurrent := w.proc == cmd
	if isCurrent {
		w.proc = nil
		// Crash-Zaehler: nur bei kurzen Laufzeiten erhoehen
		if runDuration < 30*time.Second {
			if time.Since(w.lastCrashAt) < 60*time.Second {
				w.crashCount++
			} else {
				w.crashCount = 1
			}
			w.lastCrashAt = time.Now()
		} else {
			w.crashCount = 0
		}
	}
	crashCount := w.crashCount
	w.mu.Unlock()

	if isCurrent {
		if crashCount >= 5 {
			log.Printf("WARNUNG: Kiosk %d Mal in kurzer Zeit abgestuerzt — moeglicher Fehler in kiosk.exe", crashCount)
		}
		log.Printf("kiosk beendet (%v, Laufzeit: %v) — Neustart in 3s", err, runDuration.Round(time.Second))
		time.Sleep(3 * time.Second)
		w.start()
	}
}

// connectWS haelt WebSocket-Verbindung und verarbeitet Befehle
func (w *Watchdog) connectWS() {
	url := fmt.Sprintf("ws://%s:%d/ws/kiosk", w.cfg.ServerHost, w.cfg.Port)
	for {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Printf("ws: %v — retry in 2s", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("ws: verbunden mit %s", url)

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("ws read: %v", err)
				break
			}
			var msg map[string]any
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			if msg["action"] == "kiosk" && msg["command"] == "reload" {
				log.Println("reload — starte kiosk neu")
				go w.start()
			}
		}
		conn.Close()
		time.Sleep(2 * time.Second)
	}
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exeDir := filepath.Dir(exePath)

	if f, err := os.OpenFile(filepath.Join(exeDir, "watchdog.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	}

	cfg, err := loadConfig(exeDir)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	log.Printf("Watchdog — Server: %s:%d", cfg.ServerHost, cfg.Port)

	w := &Watchdog{cfg: cfg, exeDir: exeDir}
	w.start()
	w.connectWS() // blockiert
}
