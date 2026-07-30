package main

import "chit/internal/firewall"

// FirewallHint tells the page whether a firewall on this computer might be
// dropping the connections the other machine is making to a port CHIT just
// opened. It is shared by the three tools that open an inbound socket rather
// than being three copies, and it never changes a rule or asks for elevation.
//
// Everything is empty when nothing was detected, which the pages render as
// nothing at all.
func (a *App) FirewallHint(port int, proto string) firewall.Hint {
	return firewall.Check(port, proto)
}
