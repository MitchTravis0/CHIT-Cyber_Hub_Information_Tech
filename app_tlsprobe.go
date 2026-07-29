package main

import "chit/internal/tools/tlsprobe"

// ProbeTLS tries one handshake per TLS version against a server and reports
// which versions it accepts.
func (a *App) ProbeTLS(p tlsprobe.Params) (tlsprobe.Report, error) {
	return tlsprobe.Probe(a.ctx, p)
}
