//go:build windows

package main

import (
	goruntime "runtime"
	"syscall"
	"unsafe"
)

const (
	VK_CONTROL = 0x11
	VK_MENU    = 0x12 // Alt
	VK_Q       = 0x51
)

var (
	procGetAsyncKeyState = modUser32.NewProc("GetAsyncKeyState")
	quitHookHandle       uintptr
)

// isKeyDown prüft den echten Hardware-Zustand einer Taste (auch aus Low-Level-Hooks heraus korrekt).
func isKeyDown(vk uintptr) bool {
	state, _, _ := procGetAsyncKeyState.Call(vk)
	return state&0x8000 != 0
}

func quitShortcutProc(nCode, wParam, lParam uintptr) uintptr {
	if nCode == 0 && (wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN) {
		kb := (*KBDLLHOOKSTRUCT)(*(*unsafe.Pointer)(unsafe.Pointer(&lParam)))
		if kb.VkCode == VK_Q && isKeyDown(VK_CONTROL) && isKeyDown(VK_MENU) {
			select {
			case directQuitChan <- struct{}{}:
			default:
			}
			return 1 // Tastendrück schlucken
		}
	}
	ret, _, _ := procCallNextHookEx.Call(quitHookHandle, nCode, wParam, lParam)
	return ret
}

// directQuitChan wird vom Hook befüllt und direkt vom App abgehört.
var directQuitChan = make(chan struct{}, 1)

// startQuitShortcut registriert einen globalen Keyboard-Hook für Ctrl+Alt+Q.
// Bei Auslösung wird der Screen-Prozess direkt beendet (kein WebSocket-Roundtrip).
func startQuitShortcut(onQuit func()) {
	go func() {
		<-directQuitChan
		onQuit()
	}()

	go func() {
		goruntime.LockOSThread()
		cb := syscall.NewCallback(quitShortcutProc)
		handle, _, _ := procSetWindowsHookEx.Call(WH_KEYBOARD_LL, cb, 0, 0)
		if handle == 0 {
			return
		}
		quitHookHandle = handle

		var msgBuf [64]byte
		for {
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])), 0, 0, 0)
			if ret == 0 || ret == ^uintptr(0) {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])))
		}
		procUnhookWindowsHookEx.Call(quitHookHandle)
	}()
}
