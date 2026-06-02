//go:build windows

package main

import (
	"log"
	"os"
	"syscall"
	"unsafe"
)

const (
	SWP_FRAMECHANGED   = uintptr(0x0020)
	SWP_SHOWWINDOW     = uintptr(0x0040)
	SWP_NOSENDCHANGING = uintptr(0x0400)
	SWP_NOMOVE         = uintptr(0x0002)
	SWP_NOSIZE         = uintptr(0x0001)
	SWP_NOZORDER       = uintptr(0x0004)
	SW_RESTORE         = uintptr(9)
	HWND_TOPMOST       = ^uintptr(0) // -1
	HWND_NOTOPMOST     = ^uintptr(1) // -2

	// Fensterrahmen-Stile
	WS_POPUP            = uintptr(0x80000000)
	WS_OVERLAPPEDWINDOW = uintptr(0x00CF0000)
	GWL_STYLE           = ^uintptr(15) // -16
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

type monitorRect struct{ X, Y, W, H int }

func getMonitorRects() []monitorRect {
	var rects []monitorRect
	cb := syscall.NewCallback(func(_, _, lprcMonitor, _ uintptr) uintptr {
		type RECT struct{ Left, Top, Right, Bottom int32 }
		r := (*RECT)(*(*unsafe.Pointer)(unsafe.Pointer(&lprcMonitor)))
		rects = append(rects, monitorRect{
			X: int(r.Left), Y: int(r.Top),
			W: int(r.Right - r.Left), H: int(r.Bottom - r.Top),
		})
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
	return rects
}

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

// setWindowPos positioniert das eigene Fenster.
func setWindowPos(r monitorRect, topmost bool) {
	hwnd := getOwnHWND()
	if hwnd == 0 {
		log.Println("setWindowPos: HWND nicht gefunden")
		return
	}
	procShowWindow.Call(hwnd, SW_RESTORE)
	procSetForegroundWindow.Call(hwnd)
	insertAfter := HWND_NOTOPMOST
	flags := SWP_FRAMECHANGED | SWP_SHOWWINDOW
	if topmost {
		insertAfter = HWND_TOPMOST
		flags |= SWP_NOSENDCHANGING
	}
	procSetWindowPos.Call(hwnd, insertAfter,
		uintptr(r.X), uintptr(r.Y), uintptr(r.W), uintptr(r.H), flags)
}

// setWindowFrame schaltet den nativen Windows-Rahmen (Titelleiste) an oder aus.
// withFrame=true  → WS_OVERLAPPEDWINDOW (echte Titelleiste mit Minimize/Maximize/Close)
// withFrame=false → WS_POPUP (rahmenlos für Vollbild)
//
// Funktioniert nur wenn Wails mit Frameless=false gestartet wurde — andernfalls
// überschreibt Wails WM_NCCALCSIZE und der Rahmen bleibt unsichtbar.
func setWindowFrame(withFrame bool) {
	hwnd := getOwnHWND()
	if hwnd == 0 {
		return
	}
	style, _, _ := procGetWindowLongPtr.Call(hwnd, GWL_STYLE)
	if withFrame {
		style &^= WS_POPUP
		style |= WS_OVERLAPPEDWINDOW
	} else {
		style &^= WS_OVERLAPPEDWINDOW
		style |= WS_POPUP
	}
	procSetWindowLongPtr.Call(hwnd, GWL_STYLE, style)
	// SWP_FRAMECHANGED zwingt Windows den neuen Rahmen sofort neu zu zeichnen
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		SWP_FRAMECHANGED|SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER)
}
