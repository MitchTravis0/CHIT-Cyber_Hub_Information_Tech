package main

import (
	"chit/internal/update"
)

// CheckForUpdate asks GitHub whether a newer release exists and returns a
// sentence plus a link. It never downloads or replaces anything, and it only
// runs when someone presses the button on the settings page.
func (a *App) CheckForUpdate() (update.Result, error) {
	return update.Check(a.ctx, Version)
}
