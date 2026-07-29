package main

import "chit/internal/tools/portscan"

// StartPortScan checks the ports named in p.Ports on p.Host and streams one
// result per port. Stop it with CancelJob.
func (a *App) StartPortScan(p portscan.Params) (string, error) {
	return portscan.New(a.jobs).StartScan(p)
}
