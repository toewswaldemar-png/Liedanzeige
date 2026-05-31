//go:build windows

package main

import (
	"log"
	"os"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SetWindowPos-Flags / ShowWindow-Kommandos / Fensterstile
const (
	SWP_FRAMECHANGED    = 0x0020
	SWP_SHOWWINDOW      = 0x0040
	SWP_NOSENDCHANGING  = 0x0400
	SW_RESTORE          = 9
	HWND_TOPMOST     = ^uintptr(0) // -1: bleibt über allen anderen Fenstern inkl. Taskleiste
	GWL_STYLE        = ^uintptr(15)  // -16
	WS_POPUP         = uintptr(0x80000000) // kein Rahmen, keine Titelleiste
	WS_VISIBLE       = uintptr(0x10000000)
)

var (
	modUser32                    = syscall.NewLazyDLL("user32.dll")
	procEnumDisplayMonitors      = modUser32.NewProc("EnumDisplayMonitors")
	procEnumWindows              = modUser32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = modUser32.NewProc("IsWindowVisible")
	procSetWindowPos             = modUser32.NewProc("SetWindowPos")
	procShowWindow               = modUser32.NewProc("ShowWindow")
	procSetForegroundWindow      = modUser32.NewProc("SetForegroundWindow")
	procGetWindowLongPtr         = modUser32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr         = modUser32.NewProc("SetWindowLongPtrW")
)

// monitorRect enthält Position und Größe eines Monitors in physischen Pixeln.
type monitorRect struct{ X, Y, W, H int }

// getMonitorRects gibt alle angeschlossenen Monitore in der Reihenfolge zurück,
// wie Windows sie aufzählt.
func getMonitorRects() []monitorRect {
	var rects []monitorRect
	cb := syscall.NewCallback(func(_, _, lprcMonitor, _ uintptr) uintptr {
		type RECT struct{ Left, Top, Right, Bottom int32 }
		r := (*RECT)(unsafe.Pointer(lprcMonitor))
		rects = append(rects, monitorRect{
			X: int(r.Left), Y: int(r.Top),
			W: int(r.Right - r.Left), H: int(r.Bottom - r.Top),
		})
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
	return rects
}

// getOwnHWND gibt das erste sichtbare Top-Level-Fenster dieses Prozesses zurück.
func getOwnHWND() uintptr {
	pid := uint32(os.Getpid())
	var hwnd uintptr
	cb := syscall.NewCallback(func(h, _ uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(h)
		if visible == 0 {
			return 1
		}
		var wPid uint32
		procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&wPid)))
		if wPid == pid {
			hwnd = h
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return hwnd
}

// restoreWindow hebt eine Minimierung auf und bringt das Fenster in den Vordergrund.
// Muss vor SetWindowPos aufgerufen werden, da SWP_SHOWWINDOW allein das WS_MINIMIZE-Bit
// nicht löscht und ein minimiertes Fenster daher unsichtbar bleibt.
func restoreWindow(hwnd uintptr) {
	procShowWindow.Call(hwnd, SW_RESTORE)
	procSetForegroundWindow.Call(hwnd)
}

// moveWindowFullscreenToMonitor teleportiert das eigene Fenster auf den angegebenen Monitor
// und bringt es in den Vordergrund (HWND_TOP).
func moveWindowFullscreenToMonitor(r monitorRect) {
	hwnd := getOwnHWND()
	if hwnd == 0 {
		log.Println("moveWindowFullscreenToMonitor: HWND nicht gefunden")
		return
	}
	restoreWindow(hwnd)
	// WS_POPUP entfernt Rahmen/Titelleiste — Windows beschränkt das Fenster sonst
	// auf den Work Area (Monitor ohne Taskleiste) statt auf die volle Monitorfläche.
	procSetWindowLongPtr.Call(hwnd, GWL_STYLE, WS_POPUP|WS_VISIBLE)
	procSetWindowPos.Call(
		hwnd, HWND_TOPMOST,
		uintptr(r.X), uintptr(r.Y),
		uintptr(r.W), uintptr(r.H),
		SWP_FRAMECHANGED|SWP_SHOWWINDOW|SWP_NOSENDCHANGING,
	)
}

// positionWindowWindowed positioniert das eigene Fenster im Fenstermodus auf den angegebenen Bereich.
// Verwendet Win32 direkt für konsistente physikalische Pixelkoordinaten (identisch mit getMonitorRects).
func positionWindowWindowed(x, y, w, h int) {
	hwnd := getOwnHWND()
	if hwnd == 0 {
		log.Println("positionWindowWindowed: HWND nicht gefunden")
		return
	}
	restoreWindow(hwnd)
	procSetWindowPos.Call(
		hwnd, 0,
		uintptr(x), uintptr(y),
		uintptr(w), uintptr(h),
		SWP_FRAMECHANGED|SWP_SHOWWINDOW,
	)
}

// positionOnConfiguredMonitor verschiebt das Fenster auf den in config.json konfigurierten Monitor.
func (a *App) positionOnConfiguredMonitor() {
	rects := getMonitorRects()
	if a.currentMonitorIdx >= len(rects) {
		log.Printf("Monitor %d nicht verfügbar (%d Monitore erkannt) — bleibe auf aktuellem Monitor", a.currentMonitorIdx, len(rects))
		return
	}
	r := rects[a.currentMonitorIdx]
	runtime.WindowSetPosition(a.ctx, r.X, r.Y)
	log.Printf("[screen %d] positioniert auf Monitor %d (%d,%d)", a.screenIdx, a.currentMonitorIdx, r.X, r.Y)
}
