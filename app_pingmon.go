package main

import "chit/internal/tools/pingmon"

// StartPingMonitor pings p.Targets on a repeating interval and streams one
// sample per target per round. Stop it with CancelJob.
func (a *App) StartPingMonitor(p pingmon.Params) (string, error) {
	return pingmon.New(a.jobs).StartMonitor(p)
}
