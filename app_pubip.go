package main

import "chit/internal/tools/pubip"

// PublicIP asks an outside service what address this machine appears from, and
// who owns it. It is one of the few tools that talks to the internet, so the UI
// says so plainly.
func (a *App) PublicIP() (pubip.Info, error) {
	return pubip.DefaultClient().Lookup(a.ctx)
}
