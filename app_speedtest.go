package main

import "chit/internal/tools/speedtest"

// StartSpeedTest measures latency, then download, then upload, streaming a
// throughput sample about five times a second. Stop it with CancelJob.
func (a *App) StartSpeedTest(params speedtest.Params) (string, error) {
	return speedtest.New(a.jobs).StartTest(params)
}
