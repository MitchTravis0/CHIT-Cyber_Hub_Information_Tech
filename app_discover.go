package main

import "chit/internal/tools/discover"

// StartDeviceDiscovery listens for mDNS and SSDP announcements and streams one
// result per device heard. Stop it with CancelJob.
func (a *App) StartDeviceDiscovery(p discover.Params) (string, error) {
	return discover.New(a.jobs).StartDiscovery(p)
}
