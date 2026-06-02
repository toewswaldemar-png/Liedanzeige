package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
)

type Screen struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Monitor int    `json:"monitor"`
}

type KioskConfig struct {
	AlwaysOnTop bool `json:"always_on_top"`
}

type AppConfig struct {
	ServerHost string      `json:"server_host"`
	Port       int         `json:"port"`
	Dev        bool        `json:"dev"`
	Screens    []Screen    `json:"screens"`
	Kiosk      KioskConfig `json:"kiosk"`
}

type DisplaySettings struct {
	TimeSize       int    `json:"timeSize"`
	GapTimeDate    int    `json:"gapTimeDate"`
	Font           string `json:"font"`
	ShadowStrength int    `json:"shadowStrength"`
	ResetDelay     int    `json:"resetDelay"`
}

var defaultConfig = AppConfig{
	ServerHost: "localhost",
	Port:       1980,
	Dev:        true,
	Screens: []Screen{
		{Name: "liedanzeige", URL: "/lied", Monitor: 1},
		{Name: "choranzeige", URL: "/chor", Monitor: 0},
	},
	Kiosk: KioskConfig{AlwaysOnTop: true},
}

var defaultSettings = DisplaySettings{
	TimeSize:       75,
	GapTimeDate:    0,
	Font:           "segoe-ui",
	ShadowStrength: 40,
	ResetDelay:     5,
}

func detectLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	var fallback string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			continue
		}
		// 169.254.x.x link-local überspringen
		if ip4[0] == 169 && ip4[1] == 254 {
			continue
		}
		// Private Ranges bevorzugen
		if ip4[0] == 10 || ip4[0] == 192 || (ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) {
			return ip4.String()
		}
		if fallback == "" {
			fallback = ip4.String()
		}
	}
	if fallback != "" {
		return fallback
	}
	return "localhost"
}

func loadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := defaultConfig
		cfg.ServerHost = detectLANIP()
		return &cfg, writeJSON(path, cfg)
	}
	if err != nil {
		return nil, err
	}
	cfg := defaultConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// server_host validieren: kein "localhost" und keine gültige IP → auto-detect und Config korrigieren
	if cfg.ServerHost != "localhost" && net.ParseIP(cfg.ServerHost) == nil {
		log.Printf("config: ungültige server_host %q — erkenne LAN-IP automatisch", cfg.ServerHost)
		cfg.ServerHost = detectLANIP()
		_ = writeJSON(path, cfg)
	}
	return &cfg, nil
}

func loadSettings(path string) (*DisplaySettings, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		s := defaultSettings
		return &s, writeJSON(path, s)
	}
	if err != nil {
		return nil, err
	}
	s := defaultSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveSettings(path string, s DisplaySettings) error {
	return writeJSON(path, s)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
