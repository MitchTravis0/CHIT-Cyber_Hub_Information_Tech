package battery

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// parseKeyValue reads the "KEY=VALUE" form Linux uses in a power supply's uevent
// file. A line with no separator is skipped rather than half stored.
func parseKeyValue(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found || key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

// number reads a key as an integer, or 0.
func number(fields map[string]string, key string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(fields[key]), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// wattHoursFromCharge converts the amp-hour shape some drivers report. Charge
// alone is not a capacity, which is why the design voltage is required: mixing
// amp-hours and watt-hours would produce a number that looks like a capacity and
// is not.
func wattHoursFromCharge(chargeMicroAh, voltageMicroV int64) float64 {
	if chargeMicroAh <= 0 || voltageMicroV <= 0 {
		return 0
	}
	return (float64(chargeMicroAh) / 1e6) * (float64(voltageMicroV) / 1e6)
}

// wattHoursFromEnergy converts the watt-hour shape other drivers report.
func wattHoursFromEnergy(microWh int64) float64 {
	if microWh <= 0 {
		return 0
	}
	return float64(microWh) / 1e6
}

// healthPercent is the full charge as a percentage of the original. It is not
// capped at 100: a good cell often beats the figure printed on it, and capping
// would be inventing a value.
func healthPercent(designWh, fullWh float64) int {
	if designWh <= 0 || fullWh <= 0 {
		return 0
	}
	return int(math.Round(fullWh / designWh * 100))
}

// isPowerDeliverySource says whether a /sys/class/power_supply entry is a USB-C
// power delivery source rather than a laptop battery. This machine has four of
// them and each would otherwise render as its own card.
func isPowerDeliverySource(name string) bool {
	return strings.HasPrefix(name, "ucsi-source-psy-")
}

// linuxState maps POWER_SUPPLY_STATUS onto the four states.
func linuxState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "charging":
		return StateCharging
	case "discharging":
		return StateDischarging
	case "full":
		return StateFull
	}
	return StateUnknown
}

// batteryFromUevent builds one battery from a Linux power supply's uevent file.
// Both capacity shapes are handled; a driver that reports charge with no design
// voltage leaves the capacities at zero rather than guessing.
func batteryFromUevent(name, text string) Battery {
	fields := parseKeyValue(text)

	design := wattHoursFromEnergy(number(fields, "POWER_SUPPLY_ENERGY_FULL_DESIGN"))
	full := wattHoursFromEnergy(number(fields, "POWER_SUPPLY_ENERGY_FULL"))
	if design == 0 || full == 0 {
		volts := number(fields, "POWER_SUPPLY_VOLTAGE_MIN_DESIGN")
		design = wattHoursFromCharge(number(fields, "POWER_SUPPLY_CHARGE_FULL_DESIGN"), volts)
		full = wattHoursFromCharge(number(fields, "POWER_SUPPLY_CHARGE_FULL"), volts)
	}

	charge := -1
	if _, ok := fields["POWER_SUPPLY_CAPACITY"]; ok {
		charge = int(number(fields, "POWER_SUPPLY_CAPACITY"))
	}

	label := fields["POWER_SUPPLY_NAME"]
	if label == "" {
		label = name
	}

	return Battery{
		Name:          label,
		State:         linuxState(fields["POWER_SUPPLY_STATUS"]),
		ChargePercent: charge,
		DesignWh:      design,
		FullWh:        full,
		HealthPercent: healthPercent(design, full),
		CycleCount:    int(number(fields, "POWER_SUPPLY_CYCLE_COUNT")),
		Technology:    fields["POWER_SUPPLY_TECHNOLOGY"],
		Manufacturer:  fields["POWER_SUPPLY_MANUFACTURER"],
		Model:         fields["POWER_SUPPLY_MODEL_NAME"],
		Serial:        fields["POWER_SUPPLY_SERIAL_NUMBER"],
		Source:        "/sys/class/power_supply",
	}
}

// parseIoregPairs reads the `"Key" = Value` lines ioreg prints. The plist form
// (-a) is deliberately not used: it would need an XML plist parser and the keys
// are identical.
func parseIoregPairs(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `"`) {
			continue
		}
		key, rest, found := strings.Cut(line[1:], `"`)
		if !found || key == "" {
			continue
		}
		value, found := strings.CutPrefix(strings.TrimSpace(rest), "=")
		if !found {
			continue
		}
		out[key] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return out
}

// batteryFromIoreg builds one battery from ioreg's AppleSmartBattery node.
//
// The trap here is MaxCapacity. On Intel Macs it is a mAh figure; on Apple
// Silicon it is a percentage, and dividing a percentage by a mAh design capacity
// reports every Mac as 2% healthy. AppleRawMaxCapacity is preferred where it
// exists, and a MaxCapacity at or below 100 with no raw figure is read as the
// percentage it is.
func batteryFromIoreg(pairs map[string]string) (Battery, bool) {
	if len(pairs) == 0 {
		return Battery{}, false
	}

	value := func(key string) int64 {
		n, err := strconv.ParseInt(strings.TrimSpace(pairs[key]), 10, 64)
		if err != nil {
			return 0
		}
		return n
	}

	design := value("DesignCapacity")
	raw := value("AppleRawMaxCapacity")
	maxCapacity := value("MaxCapacity")
	millivolts := value("Voltage")

	b := Battery{
		Name:          "Battery",
		ChargePercent: -1,
		CycleCount:    int(value("CycleCount")),
		Manufacturer:  pairs["Manufacturer"],
		Model:         pairs["DeviceName"],
		Serial:        pairs["BatterySerialNumber"],
		Source:        "ioreg",
	}

	switch {
	case raw > 0 && design > 0:
		b.DesignWh = mahToWh(design, millivolts)
		b.FullWh = mahToWh(raw, millivolts)
		b.HealthPercent = healthPercent(float64(design), float64(raw))
	case maxCapacity > 0 && maxCapacity <= 100:
		// The Apple Silicon shape: a percentage, not a capacity.
		b.HealthPercent = int(maxCapacity)
	case maxCapacity > 0 && design > 0:
		b.DesignWh = mahToWh(design, millivolts)
		b.FullWh = mahToWh(maxCapacity, millivolts)
		b.HealthPercent = healthPercent(float64(design), float64(maxCapacity))
	}

	if current := value("CurrentCapacity"); current > 0 {
		switch {
		case raw > 0:
			b.ChargePercent = int(math.Round(float64(current) / float64(raw) * 100))
		case maxCapacity > 100:
			b.ChargePercent = int(math.Round(float64(current) / float64(maxCapacity) * 100))
		case current <= 100:
			b.ChargePercent = int(current)
		}
	}

	switch {
	case yesNo(pairs["FullyCharged"]):
		b.State = StateFull
	case yesNo(pairs["IsCharging"]):
		b.State = StateCharging
	case pairs["ExternalConnected"] != "":
		b.State = StateDischarging
	default:
		b.State = StateUnknown
	}

	return b, true
}

// mahToWh turns a milliamp-hour figure and a millivolt figure into watt-hours,
// or 0 when the voltage is missing.
func mahToWh(mah, millivolts int64) float64 {
	if mah <= 0 || millivolts <= 0 {
		return 0
	}
	return (float64(mah) / 1000) * (float64(millivolts) / 1000)
}

func yesNo(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "Yes")
}

// batteryStatusToState maps Win32_Battery.BatteryStatus. Only 3 means fully
// charged; 2 means running on mains and says nothing about the charge.
func batteryStatusToState(code int) string {
	switch code {
	case 1:
		return StateDischarging
	case 3:
		return StateFull
	case 6, 7, 8:
		return StateCharging
	}
	return StateUnknown
}

// windowsPayload mirrors the object the one PowerShell command emits. Every
// collection may be absent or empty and none of them is required.
type windowsPayload struct {
	Battery []map[string]any `json:"battery"`
	Static  []map[string]any `json:"static"`
	Full    []map[string]any `json:"full"`
	Cycles  []map[string]any `json:"cycles"`
}

// batteriesFromWindowsJSON reads what the CIM query returned. The three
// root\wmi classes are readable by an ordinary user on most machines and refused
// on some, so everything past Win32_Battery is optional.
func batteriesFromWindowsJSON(raw []byte) ([]Battery, bool) {
	var payload windowsPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	if len(payload.Battery) == 0 {
		return nil, false
	}

	health := false
	out := make([]Battery, 0, len(payload.Battery))
	for i, entry := range payload.Battery {
		b := Battery{
			Name:          jsonString(entry, "Name"),
			State:         batteryStatusToState(int(jsonNumber(entry, "BatteryStatus"))),
			ChargePercent: -1,
			Model:         jsonString(entry, "Name"),
			Serial:        jsonString(entry, "DeviceID"),
			Manufacturer:  jsonString(entry, "Manufacturer"),
			Technology:    jsonString(entry, "Chemistry"),
			Source:        "Get-CimInstance",
		}
		if b.Name == "" {
			b.Name = "Battery"
		}
		if remaining, ok := entry["EstimatedChargeRemaining"]; ok && remaining != nil {
			b.ChargePercent = int(jsonNumber(entry, "EstimatedChargeRemaining"))
		}

		// mWh in both classes, so the conversion is the same one.
		design := jsonNumber(at(payload.Static, i), "DesignedCapacity")
		full := jsonNumber(at(payload.Full, i), "FullChargedCapacity")
		if design > 0 && full > 0 {
			b.DesignWh = design / 1000
			b.FullWh = full / 1000
			b.HealthPercent = healthPercent(b.DesignWh, b.FullWh)
			health = true
		}
		b.CycleCount = int(jsonNumber(at(payload.Cycles, i), "CycleCount"))

		out = append(out, b)
	}
	return out, health
}

// at returns the nth element of a collection, or nil. The three root\wmi classes
// come back in the same order as Win32_Battery on a machine that answers all of
// them, and a machine with one battery is the only shape anybody has.
func at(items []map[string]any, i int) map[string]any {
	if i < len(items) {
		return items[i]
	}
	if len(items) == 1 {
		return items[0]
	}
	return nil
}

func jsonString(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	if value, ok := item[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func jsonNumber(item map[string]any, key string) float64 {
	if item == nil {
		return 0
	}
	switch value := item[key].(type) {
	case float64:
		return value
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}
