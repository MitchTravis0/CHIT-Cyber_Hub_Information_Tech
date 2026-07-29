//go:build linux

package netscan

import (
	"context"
	"os"
	"os/exec"

	"chit/internal/core"
)

// arpTable reads the kernel neighbour table. /proc/net/arp is preferred because
// it needs no helper binary; "ip neigh" covers systems where procfs is hidden
// (some containers) and net-tools is absent.
func arpTable(ctx context.Context) ([]ARPEntry, error) {
	if data, err := os.ReadFile("/proc/net/arp"); err == nil {
		if entries := parseProcNetARP(string(data)); len(entries) > 0 {
			return entries, nil
		}
	}
	out, err := runARPCommand(ctx, "ip", "neigh")
	if err != nil {
		return nil, core.Errorf(core.CodeNotFound,
			"CHIT could not read this computer's ARP cache, so some quiet devices and MAC addresses may be missing.")
	}
	return parseIPNeigh(out), nil
}

func hideWindow(*exec.Cmd) {}
