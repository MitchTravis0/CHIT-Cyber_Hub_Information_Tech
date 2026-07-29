//go:build windows

package netscan

import (
	"context"
	"os/exec"
	"syscall"

	"chit/internal/core"
)

func arpTable(ctx context.Context) ([]ARPEntry, error) {
	out, err := runARPCommand(ctx, "arp", "-a")
	if err != nil {
		return nil, core.Errorf(core.CodeNotFound,
			"CHIT could not read this computer's ARP cache, so some quiet devices and MAC addresses may be missing.")
	}
	return parseARPCommand(out), nil
}

// hideWindow keeps a console window from flashing over the app when the helper
// command runs.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
