package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	screenIdx := flag.Int("screen", -1, "Screen-Index in config.screens; ohne Flag → Supervisor-Modus")
	flag.Parse()

	if *screenIdx < 0 {
		runSupervisor()
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app := NewApp(cfg, *screenIdx)

	width, height := 1920, 1080
	if cfg.Dev {
		width, height = 800, 600
	}

	title := "Liedanzeige"
	if *screenIdx < len(cfg.Screens) {
		title = cfg.Screens[*screenIdx].Name
	}

	// Eindeutiges WebView2-Datenverzeichnis pro Screen-Prozess.
	// Aufräumen falls Vorprozess unsauber beendet wurde (verhindert lautloses WebView2-Versagen).
	webviewDataPath := filepath.Join(os.TempDir(), fmt.Sprintf("liedanzeige-screen-%d", *screenIdx))
	_ = os.RemoveAll(webviewDataPath)

	err = wails.Run(&options.App{
		Title:            title,
		Width:            width,
		Height:           height,
		StartHidden:      !cfg.Dev, // Fenster erst nach Positionierung anzeigen → kein Größensprung
		AlwaysOnTop:      false,
		Frameless:        false, // Rahmen immer aktiv — wird per Win32 für Vollbild entfernt
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:                app.startup,
		OnDomReady:               app.domReady,
		OnBeforeClose:            app.beforeClose,
		Bind:                     []interface{}{app},
		EnableDefaultContextMenu: false,
		Windows: &windows.Options{
			WebviewUserDataPath: webviewDataPath,
		},
	})

	if err != nil {
		log.Fatal("Error:", err)
	}
}
