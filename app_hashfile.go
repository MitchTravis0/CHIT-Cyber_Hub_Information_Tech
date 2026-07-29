package main

import (
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/tools/hashfile"
)

// PickHashFile opens the native file dialog. A cancelled dialog returns an empty
// string and no error, which the page treats as "nothing chosen" rather than a
// failure.
func (a *App) PickHashFile() (string, error) {
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose a file to check",
	})
}

// StartFileHash reads one file once and emits its MD5, SHA-1 and SHA-256 as a
// single "digest" item when it reaches the end. Stop it with CancelJob.
func (a *App) StartFileHash(params hashfile.Params) (string, error) {
	return hashfile.New(a.jobs).StartHash(params)
}

// CompareHash decides whether a pasted value matches one of the three hashes.
// It is separate from the job so a tech can paste the expected value after the
// file has already been read, without hashing a 4 GB ISO twice.
func (a *App) CompareHash(expected string, digests hashfile.Digests) (hashfile.Verdict, error) {
	return hashfile.Compare(expected, digests), nil
}
