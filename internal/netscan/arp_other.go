//go:build !linux && !windows

package netscan

import (
	"context"
	"os/exec"

	"chit/internal/core"
)

// arpTable covers macOS and the BSDs. -n keeps arp from doing a reverse lookup
// on every entry, which would make the readback take seconds.
func arpTable(ctx context.Context) ([]ARPEntry, error) {
	out, err := runARPCommand(ctx, "arp", "-a", "-n")
	if err != nil {
		return nil, core.Errorf(core.CodeNotFound,
			"CHIT could not read this computer's ARP cache, so some quiet devices and MAC addresses may be missing.")
	}
	return parseARPCommand(out), nil
}

func hideWindow(*exec.Cmd) {}
