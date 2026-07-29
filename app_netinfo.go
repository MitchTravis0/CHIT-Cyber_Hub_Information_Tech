package main

import "chit/internal/netinfo"

// ListAdapters reports every network adapter on this machine with the details
// a tech reads off ipconfig / ifconfig: IP, mask, gateway, DNS, DHCP and MAC.
func (a *App) ListAdapters() (netinfo.Report, error) {
	return netinfo.List()
}
