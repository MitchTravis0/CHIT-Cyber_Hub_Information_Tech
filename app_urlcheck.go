package main

import "chit/internal/tools/urlcheck"

// InspectLink follows a suspicious link to its real destination and reports what
// is worth knowing about where it ends up. Nothing on the page is loaded, run or
// rendered: each address is only asked where it points next.
func (a *App) InspectLink(params urlcheck.Params) (urlcheck.Report, error) {
	return urlcheck.DefaultClient().Inspect(a.ctx, params)
}
