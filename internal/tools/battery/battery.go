// Package battery reports how much of its original charge a laptop's battery
// still holds, which is the one number that settles "my laptop dies after an
// hour". Everything here is read only: no power setting is changed and no
// charge limit is applied.
package battery

import (
	"runtime"
	"strconv"
	"strings"

	"chit/internal/core"
)

// States.
const (
	StateCharging    = "charging"
	StateDischarging = "discharging"
	StateFull        = "full"
	StateUnknown     = "unknown"
)

// Ids used in Report.Unsupported.
const (
	FieldHealth = "health"
	FieldCycles = "cycles"
	FieldSerial = "serial"
	FieldOnAC   = "onAc"
)

// commandTimeout caps ioreg. PowerShell gets its own, longer, budget.
const (
	commandTimeout           = 10 // seconds
	powershellCommandTimeout = 30 // seconds
)

// Battery is one battery.
type Battery struct {
	Name string `json:"name"`
	// State is one of the four constants above.
	State string `json:"state"`
	// ChargePercent is how full it is now, 0 to 100, or -1 when unknown.
	ChargePercent int `json:"chargePercent"`
	// DesignWh and FullWh are watt-hours, or 0 when this OS did not say.
	DesignWh float64 `json:"designWh"`
	FullWh   float64 `json:"fullWh"`
	// HealthPercent is FullWh as a percentage of DesignWh, or 0 when either is
	// missing. It is NOT capped at 100: a battery that measures slightly above
	// its design capacity is real, and capping it would be fabricating a value.
	HealthPercent int `json:"healthPercent"`
	// CycleCount is 0 when this OS did not say.
	CycleCount   int    `json:"cycleCount"`
	Technology   string `json:"technology"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	// Verdict is the plain sentence, never empty.
	Verdict string `json:"verdict"`
	// Source is where CHIT read it.
	Source string `json:"source"`
}

type Report struct {
	OS        string    `json:"os"`
	Batteries []Battery `json:"batteries"`
	// OnAC is true when this machine is plugged in. HasAC is false when this OS
	// would not say, so the page can tell "on battery" from "did not answer".
	OnAC  bool `json:"onAc"`
	HasAC bool `json:"hasAc"`
	// Unsupported lists field ids this operating system would not report.
	Unsupported []string `json:"unsupported"`
	// Note always has a sentence in it.
	Note string `json:"note"`
}

// List reads whatever this operating system will say about the battery.
func List() (Report, error) {
	r := Report{
		OS: runtime.GOOS,
		// Both slices are initialised so they marshal as [] rather than null.
		Batteries:   []Battery{},
		Unsupported: []string{},
	}

	collect(&r)

	health, cycles := false, false
	for i := range r.Batteries {
		b := &r.Batteries[i]
		if b.HealthPercent > 0 {
			health = true
		} else {
			b.HealthPercent = 0
		}
		if b.CycleCount > 0 {
			cycles = true
		}
		b.Verdict = verdictFor(b.HealthPercent, b.CycleCount)
	}
	if len(r.Batteries) > 0 {
		if !health {
			r.markUnsupported(FieldHealth)
		}
		if !cycles {
			r.markUnsupported(FieldCycles)
		}
	}
	if !r.HasAC {
		r.markUnsupported(FieldOnAC)
	}

	if r.Note == "" {
		return r, core.Errorf(core.CodeInternal,
			"Could not read this machine's battery details. Try Refresh, and if it keeps happening this computer is refusing something CHIT normally gets without admin rights.")
	}
	return r, nil
}

// verdictFor is the plain sentence beside the health figure. It is never empty:
// a card with a number and no reading makes a junior guess.
func verdictFor(healthPercent, cycleCount int) string {
	var verdict string
	switch {
	case healthPercent <= 0:
		return "This computer did not report the battery's original capacity, so its health cannot be worked out."
	case healthPercent >= 95:
		verdict = "This battery still holds essentially all of its original charge."
	case healthPercent >= 80:
		verdict = "Normal wear for a battery of this age. There is plenty of life left in it."
	case healthPercent >= 60:
		return "Noticeably worn. It will not last as long between charges as it did when new."
	case healthPercent >= 40:
		return "Worn out. Worth replacing if this person works away from a desk."
	default:
		return "Nearly dead. Replace it."
	}

	// A high cycle count with good health is the one case the health figure
	// alone misses: the cells are about to fall away whatever the number says.
	if cycleCount >= 1000 {
		verdict += " It has done " + thousands(cycleCount) +
			" charge cycles, which is past what most laptop batteries are rated for, so expect it to fall away soon."
	}
	return verdict
}

// thousands groups a count so a four-figure number reads as one.
func thousands(n int) string {
	text := strconv.Itoa(n)
	if n < 0 {
		return text
	}
	var out strings.Builder
	for i, digit := range text {
		if i > 0 && (len(text)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(digit)
	}
	return out.String()
}

// The per-OS notes, in one place so a test can pin every branch.
const (
	noteNoBattery = "This machine has no battery, which is normal for a desktop."
	noteLinux     = "These figures come straight from the kernel and need no special rights. A health figure slightly above 100% is normal: the battery is measured against the capacity printed on it, not against what it actually held when new."
	noteDarwin    = "These figures come from the system's own battery controller. A health figure slightly above 100% is normal."
	noteWindows   = "These figures come from the same place powercfg /batteryreport uses. A health figure slightly above 100% is normal."
	noteWindowsNo = "This computer would not report the battery's original capacity to an ordinary user, so its health could not be worked out. Running powercfg /batteryreport from a command prompt writes a full report to a file and does not need administrator rights either."
	noteOther     = "CHIT does not know how to read battery details on this operating system."

	failPowerShell = "Windows did not answer within thirty seconds, so no battery details could be read. Try Refresh."
	failIoreg      = "macOS did not answer, so no battery details could be read."
)

// noteFor picks the sentence for this operating system and what it managed to
// read. A tech who does not read the note will take a missing health figure for
// a broken tool.
func noteFor(os string, batteries int, health bool) string {
	if batteries == 0 {
		if os == "windows" || os == "linux" || os == "darwin" {
			return noteNoBattery
		}
		return noteOther
	}
	switch os {
	case "linux":
		return noteLinux
	case "darwin":
		return noteDarwin
	case "windows":
		if health {
			return noteWindows
		}
		return noteWindowsNo
	}
	return noteOther
}

// markUnsupported records a field this operating system would not report, once.
func (r *Report) markUnsupported(fields ...string) {
	for _, f := range fields {
		found := false
		for _, existing := range r.Unsupported {
			if existing == f {
				found = true
				break
			}
		}
		if !found {
			r.Unsupported = append(r.Unsupported, f)
		}
	}
}
