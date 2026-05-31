//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	goruntime "runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
)

// Windows-Konstanten (Keyboard-Hook)
const (
	WH_KEYBOARD_LL = 13
	WM_KEYDOWN     = 0x0100
	WM_SYSKEYDOWN  = 0x0104

	VK_RETURN   = 0x0D
	VK_NUMPAD0  = 0x60
	VK_NUMPAD9  = 0x69
	VK_MULTIPLY = 0x6A // Numpad *
	VK_ADD      = 0x6B // Numpad +
	VK_SUBTRACT = 0x6D // Numpad -
	VK_DECIMAL  = 0x6E // Numpad .
	VK_DIVIDE   = 0x6F // Numpad /
	VK_NUMLOCK  = 0x90

	LLKHF_EXTENDED = 0x01
)

var (
	// modUser32 ist in monitor.go deklariert
	procSetWindowsHookEx    = modUser32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = modUser32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = modUser32.NewProc("CallNextHookEx")
	procGetMessage          = modUser32.NewProc("GetMessageW")
	procTranslateMessage    = modUser32.NewProc("TranslateMessage")
	procDispatchMessage     = modUser32.NewProc("DispatchMessageW")
)

// KBDLLHOOKSTRUCT entspricht tagKBDLLHOOKSTRUCT (winuser.h)
type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

const debounceWindow uint32 = 50 // ms — identische Taste innerhalb dieses Fensters ignorieren

var (
	numpadHookHandle uintptr
	numpadChan       = make(chan map[string]any, 16)

	// Debounce-State (nur vom Hook-Thread geschrieben/gelesen — kein Mutex nötig)
	debounceVk   uint32
	debounceTime uint32
)

// numpadNavDigits: VK-Codes die Numpad-Ziffern erzeugen wenn NumLock AUS ist.
// Reguläre Navigationstasten (dediziertes Tastenfeld) haben LLKHF_EXTENDED=1 und werden
// dadurch korrekt unterschieden.
var numpadNavDigits = map[uint32]string{
	0x0C: "5", // VK_CLEAR  — nur Numpad 5, kein Konflikt
	0x21: "9", // VK_PRIOR  (PgUp)
	0x22: "3", // VK_NEXT   (PgDn)
	0x23: "1", // VK_END
	0x24: "7", // VK_HOME
	0x25: "4", // VK_LEFT
	0x26: "8", // VK_UP
	0x27: "6", // VK_RIGHT
	0x28: "2", // VK_DOWN
	0x2D: "0", // VK_INSERT
}

func numpadHookProc(nCode, wParam, lParam uintptr) uintptr {
	if nCode == 0 && (wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN) {
		kb := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		vk := kb.VkCode
		extended := kb.Flags&LLKHF_EXTENDED != 0

		// Debounce: identische Taste innerhalb von 50ms ignorieren.
		// kb.Time ist ein uint32-Tickcount (ms) — uint32-Subtraktion ist wrap-safe.
		if vk == debounceVk && kb.Time-debounceTime < debounceWindow {
			return 1
		}
		debounceVk = vk
		debounceTime = kb.Time

		// NumLock AN: Numpad-Ziffern haben eindeutige VK_NUMPAD0–9 Codes
		if vk >= VK_NUMPAD0 && vk <= VK_NUMPAD9 {
			select {
			case numpadChan <- map[string]any{
				"action": "input",
				"key":    string(rune('0' + int(vk-VK_NUMPAD0))),
				"target": "chor",
			}:
			default:
			}
			return 1
		}

		// NumLock AUS: Numpad-Ziffern erzeugen Navigations-VKs OHNE extended-Flag.
		// Reguläre Navigationstasten (Home, End, Pfeile …) haben extended=1 → kein Konflikt.
		if !extended {
			if digit, ok := numpadNavDigits[vk]; ok {
				select {
				case numpadChan <- map[string]any{"action": "input", "key": digit, "target": "chor"}:
				default:
				}
				return 1
			}
			// Numpad-Dezimalpunkt (NumLock aus) → VK_DELETE ohne extended → reset
			if vk == 0x2E {
				select {
				case numpadChan <- map[string]any{"action": "reset", "target": "chor"}:
				default:
				}
				return 1
			}
		}

		// Numpad-Operatoren (*, +, -, ., /), Numpad-Enter, NumLock, Sondertasten → reset
		isNumpadOp := vk >= VK_MULTIPLY && vk <= VK_DIVIDE
		isNumpadEnter := vk == VK_RETURN && extended
		isNumLock := vk == VK_NUMLOCK
		isExtra := vk >= 0xA6 && vk <= 0xB7 // Multimedia- und App-Tasten (inkl. Calc 0xB7)

		if isNumpadOp || isNumpadEnter || isNumLock || isExtra {
			select {
			case numpadChan <- map[string]any{"action": "reset", "target": "chor"}:
			default:
			}
			return 1
		}
	}

	ret, _, _ := procCallNextHookEx.Call(numpadHookHandle, nCode, wParam, lParam)
	return ret
}

// isChorScreen prüft ob der aktuelle Screen der Chor-Bildschirm ist
func (a *App) isChorScreen() bool {
	if a.screenIdx >= len(a.cfg.Screens) {
		return false
	}
	return strings.Contains(a.cfg.Screens[a.screenIdx].URL, "chor")
}

// startNumpadHook registriert einen globalen Low-Level-Keyboard-Hook und leitet
// Numpad-Eingaben direkt per WebSocket an den Server weiter — unabhängig vom Fensterfokus.
func (a *App) startNumpadHook() {
	wsURL := fmt.Sprintf("ws://%s:%d/ws/steuerung", a.cfg.ServerHost, a.cfg.Port)

	// WS-Verbindung halten und Nachrichten aus numpadChan senden
	go func() {
		for {
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				log.Printf("numpad ws: %v — retry in 500ms", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			log.Println("numpad ws: verbunden")

			// Lese-Goroutine: gorilla/websocket beantwortet Pings automatisch beim Lesen
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
			}()

			// Sende-Schleife
		send:
			for {
				select {
				case msg := <-numpadChan:
					data, _ := json.Marshal(msg)
					if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
						log.Printf("numpad ws write: %v", err)
						select {
						case numpadChan <- msg:
						default:
						}
						break send
					}
				case <-done:
					break send
				}
			}

			conn.Close()
			<-done // Warten bis Lese-Goroutine beendet ist

			// Veraltete Eingaben verwerfen — nach Reconnect nur frische Tastendrücke senden
			for len(numpadChan) > 0 {
				<-numpadChan
			}

			time.Sleep(500 * time.Millisecond)
		}
	}()

	// Hook registrieren + Windows Message Loop auf festem OS-Thread
	go func() {
		goruntime.LockOSThread()
		// Kein UnlockOSThread — Thread läuft bis Programmende

		cb := syscall.NewCallback(numpadHookProc)
		handle, _, err := procSetWindowsHookEx.Call(WH_KEYBOARD_LL, cb, 0, 0)
		if handle == 0 {
			log.Printf("numpad: SetWindowsHookEx fehlgeschlagen: %v", err)
			return
		}
		numpadHookHandle = handle
		log.Println("numpad: globaler Keyboard-Hook aktiv")

		// 64 Byte reichen für MSG auf allen Windows-Architekturen
		var msgBuf [64]byte
		for {
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])), 0, 0, 0)
			if ret == 0 || ret == ^uintptr(0) { // WM_QUIT oder Fehler
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&msgBuf[0])))
		}

		procUnhookWindowsHookEx.Call(numpadHookHandle)
		log.Println("numpad: Hook entfernt")
	}()
}
