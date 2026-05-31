package main

import (
	"embed"
	"flag"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	screenIdx := flag.Int("screen", 0, "Index in config.screens (wird von Screen 0 automatisch gesetzt)")
	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app := NewApp(cfg, *screenIdx)

	width, height := 1920, 1080
	if cfg.Dev {
		width, height = 800, 600
	}

	title := "Uhr Kiosk"
	if *screenIdx < len(cfg.Screens) {
		title = cfg.Screens[*screenIdx].Name
	}

	err = wails.Run(&options.App{
		Title:            title,
		Width:            width,
		Height:           height,
		AlwaysOnTop:      false,
		Frameless:        !cfg.Dev,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:                app.startup,
		OnDomReady:               app.domReady,
		Bind:                     []interface{}{app},
		EnableDefaultContextMenu: false,
	})

	if err != nil {
		log.Fatal("Error:", err)
	}
}
