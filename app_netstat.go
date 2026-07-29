package main

import "chit/internal/tools/netstat"

// ListeningPorts lists every TCP socket in the listen state and every bound UDP
// socket on this machine, with the owning program where the operating system
// will say. It only reads: nothing is closed or changed.
func (a *App) ListeningPorts() (netstat.Report, error) {
	return netstat.List()
}
