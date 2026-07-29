package main

import "chit/internal/tools/portlisten"

// StartPortListener opens a listener on one port and streams a row for every
// connection or datagram that arrives. Stop it with CancelJob, which releases
// the port.
//
// The page gets the addresses to offer from the already bound
// FileDropAddresses, so there is no second address binding.
func (a *App) StartPortListener(p portlisten.Params) (string, error) {
	return portlisten.New(a.jobs).StartListen(p)
}
