package main

import "chit/internal/tools/rawprint"

// QueryPrinter opens one connection to the printer and asks PJL what it is and
// whether it is online. It sends nothing that can cause a page to print.
func (a *App) QueryPrinter(p rawprint.Params) (rawprint.Result, error) {
	return rawprint.Query(a.ctx, p)
}

// SendPrinterTestPage sends a plain text test page and a form feed, which makes
// the printer physically print a sheet of paper. It is a separate method from
// QueryPrinter, rather than a flag on one, so nothing can print by accident.
func (a *App) SendPrinterTestPage(p rawprint.Params) (rawprint.Result, error) {
	return rawprint.SendTestPage(a.ctx, p)
}
