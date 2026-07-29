package main

import (
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/netinfo"
	"chit/internal/tools/filedrop"
)

// PickDropFiles opens the native multiple-file dialog. A cancelled dialog
// returns an empty slice and no error, which the page treats as "nothing
// chosen" rather than a failure.
func (a *App) PickDropFiles() ([]string, error) {
	return wruntime.OpenMultipleFilesDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose the files to hand over",
	})
}

// PickDropFolder opens the native folder dialog for the folder that received
// files are written into.
func (a *App) PickDropFolder() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose where files sent to you should land",
	})
}

// FileDropAddresses lists the addresses another machine on this network could
// reach this one on, best first.
func (a *App) FileDropAddresses() ([]filedrop.Address, error) {
	return filedrop.Addresses(primaryIP())
}

// StartFileDrop puts the chosen files on a small web server and streams one
// result per transfer. The server runs until CancelJob stops it.
func (a *App) StartFileDrop(p filedrop.Params) (string, error) {
	return filedrop.Shared(a.jobs).StartDrop(p, primaryIP())
}

// FileDropSession returns the link a running drop is serving, so the page can
// show it and turn it into a QR code. It returns an empty Session when that job
// is not a file drop or is no longer running.
func (a *App) FileDropSession(jobID string) (filedrop.Session, error) {
	return filedrop.Shared(a.jobs).SessionFor(jobID), nil
}

// primaryIP is the address on the adapter this machine routes through, which is
// the one to offer first when a laptop is on wifi and a cable at once. An empty
// string simply means no address is marked primary.
func primaryIP() string {
	subnet, err := netinfo.PrimarySubnet()
	if err != nil {
		return ""
	}
	return subnet.IP
}
