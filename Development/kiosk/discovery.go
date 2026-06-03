package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

const discoveryPort = 1981
const discoveryMagic = "LIEDANZEIGE_DISCOVER"

// quickHealthCheck prüft ob der Server unter host:port erreichbar ist.
func quickHealthCheck(host string, port int, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(fmt.Sprintf("http://%s:%d/health", host, port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// discoverServer sendet einen UDP-Broadcast und wartet auf Antwort vom Server.
func discoverServer(timeout time.Duration) (host string, port int, ok bool) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		log.Printf("discovery: ListenPacket: %v", err)
		return
	}
	defer conn.Close()

	broadcast, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", discoveryPort))
	if err != nil {
		return
	}

	conn.SetDeadline(time.Now().Add(timeout))

	if _, err = conn.WriteTo([]byte(discoveryMagic), broadcast); err != nil {
		log.Printf("discovery: WriteTo: %v", err)
		return
	}

	buf := make([]byte, 256)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return
	}

	var resp struct {
		Service string `json:"service"`
		Host    string `json:"host"`
		Port    int    `json:"port"`
	}
	if json.Unmarshal(buf[:n], &resp) == nil && resp.Service == "liedanzeige" && resp.Host != "" {
		host, port, ok = resp.Host, resp.Port, true
	}
	return
}
