package main

import "chit/internal/tools/lanspeed"

// StartLANSpeed puts a generated data stream on a small web server and streams
// one result per pull. The server runs until CancelJob stops it.
//
// The page gets the addresses to offer from the already bound
// FileDropAddresses, so there is no second address binding.
func (a *App) StartLANSpeed(p lanspeed.Params) (string, error) {
	return lanspeed.Shared(a.jobs).StartSpeed(p, primaryIP())
}

// LANSpeedSession returns the link a running test is serving, so the page can
// show it. It returns an empty Session when that job is not a throughput test
// or is no longer running.
func (a *App) LANSpeedSession(jobID string) (lanspeed.Session, error) {
	return lanspeed.Shared(a.jobs).SessionFor(jobID), nil
}
