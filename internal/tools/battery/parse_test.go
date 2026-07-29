package battery

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

// The real uevent captured from this machine's BAT1 before the parser was
// written, so the fixture is the kernel's output rather than this package's idea
// of it.
const bat1Uevent = `DEVTYPE=power_supply
POWER_SUPPLY_NAME=BAT1
POWER_SUPPLY_TYPE=Battery
POWER_SUPPLY_STATUS=Charging
POWER_SUPPLY_PRESENT=1
POWER_SUPPLY_TECHNOLOGY=Li-ion
POWER_SUPPLY_CYCLE_COUNT=23
POWER_SUPPLY_VOLTAGE_MIN_DESIGN=15480000
POWER_SUPPLY_VOLTAGE_NOW=17814000
POWER_SUPPLY_CURRENT_NOW=159000
POWER_SUPPLY_CHARGE_FULL_DESIGN=3915000
POWER_SUPPLY_CHARGE_FULL=3992000
POWER_SUPPLY_CHARGE_NOW=3955000
POWER_SUPPLY_CAPACITY=99
POWER_SUPPLY_CAPACITY_LEVEL=Normal
POWER_SUPPLY_TYPE=Battery
POWER_SUPPLY_MODEL_NAME=FRANGWA
POWER_SUPPLY_MANUFACTURER=NVT
POWER_SUPPLY_SERIAL_NUMBER=03A5
`

// The other reporting shape: microwatt-hours rather than microamp-hours.
const energyUevent = `POWER_SUPPLY_NAME=BAT0
POWER_SUPPLY_TYPE=Battery
POWER_SUPPLY_STATUS=Discharging
POWER_SUPPLY_PRESENT=1
POWER_SUPPLY_ENERGY_FULL_DESIGN=60600000
POWER_SUPPLY_ENERGY_FULL=48480000
POWER_SUPPLY_ENERGY_NOW=24000000
POWER_SUPPLY_CAPACITY=50
POWER_SUPPLY_CYCLE_COUNT=412
`

func TestParseKeyValue(t *testing.T) {
	fields := parseKeyValue(bat1Uevent)
	if fields["POWER_SUPPLY_NAME"] != "BAT1" {
		t.Errorf("name = %q", fields["POWER_SUPPLY_NAME"])
	}
	if fields["POWER_SUPPLY_CYCLE_COUNT"] != "23" {
		t.Errorf("cycles = %q", fields["POWER_SUPPLY_CYCLE_COUNT"])
	}
	// A repeated key takes the last value, which is what the kernel means by it.
	if fields["POWER_SUPPLY_TYPE"] != "Battery" {
		t.Errorf("type = %q", fields["POWER_SUPPLY_TYPE"])
	}
	if len(parseKeyValue("no separator here\n\n")) != 0 {
		t.Error("a line with no = must be skipped, not half stored")
	}
	if len(parseKeyValue("")) != 0 {
		t.Error("an empty input yields no fields")
	}
}

func TestBatteryFromUeventChargeShape(t *testing.T) {
	b := batteryFromUevent("BAT1", bat1Uevent)

	if b.Name != "BAT1" || b.Technology != "Li-ion" || b.Manufacturer != "NVT" ||
		b.Model != "FRANGWA" || b.Serial != "03A5" {
		t.Errorf("identity fields wrong: %+v", b)
	}
	if b.State != StateCharging {
		t.Errorf("state = %q", b.State)
	}
	if b.ChargePercent != 99 {
		t.Errorf("charge = %d, want 99", b.ChargePercent)
	}
	if b.CycleCount != 23 {
		t.Errorf("cycles = %d, want 23", b.CycleCount)
	}
	// 3.915 Ah at 15.48 V. The literal came from that rule, computed with python3
	// before this function was written.
	if math.Abs(b.DesignWh-60.6042) > 1e-4 {
		t.Errorf("designWh = %v, want 60.6042", b.DesignWh)
	}
	if math.Abs(b.FullWh-61.79616) > 1e-4 {
		t.Errorf("fullWh = %v, want 61.79616", b.FullWh)
	}
	// A real battery measuring above its printed capacity. Capping this at 100
	// would be inventing a value.
	if b.HealthPercent != 102 {
		t.Errorf("health = %d, want 102 (uncapped)", b.HealthPercent)
	}
}

func TestBatteryFromUeventEnergyShape(t *testing.T) {
	b := batteryFromUevent("BAT0", energyUevent)
	if math.Abs(b.DesignWh-60.6) > 1e-6 || math.Abs(b.FullWh-48.48) > 1e-6 {
		t.Errorf("capacities = %v / %v", b.DesignWh, b.FullWh)
	}
	if b.HealthPercent != 80 {
		t.Errorf("health = %d, want 80", b.HealthPercent)
	}
	if b.State != StateDischarging || b.ChargePercent != 50 || b.CycleCount != 412 {
		t.Errorf("battery = %+v", b)
	}
}

func TestBatteryFromUeventWithoutADesignVoltage(t *testing.T) {
	// Amp-hours with no voltage is not a capacity. Reporting it as one would put
	// a number on screen that looks like watt-hours and is not.
	b := batteryFromUevent("BAT1", `POWER_SUPPLY_NAME=BAT1
POWER_SUPPLY_TYPE=Battery
POWER_SUPPLY_CHARGE_FULL_DESIGN=3915000
POWER_SUPPLY_CHARGE_FULL=3992000
POWER_SUPPLY_CAPACITY=99
`)
	if b.DesignWh != 0 || b.FullWh != 0 || b.HealthPercent != 0 {
		t.Errorf("without a design voltage the capacities must stay zero, got %+v", b)
	}
	if b.ChargePercent != 99 {
		t.Errorf("the charge is still known: %d", b.ChargePercent)
	}
}

func TestBatteryFromUeventWithNoCapacityField(t *testing.T) {
	b := batteryFromUevent("BAT1", "POWER_SUPPLY_TYPE=Battery\n")
	if b.ChargePercent != -1 {
		t.Errorf("an absent charge must be -1 (unknown), not 0 (empty), got %d", b.ChargePercent)
	}
	if b.Name != "BAT1" {
		t.Errorf("the name falls back to the folder name, got %q", b.Name)
	}
}

func TestWattHoursFromCharge(t *testing.T) {
	// 3.915 Ah at 15.48 V = 60.6042 Wh. Computed with python3 from the rule
	// before this function existed.
	if got := wattHoursFromCharge(3915000, 15480000); math.Abs(got-60.6042) > 1e-6 {
		t.Errorf("got %v, want 60.6042", got)
	}
	if wattHoursFromCharge(3915000, 0) != 0 {
		t.Error("no voltage must give no capacity")
	}
	if wattHoursFromCharge(0, 15480000) != 0 {
		t.Error("no charge must give no capacity")
	}
	if wattHoursFromCharge(-1, -1) != 0 {
		t.Error("negatives must give no capacity")
	}
}

func TestWattHoursFromEnergy(t *testing.T) {
	if got := wattHoursFromEnergy(60600000); math.Abs(got-60.6) > 1e-9 {
		t.Errorf("got %v, want 60.6", got)
	}
	if wattHoursFromEnergy(0) != 0 || wattHoursFromEnergy(-1) != 0 {
		t.Error("zero and negative must give no capacity")
	}
}

func TestHealthPercent(t *testing.T) {
	tests := []struct {
		design, full float64
		want         int
	}{
		{60.6042, 61.79616, 102}, // above 100 and not capped
		{60.0, 30.0, 50},
		{60.0, 60.0, 100},
		{60.0, 0, 0},
		{0, 60.0, 0},
		{0, 0, 0},
		{100, 61.4, 61}, // rounds down
		{100, 61.6, 62}, // rounds up
	}
	for _, tt := range tests {
		if got := healthPercent(tt.design, tt.full); got != tt.want {
			t.Errorf("healthPercent(%v,%v) = %d, want %d", tt.design, tt.full, got, tt.want)
		}
	}
}

func TestIsPowerDeliverySource(t *testing.T) {
	// The four real names from this machine, which would otherwise be four cards.
	for _, name := range []string{
		"ucsi-source-psy-USBC000:001", "ucsi-source-psy-USBC000:002",
		"ucsi-source-psy-USBC000:003", "ucsi-source-psy-USBC000:004",
	} {
		if !isPowerDeliverySource(name) {
			t.Errorf("%s should be skipped", name)
		}
	}
	for _, name := range []string{"BAT0", "BAT1", "ACAD", "AC", "CMB0"} {
		if isPowerDeliverySource(name) {
			t.Errorf("%s must not be skipped by this rule", name)
		}
	}
}

func TestLinuxState(t *testing.T) {
	tests := map[string]string{
		"Charging": StateCharging, "charging": StateCharging,
		"Discharging":  StateDischarging,
		"Full":         StateFull,
		"Not charging": StateUnknown, "Unknown": StateUnknown, "": StateUnknown,
	}
	for in, want := range tests {
		if got := linuxState(in); got != want {
			t.Errorf("linuxState(%q) = %q, want %q", in, got, want)
		}
	}
}

// An Intel Mac's ioreg output: a mAh MaxCapacity alongside a raw figure.
const ioregIntel = `
    "CycleCount" = 412
    "DesignCapacity" = 4790
    "AppleRawMaxCapacity" = 4200
    "MaxCapacity" = 4200
    "CurrentCapacity" = 3360
    "Voltage" = 12600
    "ExternalConnected" = Yes
    "IsCharging" = Yes
    "FullyCharged" = No
    "Manufacturer" = "SMP"
    "DeviceName" = "bq20z451"
    "BatterySerialNumber" = "D86########A2LQ"
`

// An Apple Silicon Mac: MaxCapacity is a percentage and there is no raw figure.
// Dividing this by a mAh design capacity would report the Mac as 2% healthy.
const ioregSilicon = `
    "CycleCount" = 118
    "DesignCapacity" = 8694
    "MaxCapacity" = 87
    "CurrentCapacity" = 74
    "Voltage" = 12600
    "ExternalConnected" = No
    "IsCharging" = No
    "FullyCharged" = No
`

func TestParseIoregPairs(t *testing.T) {
	pairs := parseIoregPairs(ioregIntel)
	if pairs["CycleCount"] != "412" || pairs["DesignCapacity"] != "4790" {
		t.Errorf("numbers wrong: %v", pairs)
	}
	if pairs["Manufacturer"] != "SMP" || pairs["DeviceName"] != "bq20z451" {
		t.Errorf("quoted values must lose their quotes: %v", pairs)
	}
	if pairs["ExternalConnected"] != "Yes" {
		t.Errorf("ExternalConnected = %q", pairs["ExternalConnected"])
	}
	if len(parseIoregPairs("")) != 0 {
		t.Error("an empty input yields no pairs")
	}
	if len(parseIoregPairs("+-o AppleSmartBattery  <class AppleSmartBattery>\n")) != 0 {
		t.Error("ioreg's tree lines must be skipped")
	}
}

func TestBatteryFromIoregIntel(t *testing.T) {
	b, ok := batteryFromIoreg(parseIoregPairs(ioregIntel))
	if !ok {
		t.Fatal("wanted a battery")
	}
	if b.CycleCount != 412 {
		t.Errorf("cycles = %d", b.CycleCount)
	}
	// 4200 of 4790 mAh.
	if b.HealthPercent != 88 {
		t.Errorf("health = %d, want 88", b.HealthPercent)
	}
	// 4.79 Ah at 12.6 V = 60.354 Wh.
	if math.Abs(b.DesignWh-60.354) > 1e-6 {
		t.Errorf("designWh = %v, want 60.354", b.DesignWh)
	}
	if b.ChargePercent != 80 {
		t.Errorf("charge = %d, want 80", b.ChargePercent)
	}
	if b.State != StateCharging {
		t.Errorf("state = %q", b.State)
	}
	if b.Model != "bq20z451" || b.Manufacturer != "SMP" {
		t.Errorf("identity = %+v", b)
	}
}

func TestBatteryFromIoregAppleSilicon(t *testing.T) {
	// The trap: MaxCapacity is a percentage here. Treating it as mAh would
	// report every Apple Silicon Mac as 1% healthy.
	b, ok := batteryFromIoreg(parseIoregPairs(ioregSilicon))
	if !ok {
		t.Fatal("wanted a battery")
	}
	if b.HealthPercent != 87 {
		t.Errorf("health = %d, want 87 read as the percentage it is", b.HealthPercent)
	}
	if b.FullWh != 0 || b.DesignWh != 0 {
		t.Errorf("a percentage is not a capacity, so both must stay zero: %v / %v", b.DesignWh, b.FullWh)
	}
	if b.ChargePercent != 74 {
		t.Errorf("charge = %d, want 74", b.ChargePercent)
	}
	if b.State != StateDischarging {
		t.Errorf("state = %q", b.State)
	}
}

func TestBatteryFromIoregEmpty(t *testing.T) {
	if _, ok := batteryFromIoreg(map[string]string{}); ok {
		t.Error("no pairs must yield no battery")
	}
}

func TestBatteryStatusToState(t *testing.T) {
	tests := map[int]string{
		1: StateDischarging,
		2: StateUnknown, // on mains, which says nothing about the charge
		3: StateFull,
		6: StateCharging, 7: StateCharging, 8: StateCharging,
		4: StateUnknown, 5: StateUnknown, 99: StateUnknown, 0: StateUnknown,
	}
	for code, want := range tests {
		if got := batteryStatusToState(code); got != want {
			t.Errorf("batteryStatusToState(%d) = %q, want %q", code, got, want)
		}
	}
}

const windowsFull = `{"battery":[{"Name":"DELL 5XJ28","BatteryStatus":2,"EstimatedChargeRemaining":97,"DeviceID":"12345","Manufacturer":"SMP","Chemistry":6}],` +
	`"static":[{"DesignedCapacity":60000}],"full":[{"FullChargedCapacity":51000}],"cycles":[{"CycleCount":412}]}`

const windowsNoWMI = `{"battery":[{"Name":"DELL 5XJ28","BatteryStatus":1,"EstimatedChargeRemaining":42,"DeviceID":"12345"}],` +
	`"static":[],"full":[],"cycles":[]}`

func TestBatteriesFromWindowsJSON(t *testing.T) {
	batteries, health := batteriesFromWindowsJSON([]byte(windowsFull))
	if len(batteries) != 1 {
		t.Fatalf("got %d batteries", len(batteries))
	}
	if !health {
		t.Error("health should be available in this fixture")
	}
	b := batteries[0]
	if b.ChargePercent != 97 || b.State != StateUnknown {
		t.Errorf("charge/state = %d %q", b.ChargePercent, b.State)
	}
	// 60000 and 51000 mWh become 60 and 51 Wh, so 85%.
	if b.DesignWh != 60 || b.FullWh != 51 || b.HealthPercent != 85 {
		t.Errorf("capacities = %v / %v health %d", b.DesignWh, b.FullWh, b.HealthPercent)
	}
	if b.CycleCount != 412 {
		t.Errorf("cycles = %d", b.CycleCount)
	}
	if b.Serial != "12345" || b.Model != "DELL 5XJ28" {
		t.Errorf("identity = %+v", b)
	}
}

func TestBatteriesFromWindowsJSONWithoutTheWMIClasses(t *testing.T) {
	// The case on a machine that refuses root\wmi to an ordinary user: charge and
	// state still work, health does not, and nothing is invented.
	batteries, health := batteriesFromWindowsJSON([]byte(windowsNoWMI))
	if len(batteries) != 1 {
		t.Fatalf("got %d batteries", len(batteries))
	}
	if health {
		t.Error("health must not be claimed when the class was refused")
	}
	b := batteries[0]
	if b.HealthPercent != 0 || b.DesignWh != 0 || b.CycleCount != 0 {
		t.Errorf("nothing may be invented: %+v", b)
	}
	if b.ChargePercent != 42 || b.State != StateDischarging {
		t.Errorf("charge and state must still work: %d %q", b.ChargePercent, b.State)
	}
}

func TestBatteriesFromWindowsJSONEdges(t *testing.T) {
	if _, _ = batteriesFromWindowsJSON([]byte(`{"battery":[]}`)); true {
		batteries, _ := batteriesFromWindowsJSON([]byte(`{"battery":[]}`))
		if len(batteries) != 0 {
			t.Error("an empty battery collection yields nothing")
		}
	}
	if batteries, _ := batteriesFromWindowsJSON([]byte("not json")); len(batteries) != 0 {
		t.Error("unreadable output yields nothing rather than a panic")
	}
	two := `{"battery":[{"Name":"A","BatteryStatus":1},{"Name":"B","BatteryStatus":1}],"static":[],"full":[],"cycles":[]}`
	if batteries, _ := batteriesFromWindowsJSON([]byte(two)); len(batteries) != 2 {
		t.Error("a machine with two batteries yields two cards")
	}
}

func TestVerdictFor(t *testing.T) {
	tests := []struct {
		name          string
		health, cycle int
		want          string
	}{
		{"unknown", 0, 0, "This computer did not report the battery's original capacity, so its health cannot be worked out."},
		{"negative is unknown too", -5, 0, "This computer did not report the battery's original capacity, so its health cannot be worked out."},
		{"above 100", 102, 23, "This battery still holds essentially all of its original charge."},
		{"at 95", 95, 0, "This battery still holds essentially all of its original charge."},
		{"just below 95", 94, 0, "Normal wear for a battery of this age. There is plenty of life left in it."},
		{"at 80", 80, 0, "Normal wear for a battery of this age. There is plenty of life left in it."},
		{"just below 80", 79, 0, "Noticeably worn. It will not last as long between charges as it did when new."},
		{"at 60", 60, 0, "Noticeably worn. It will not last as long between charges as it did when new."},
		{"just below 60", 59, 0, "Worn out. Worth replacing if this person works away from a desk."},
		{"at 40", 40, 0, "Worn out. Worth replacing if this person works away from a desk."},
		{"just below 40", 39, 0, "Nearly dead. Replace it."},
		{"at 1", 1, 0, "Nearly dead. Replace it."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := verdictFor(tt.health, tt.cycle); got != tt.want {
				t.Errorf("verdictFor(%d,%d)\n got %q\nwant %q", tt.health, tt.cycle, got, tt.want)
			}
		})
	}
}

func TestVerdictAppendsTheCycleWarning(t *testing.T) {
	// A high cycle count with good health is the one case the health figure alone
	// misses.
	const tail = " It has done 1,234 charge cycles, which is past what most laptop batteries are rated for, so expect it to fall away soon."

	got := verdictFor(85, 1234)
	if !strings.HasSuffix(got, tail) {
		t.Errorf("the warning was not appended:\n%q", got)
	}

	// Exactly at the threshold it fires.
	if !strings.Contains(verdictFor(85, 1000), "1,000 charge cycles") {
		t.Error("1000 cycles must fire the warning")
	}
	// One below it does not.
	if strings.Contains(verdictFor(85, 999), "charge cycles") {
		t.Error("999 cycles must not fire the warning")
	}
	// And below 80% health the verdict is already blunt enough.
	if strings.Contains(verdictFor(79, 2000), "charge cycles") {
		t.Error("below 80% health the extra sentence must not appear")
	}
}

func TestThousands(t *testing.T) {
	tests := map[int]string{0: "0", 23: "23", 999: "999", 1000: "1,000", 1234: "1,234", 12345: "12,345"}
	for in, want := range tests {
		if got := thousands(in); got != want {
			t.Errorf("thousands(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestNoteFor(t *testing.T) {
	tests := []struct {
		name      string
		os        string
		batteries int
		health    bool
		want      string
	}{
		{"a desktop", "linux", 0, false, noteNoBattery},
		{"a windows desktop", "windows", 0, false, noteNoBattery},
		{"linux with a battery", "linux", 1, true, noteLinux},
		{"darwin", "darwin", 1, true, noteDarwin},
		{"windows with health", "windows", 1, true, noteWindows},
		{"windows without health", "windows", 1, false, noteWindowsNo},
		{"anything else", "freebsd", 0, false, noteOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noteFor(tt.os, tt.batteries, tt.health); got != tt.want {
				t.Errorf("note\n got %q\nwant %q", got, tt.want)
			}
		})
	}

	// The note is always shown, so every one must be a real sentence.
	for _, note := range []string{noteNoBattery, noteLinux, noteDarwin, noteWindows, noteWindowsNo, noteOther} {
		if len(note) < 40 || !strings.HasSuffix(note, ".") {
			t.Errorf("note is not a sentence: %q", note)
		}
	}
	// And the two that explain a figure above 100 must say so.
	for _, note := range []string{noteLinux, noteDarwin, noteWindows} {
		if !strings.Contains(note, "above 100% is normal") {
			t.Errorf("note does not explain a health figure above 100: %q", note)
		}
	}
	if !strings.Contains(noteWindowsNo, "powercfg /batteryreport") {
		t.Error("the Windows fallback note must name powercfg")
	}
}

func TestReportSlicesAreNeverNil(t *testing.T) {
	data, err := json.Marshal(Report{Batteries: []Battery{}, Unsupported: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"batteries":[]`) {
		t.Errorf("batteries marshalled as null: %s", data)
	}
	if !strings.Contains(string(data), `"unsupported":[]`) {
		t.Errorf("unsupported marshalled as null: %s", data)
	}
}

func TestListFillsEveryVerdictAndTheNote(t *testing.T) {
	r, err := List()
	if err != nil {
		t.Fatalf("List failed on this machine: %v", err)
	}
	if r.Batteries == nil || r.Unsupported == nil {
		t.Fatal("both slices must be initialised so neither marshals to null")
	}
	if r.Note == "" {
		t.Fatal("the note is never allowed to be empty: the page always shows it")
	}
	for _, b := range r.Batteries {
		if b.Verdict == "" {
			t.Errorf("battery %q has no verdict", b.Name)
		}
	}
}

func TestNoWrites(t *testing.T) {
	// This tool reads and nothing else: no power setting is changed.
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{"os.Remove", "os.WriteFile", "os.Create", "Set-", "New-", "Remove-", "Invoke-"}
	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range banned {
			if strings.Contains(string(src), call) {
				t.Errorf("%s contains %q; this tool must only read", name, call)
			}
		}
	}
}
