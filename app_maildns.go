package main

import "chit/internal/tools/maildns"

// CheckEmailDNS reads a domain's MX, SPF, DKIM and DMARC records and explains
// what they allow.
func (a *App) CheckEmailDNS(p maildns.Params) (maildns.Report, error) {
	return maildns.Check(a.ctx, p)
}

// EmailDKIMSelectors lists the common selectors CHIT probes, so the page can
// name them instead of implying it checked everything.
func (a *App) EmailDKIMSelectors() ([]string, error) {
	return maildns.CommonSelectors, nil
}
