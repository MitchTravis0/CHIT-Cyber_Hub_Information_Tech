package main

import (
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/selfupdate"
)

// CheckForUpdate asks GitHub whether a newer release exists and whether this
// build can install it itself. It only runs when someone presses the button on
// the settings page; CHIT never contacts GitHub on its own.
func (a *App) CheckForUpdate() (selfupdate.Status, error) {
	return selfupdate.Check(a.ctx, Version)
}

// StartInstallUpdate downloads the newer release, verifies it against the
// checksum the release published, and swaps it into place as a cancellable
// job. It takes no input: the job asks GitHub itself, so the page can never
// hand it a URL.
func (a *App) StartInstallUpdate() (string, error) {
	return selfupdate.StartInstall(a.jobs, Version)
}

// RestartForUpdate starts the newly installed version and quits this one. Only
// meaningful after StartInstallUpdate has finished; closing and reopening CHIT
// by hand does the same thing.
func (a *App) RestartForUpdate() (bool, error) {
	if err := selfupdate.Relaunch(); err != nil {
		return false, err
	}
	wruntime.Quit(a.ctx)
	return true, nil
}
