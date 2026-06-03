//go:build !windows

package main

func ensureFirewallRule(_ int)    {}
func ensureDiscoveryFirewallRule() {}
