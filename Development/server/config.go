package main

import (
	"encoding/json"
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
	TimeSize:       27,
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
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ip4 := ipNet.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
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
