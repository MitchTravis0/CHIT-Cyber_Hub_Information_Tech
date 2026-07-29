package main

import "chit/internal/tools/wifi"

// WiFiInfo reports what this machine's wireless adapters are connected to right
// now. It only reads: nothing is connected, disconnected or changed.
func (a *App) WiFiInfo() (wifi.Report, error) {
	return wifi.List()
}
