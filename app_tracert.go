package main

import "chit/internal/tools/tracert"

// StartTraceroute follows the path to p.Host and streams one result per hop.
// Stop it with CancelJob.
func (a *App) StartTraceroute(p tracert.Params) (string, error) {
	return tracert.New(a.jobs).StartTrace(p)
}
