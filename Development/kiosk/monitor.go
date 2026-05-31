//go:build windows

package main

import (
	"log"
	"os"
	"syscall"
	"unsafe"
)

const (
	SWP_FRAMECHANGED   = 0x0020
	SWP_SHOWWINDOW     = 0x0040
	SWP_NOSENDCHANGING = 0x0400
	SW_RESTORE         = 9
	HWND_TOPMOST       = ^uintptr(0) // -1
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
)

type monitorRect struct{ X, Y, W, H int }

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
// topmost=true: HWND_TOPMOST (Vollbild über Taskleiste), topmost=false: normales Z-Order.
func setWindowPos(r monitorRect, topmost bool) {
	hwnd := getOwnHWND()
	if hwnd == 0 {
		log.Println("setWindowPos: HWND nicht gefunden")
		return
	}
	procShowWindow.Call(hwnd, SW_RESTORE)
	procSetForegroundWindow.Call(hwnd)
	insertAfter := uintptr(0)
	flags := uintptr(SWP_FRAMECHANGED | SWP_SHOWWINDOW)
	if topmost {
		insertAfter = HWND_TOPMOST
		flags |= SWP_NOSENDCHANGING
	}
	procSetWindowPos.Call(hwnd, insertAfter,
		uintptr(r.X), uintptr(r.Y), uintptr(r.W), uintptr(r.H), flags)
}
