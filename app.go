package main

import (
	"context"
	"runtime"

	"chit/internal/core"
	"chit/internal/store"
)

// Version is shown on the settings page, in bug reports, and is what the update
// check compares against the latest GitHub release. Bumping this is not the same
// as tagging: no agent on this project runs a write-side git command, so the
// matching "v1.0.2" tag is pushed by the repo owner.
//
// Bump this BEFORE cutting the tag. v1.0.1 and 1.0.2 were both released with
// this constant still reading 1.0.0, so those binaries report the wrong version
// on the settings page and tell every user an update is available for ever.
const Version = "1.0.2"

// App is the single bound struct. Tools hang their StartX / request-response
// methods off it and use a.jobs for anything long running.
type App struct {
	ctx   context.Context
	jobs  *core.JobManager
	store *store.Store
}

func NewApp(st *store.Store) *App {
	return &App{jobs: core.NewJobManager(), store: st}
}

// startup keeps the Wails context so the job engine can emit events.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.jobs.SetContext(ctx)
}

// CancelJob stops a running job. Every streaming tool shares this one entry
// point, so no tool implements cancellation itself.
func (a *App) CancelJob(jobID string) error {
	return a.jobs.Cancel(jobID)
}

// AppInfo is everything the settings page needs to describe this install.
type AppInfo struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	StoreDir string `json:"storeDir"`
	Portable bool   `json:"portable"`
}

func (a *App) GetAppInfo() (AppInfo, error) {
	return AppInfo{
		Version:  Version,
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
		StoreDir: a.store.Dir(),
		Portable: a.store.Portable(),
	}, nil
}
