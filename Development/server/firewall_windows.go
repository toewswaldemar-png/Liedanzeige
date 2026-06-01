//go:build windows

package main

import (
	"fmt"
	"log"
	"os/exec"
	"syscall"
	"unsafe"
)

func ensureFirewallRule(port int) {
	ruleName := "Liedanzeige-Server"

	// Pruefen ob Regel bereits existiert
	check := exec.Command("netsh", "advfirewall", "firewall", "show", "rule",
		fmt.Sprintf("name=%s", ruleName))
	check.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if check.Run() == nil {
		log.Println("Firewall: Regel bereits vorhanden")
		return
	}

	// Regel fehlt → mit Elevation hinzufuegen (UAC-Dialog)
	log.Printf("Firewall: Neue Regel fuer Port %d wird eingerichtet (UAC-Dialog)...", port)

	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("ShellExecuteW")

	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString("netsh")
	params, _ := syscall.UTF16PtrFromString(fmt.Sprintf(
		`advfirewall firewall add rule name="%s" dir=in action=allow protocol=TCP localport=%d`,
		ruleName, port,
	))

	ret, _, _ := proc.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		1, // SW_SHOWNORMAL
	)
	if ret > 32 {
		log.Println("Firewall: Regel wird eingerichtet")
	} else {
		log.Printf("Firewall: Konnte Regel nicht einrichten (ShellExecute Code %d)", ret)
	}
}
