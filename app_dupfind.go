package main

import (
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/tools/dupfind"
)

// PickDuplicateFolder opens the native folder dialog. A cancelled dialog
// returns an empty string and no error, which the page treats as "nothing
// chosen" rather than a failure.
func (a *App) PickDuplicateFolder() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose the folder to look for duplicates in",
	})
}

// StartDuplicateScan looks for files with identical contents under one folder
// and streams one result per group of duplicates. It never changes a file.
// Stop it with CancelJob.
func (a *App) StartDuplicateScan(p dupfind.Params) (string, error) {
	return dupfind.New(a.jobs).StartScan(p)
}
