//go:build linux

package battery

import (
	"os"
	"path/filepath"
	"strings"
)

const powerSupplyDir = "/sys/class/power_supply"

// collect reads /sys/class/power_supply, which is world-readable and needs no
// privileges at all.
func collect(r *Report) {
	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil {
		r.Note = noteFor("linux", 0, false)
		return
	}

	health := false
	for _, entry := range entries {
		name := entry.Name()
		// A USB-C power delivery source is not a laptop battery. This machine has
		// four of them and each would otherwise render as its own card.
		if isPowerDeliverySource(name) {
			continue
		}

		text := readFile(filepath.Join(powerSupplyDir, name, "uevent"))
		if text == "" {
			continue
		}
		fields := parseKeyValue(text)

		switch strings.ToLower(fields["POWER_SUPPLY_TYPE"]) {
		case "battery":
			if fields["POWER_SUPPLY_PRESENT"] == "0" {
				continue
			}
			b := batteryFromUevent(name, text)
			if b.HealthPercent > 0 {
				health = true
			}
			r.Batteries = append(r.Batteries, b)
		case "mains", "usb":
			r.HasAC = true
			if fields["POWER_SUPPLY_ONLINE"] == "1" {
				r.OnAC = true
			}
		}
	}

	r.Note = noteFor("linux", len(r.Batteries), health)
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
