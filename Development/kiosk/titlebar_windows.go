//go:build windows

package main

import (
	goruntime "runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	WM_NCLBUTTONDOWN  = 0x00A1
	WM_LBUTTONDOWN    = 0x0201
	WM_CLOSE          = 0x0010
	HTCAPTION_VAL     = 2
	SW_MINIMIZE_VAL   = 6
	SW_MAXIMIZE_VAL   = 3
	WH_MOUSE_LL_VAL   = 14
	titleBarHeightCSS = 28 // CSS-Pixel — muss mit injiziertem JS übereinstimmen
	btnWidthCSS       = 46 // CSS-Pixel pro Button (×, □, –)
)

var (
	procGetWindowRect   = modUser32.NewProc("GetWindowRect")
	procPostMessage     = modUser32.NewProc("PostMessageW")
	procGetDpiForWindow = modUser32.NewProc("GetDpiForWindow")

	mouseTitleHookHandle uintptr
	titleBarMode         atomic.Bool
)

type winRECT struct{ Left, Top, Right, Bottom int32 }

type msllHookStruct struct {
	PtX, PtY    int32
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

func titleBarMouseProc(nCode, wParam, lParam uintptr) uintptr {
	if nCode == 0 && wParam == WM_LBUTTONDOWN && titleBarMode.Load() {
		ms := (*msllHookStruct)(unsafe.Pointer(lParam))
		mx, my := int(ms.PtX), int(ms.PtY)

		hwnd := getOwnHWND()
		if hwnd != 0 {
			var wr winRECT
			procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))

			dpi, _, _ := procGetDpiForWindow.Call(hwnd)
			if dpi == 0 {
				dpi = 96
			}
			scale := int(dpi)
			tbH := titleBarHeightCSS * scale / 96
			btnW := btnWidthCSS * scale / 96

			wx := int(wr.Left)
			wy := int(wr.Top)
			ww := int(wr.Right - wr.Left)

			if mx >= wx && mx < wx+ww && my >= wy && my < wy+tbH {
				fromRight := wx + ww - mx
				switch {
				case fromRight <= btnW:
					// Schliessen (×)
					procPostMessage.Call(hwnd, WM_CLOSE, 0, 0)
				case fromRight <= btnW*2:
					// Maximieren (□)
					procShowWindow.Call(hwnd, SW_MAXIMIZE_VAL)
				case fromRight <= btnW*3:
					// Minimieren (–)
					procShowWindow.Call(hwnd, SW_MINIMIZE_VAL)
				default:
					// Verschieben (Drag)
					procPostMessage.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION_VAL, 0)
				}
				return 1 // Klick schlucken
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(mouseTitleHookHandle, nCode, wParam, lParam)
	return ret
}

func startTitleBarHook() {
	titleBarMode.Store(true)
	if mouseTitleHookHandle != 0 {
		return // Hook läuft bereits
	}
	go func() {
		goruntime.LockOSThread()
		cb := syscall.NewCallback(titleBarMouseProc)
		handle, _, _ := procSetWindowsHookEx.Call(WH_MOUSE_LL_VAL, cb, 0, 0)
		if handle == 0 {
			return
		}
		mouseTitleHookHandle = handle

		var msgBuf [64]byte
		for {
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])), 0, 0, 0)
			if ret == 0 || ret == ^uintptr(0) {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])))
		}
		procUnhookWindowsHookEx.Call(mouseTitleHookHandle)
		mouseTitleHookHandle = 0
	}()
}

func stopTitleBarHook() {
	titleBarMode.Store(false)
}
