package main

import (
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/tools/diskbench"
)

// PickBenchFolder opens the native folder dialog for the folder the test file is
// written into. A cancelled dialog returns an empty string and no error, which
// the page treats as "nothing chosen" rather than a failure.
func (a *App) PickBenchFolder() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose the folder to test",
	})
}

// StartDiskSpeed writes and then reads back one temporary file inside the chosen
// folder and streams the rate as it goes. Stop it with CancelJob; the file is
// removed either way.
func (a *App) StartDiskSpeed(p diskbench.Params) (string, error) {
	return diskbench.New(a.jobs).StartTest(p)
}
