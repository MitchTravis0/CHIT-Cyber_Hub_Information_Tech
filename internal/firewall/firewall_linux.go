package firewall

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// units are asked in this order, and the first active one wins. ufw and
// firewalld come first because they are front ends: on a machine running ufw,
// iptables is loaded too, and naming iptables would send the tech to edit rules
// that ufw is going to overwrite.
var units = []struct{ unit, name string }{
	{"ufw", "ufw"},
	{"firewalld", "firewalld"},
	{"nftables", "nftables"},
	{"iptables", "iptables"},
}

// detect asks systemd which firewall service is running. systemctl is used
// rather than reading the rules directly because reading nftables or iptables
// rules needs root, and this must work as an ordinary user: a tool that asks
// for elevation to tell you why something is blocked is worse than one that
// says nothing.
//
// A machine with no systemd, or no service running, gets an empty string and
// the page shows nothing extra.
func detect() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, u := range units {
		if isActive(ctx, u.unit) {
			return u.name
		}
	}
	return ""
}

// isActive is true only for the exact word "active". systemctl exits non-zero
// for an inactive unit, which is not an error here, so the exit code is
// ignored and only the word is read. "activating" and "failed" are not active.
func isActive(ctx context.Context, unit string) bool {
	out, _ := exec.CommandContext(ctx, "systemctl", "is-active", unit).Output()
	return strings.TrimSpace(string(out)) == "active"
}
