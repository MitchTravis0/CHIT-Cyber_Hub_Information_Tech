package main

import "chit/internal/tools/triage"

// StartInternetTriage runs the connectivity ladder and streams one result per
// rung. Stop it with CancelJob.
func (a *App) StartInternetTriage(p triage.Params) (string, error) {
	return triage.New(a.jobs).StartTriage(p)
}
