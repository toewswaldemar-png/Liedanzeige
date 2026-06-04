//go:build windows

package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

func main() {
	exePath, _ := os.Executable()
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

	quit := make(chan struct{})
	var once sync.Once
	closeAll := func() {
		once.Do(func() { close(quit) })
	}

	startQuitShortcut(closeAll)

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime.LockOSThread()
			runScreen(cfg, i, closeAll, quit)
		}()
	}

	wg.Wait()
}
