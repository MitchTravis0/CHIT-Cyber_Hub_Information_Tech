package main

import (
	"os"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/core"
	"chit/internal/tools/diskscan"
)

// PickScanFolder opens the native folder dialog. A cancelled dialog returns an
// empty string and no error, which the page treats as "nothing chosen" rather
// than a failure.
func (a *App) PickScanFolder() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose the folder to measure",
	})
}

// DiskScanHome is the folder the page offers before the user picks one: the
// signed-in user's home directory, which is where the space nearly always went.
func (a *App) DiskScanHome() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", core.Errorf(core.CodeNotFound,
			"CHIT could not work out where your home folder is. Press Choose folder and pick one.")
	}
	return dir, nil
}

// StartDiskScan measures every folder and file under one folder and streams one
// result per immediate child as its subtree finishes. Stop it with CancelJob.
func (a *App) StartDiskScan(p diskscan.Params) (string, error) {
	return diskscan.New(a.jobs).StartScan(p)
}
