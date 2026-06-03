package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

const discoveryPort = 1981
const discoveryMagic = "LIEDANZEIGE_DISCOVER"

func startDiscoveryListener(serverHost string, serverPort int) {
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", discoveryPort))
	if err != nil {
		log.Printf("discovery: UDP-Listener konnte nicht gestartet werden: %v", err)
		return
	}
	defer conn.Close()
	log.Printf("discovery: lauscht auf UDP:%d", discoveryPort)

	resp, _ := json.Marshal(map[string]any{"service": "liedanzeige", "host": serverHost, "port": serverPort})
	buf := make([]byte, 64)
	// Deduplizierung: letzte Antwortzeit pro Quell-IP
	lastSeen := map[string]time.Time{}

	for {
		n, src, err := conn.ReadFrom(buf)
		if err != nil {
			log.Printf("discovery: %v", err)
			return
		}
		if string(buf[:n]) != discoveryMagic {
			continue
		}
		// Quell-IP ohne Port für Deduplizierung
		srcHost, _, _ := net.SplitHostPort(src.String())
		if time.Since(lastSeen[srcHost]) < 500*time.Millisecond {
			continue // Duplikat ignorieren
		}
		lastSeen[srcHost] = time.Now()
		log.Printf("discovery: Anfrage von %s", srcHost)
		conn.WriteTo(resp, src)
	}
}
