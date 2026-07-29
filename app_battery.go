package main

import "chit/internal/tools/battery"

// BatteryHealth reports how much of its original charge this machine's battery
// still holds. It only reads: no power setting is changed.
func (a *App) BatteryHealth() (battery.Report, error) {
	return battery.List()
}
