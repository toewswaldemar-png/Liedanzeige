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

type supervisor struct {
	cfg     *Config
	exePath string
	screens []*managedScreen
}

type managedScreen struct {
	idx         int
	sup         *supervisor
	mu          sync.Mutex
	proc        *exec.Cmd
	crashCount  int
	lastCrashAt time.Time
	stopped     bool // verhindert Neustart nach bewusstem Beenden
}

func runSupervisor() {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exeDir := filepath.Dir(exePath)

	if f, err := os.OpenFile(filepath.Join(exeDir, "kiosk.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	count := len(cfg.Screens)
	if count == 0 {
		count = 1
	}

	sup := &supervisor{
		cfg:     cfg,
		exePath: exePath,
		screens: make([]*managedScreen, count),
	}
	for i := range sup.screens {
		sup.screens[i] = &managedScreen{idx: i, sup: sup}
	}

	log.Printf("Supervisor — %d Screen(s), Server: %s:%d", count, cfg.ServerHost, cfg.Port)

	for _, s := range sup.screens {
		go s.run(exePath)
	}

	startQuitShortcut(func() {
		log.Println("Beende Supervisor und alle Screens (Tastenkürzel)...")
		for _, s := range sup.screens {
			s.mu.Lock()
			s.stopped = true
			if s.proc != nil && s.proc.Process != nil {
				_ = s.proc.Process.Kill()
			}
			s.mu.Unlock()
		}
		os.Exit(0)
	})

	sup.connectWS()
}

// run startet den Screen-Prozess und überwacht ihn in einer Endlosschleife.
func (s *managedScreen) run(exePath string) {
	for {
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		cmd := exec.Command(exePath, fmt.Sprintf("--screen=%d", s.idx))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			log.Printf("[screen %d] start: %v — retry in 3s", s.idx, err)
			time.Sleep(3 * time.Second)
			continue
		}

		s.mu.Lock()
		s.proc = cmd
		s.mu.Unlock()
		log.Printf("[screen %d] gestartet (PID %d)", s.idx, cmd.Process.Pid)

		startTime := time.Now()
		err := cmd.Wait()
		runDuration := time.Since(startTime)

		// Exit-Code 100 = bewusstes Beenden durch den Nutzer (X-Button)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 100 {
			log.Printf("[screen %d] vom Nutzer beendet — beende alle Screens", s.idx)
			for _, other := range s.sup.screens {
				other.mu.Lock()
				other.stopped = true
				if other.proc != nil && other.proc.Process != nil {
					_ = other.proc.Process.Kill()
				}
				other.mu.Unlock()
			}
			os.Exit(0)
		}

		s.mu.Lock()
		s.proc = nil
		if runDuration < 30*time.Second {
			if time.Since(s.lastCrashAt) < 60*time.Second {
				s.crashCount++
			} else {
				s.crashCount = 1
			}
			s.lastCrashAt = time.Now()
		} else {
			s.crashCount = 0
		}
		crashCount := s.crashCount
		s.mu.Unlock()

		if crashCount >= 5 {
			log.Printf("[screen %d] WARNUNG: %d Mal in kurzer Zeit abgestürzt", s.idx, crashCount)
		}
		log.Printf("[screen %d] beendet (%v, Laufzeit %v) — Neustart in 3s", s.idx, err, runDuration.Round(time.Second))
		time.Sleep(3 * time.Second)
	}
}

// restart beendet den laufenden Prozess; run() startet ihn automatisch neu.
func (s *managedScreen) restart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc != nil && s.proc.Process != nil {
		_ = s.proc.Process.Kill()
	}
}

func (sup *supervisor) restartAll() {
	log.Println("reload — starte alle Screens neu")
	for _, s := range sup.screens {
		s.restart()
	}
}

func (sup *supervisor) connectWS() {
	url := fmt.Sprintf("ws://%s:%d/ws/kiosk", sup.cfg.ServerHost, sup.cfg.Port)
	for {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Printf("supervisor ws: %v — retry in 2s", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Println("supervisor ws: verbunden")

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("supervisor ws: %v", err)
				break
			}
			var msg map[string]any
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch {
			case msg["action"] == "kiosk" && msg["command"] == "reload":
				go sup.restartAll()
			case msg["action"] == "kiosk" && msg["command"] == "quit":
				log.Println("Beende Supervisor und alle Screens...")
				for _, s := range sup.screens {
					s.mu.Lock()
					s.stopped = true
					if s.proc != nil && s.proc.Process != nil {
						_ = s.proc.Process.Kill()
					}
					s.mu.Unlock()
				}
				os.Exit(0)
			}
		}
		conn.Close()
		time.Sleep(2 * time.Second)
	}
}
