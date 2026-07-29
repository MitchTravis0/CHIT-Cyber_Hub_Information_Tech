package main

import (
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/tools/logview"
)

// PickLogFile opens the native file dialog. A cancelled dialog returns an empty
// string and no error, which the page treats as "nothing chosen" rather than a
// failure.
func (a *App) PickLogFile() (string, error) {
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose a log file to read",
	})
}

// OpenLog checks that a file can be read as text and describes it. It reads at
// most the first 8 KiB, so a 4 GB file opens as fast as a 4 KB one.
func (a *App) OpenLog(path string) (logview.Info, error) {
	return logview.Open(path)
}

// ReadLog returns one window of lines. It never reads the whole file.
func (a *App) ReadLog(p logview.ReadParams) (logview.Chunk, error) {
	return logview.Read(p)
}

// StartLogSearch scans the whole file for a plain substring and streams one
// result per matching line. Stop it with CancelJob.
func (a *App) StartLogSearch(p logview.SearchParams) (string, error) {
	return logview.New(a.jobs).StartSearch(p)
}
