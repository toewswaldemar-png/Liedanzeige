package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

type Screen struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Monitor int    `json:"monitor"`
}

type KioskConfig struct {
	AlwaysOnTop bool `json:"always_on_top"`
}

type Config struct {
	ServerHost string      `json:"server_host"`
	Port       int         `json:"port"`
	Dev        bool        `json:"dev"`
	Screens    []Screen    `json:"screens"`
	Kiosk      KioskConfig `json:"kiosk"`
}

var defaultKioskConfig = Config{
	ServerHost: "localhost",
	Port:       1980,
	Dev:        false,
	Screens: []Screen{
		{Name: "liedanzeige", URL: "/lied", Monitor: 1},
		{Name: "choranzeige", URL: "/chor", Monitor: 0},
	},
	Kiosk: KioskConfig{AlwaysOnTop: true},
}

func loadConfig() (*Config, error) {
	exePath, err := os.Executable()
	if err != nil {
		return createDefaultConfig("config.json")
	}
	exeDir := filepath.Dir(exePath)

	// Suche config.json neben der exe, dann eine Ebene höher, dann CWD
	candidates := []string{
		filepath.Join(exeDir, "config.json"),
		filepath.Join(exeDir, "..", "config.json"),
		"config.json",
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		cfg := defaultKioskConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	// Keine config.json gefunden → anlegen
	return createDefaultConfig(filepath.Join(exeDir, "config.json"))
}

func saveConfig(cfg *Config) {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	path := filepath.Join(filepath.Dir(exePath), "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("config: speichern fehlgeschlagen: %v", err)
		return
	}
	log.Printf("config: server_host aktualisiert → %s", cfg.ServerHost)
}

func createDefaultConfig(path string) (*Config, error) {
	cfg := defaultKioskConfig
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return &cfg, nil
	}
	_ = os.WriteFile(path, data, 0644)
	return &cfg, nil
}
