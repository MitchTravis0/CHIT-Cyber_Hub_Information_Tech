package main

import "chit/internal/tools/swlist"

// InstalledSoftware lists what is installed on this machine. It only reads:
// nothing is installed, updated or removed.
func (a *App) InstalledSoftware() (swlist.Report, error) {
	return swlist.List()
}
