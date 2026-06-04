//go:build windows

package main

import "syscall"

const (
	WM_CLOSE     = uintptr(0x0010)
	GWLP_WNDPROC = ^uintptr(3) // -4
)

var (
	procCallWindowProcW = modUser32.NewProc("CallWindowProcW")

	// Closures müssen am Leben gehalten werden — syscall.NewCallback schützt die
	// Trampolin-Funktion, aber nicht den Go-Closure dahinter.
	wndProcClosures []any
)

// subclassClose fängt WM_CLOSE (X-Button) ab und ruft stattdessen closeAll auf,
// sodass beide Fenster gemeinsam beendet werden.
func subclassClose(hwnd uintptr, closeAll func()) {
	var origProc uintptr
	fn := func(h, msg, wParam, lParam uintptr) uintptr {
		if msg == WM_CLOSE {
			closeAll()
			return 0
		}
		r, _, _ := procCallWindowProcW.Call(origProc, h, msg, wParam, lParam)
		return r
	}
	wndProcClosures = append(wndProcClosures, fn) // GC-Schutz
	cb := syscall.NewCallback(fn)
	origProc, _, _ = procSetWindowLongPtr.Call(hwnd, GWLP_WNDPROC, cb)
}
