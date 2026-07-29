//go:build windows

package battery

import (
	"context"
	"os/exec"
	"time"
)

// cimQuery is the one PowerShell command this package runs. Win32_Battery gives
// the charge and the state to any user; the three root\wmi classes give the
// health and the cycle count on most machines and are refused on some, which is
// why every one of them is SilentlyContinue and optional.
//
// It is a compile-time constant. No user input reaches it, and nothing in it
// sets, creates, removes or invokes anything.
const cimQuery = `$b=Get-CimInstance -ClassName Win32_Battery -ErrorAction SilentlyContinue;` +
	`$s=Get-CimInstance -Namespace root\wmi -ClassName BatteryStaticData -ErrorAction SilentlyContinue;` +
	`$f=Get-CimInstance -Namespace root\wmi -ClassName BatteryFullChargedCapacity -ErrorAction SilentlyContinue;` +
	`$c=Get-CimInstance -Namespace root\wmi -ClassName BatteryCycleCount -ErrorAction SilentlyContinue;` +
	`ConvertTo-Json -Depth 3 -Compress @{battery=@($b);static=@($s);full=@($f);cycles=@($c)}`

// collect asks Windows once and reads the answer defensively.
func collect(r *Report) {
	ctx, cancel := context.WithTimeout(context.Background(), powershellCommandTimeout*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "powershell",
		"-NoProfile", "-NonInteractive", "-Command", cimQuery).Output()
	if err != nil {
		r.Note = noteFor("windows", 0, false) + " " + failPowerShell
		return
	}

	batteries, health := batteriesFromWindowsJSON(out)
	r.Batteries = append(r.Batteries, batteries...)

	// Win32_Battery reports mains power only indirectly, so CHIT does not claim
	// to know. The page hides the line rather than guessing.
	r.HasAC = false

	r.Note = noteFor("windows", len(r.Batteries), health)
}
