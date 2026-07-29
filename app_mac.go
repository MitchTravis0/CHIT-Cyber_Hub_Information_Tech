package main

import (
	"chit/internal/core"
	"chit/internal/ouidb"
)

// LookupMAC names the manufacturer behind a MAC address and flags randomized,
// multicast and broadcast addresses. The scanner's Vendor column and the
// standalone lookup tool both go through here.
func (a *App) LookupMAC(mac string) (ouidb.Info, error) {
	info, ok := ouidb.Describe(mac)
	if !ok {
		return info, core.Errorf(core.CodeInvalidInput,
			"That is not a MAC address. Enter 12 hex digits, for example AA:BB:CC:DD:EE:FF.")
	}
	return info, nil
}

// MACDatabaseInfo lets the UI show how old the embedded vendor list is, so a
// missing vendor can be blamed on a stale database rather than the device.
func (a *App) MACDatabaseInfo() (ouidb.Meta, error) {
	return ouidb.Metadata(), nil
}
