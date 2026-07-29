package main

import "chit/internal/tools/sitecheck"

// CheckSite fetches one URL and reports what happened: status, redirects, where
// the time went, and the state of the certificate.
func (a *App) CheckSite(params sitecheck.Params) (sitecheck.Result, error) {
	return sitecheck.Check(a.ctx, params)
}
