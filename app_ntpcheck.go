package main

import "chit/internal/tools/ntpcheck"

// CheckNTPTime asks each server for the time and reports how far this
// computer's clock is from it.
func (a *App) CheckNTPTime(p ntpcheck.Params) (ntpcheck.Report, error) {
	return ntpcheck.Check(a.ctx, p)
}
