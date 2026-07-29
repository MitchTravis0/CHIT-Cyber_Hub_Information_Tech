package main

import "chit/internal/tools/startup"

// StartupItems lists what launches at login and what services are configured on
// this machine. Everything is read only: nothing here can enable, disable or
// start anything, because all of those need administrator rights.
func (a *App) StartupItems() (startup.Report, error) {
	return startup.List()
}
