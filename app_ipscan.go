package main

import "chit/internal/tools/ipscan"

// StartIPScan sweeps a range of addresses and streams one result per address
// over the job events. Stop it with CancelJob.
func (a *App) StartIPScan(params ipscan.ScanParams) (string, error) {
	return ipscan.New(a.jobs).StartScan(params)
}

// ScanDefaults describes the network this machine is on, so the scanner can
// offer "scan my subnet" without the tech looking anything up.
func (a *App) ScanDefaults() (ipscan.Defaults, error) {
	return ipscan.SuggestedRange()
}
