//go:build darwin

package battery

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// collect runs one ioreg command. Its argument list is a compile-time constant
// and no user input reaches it.
func collect(r *Report) {
	out, err := run("ioreg", "-r", "-c", "AppleSmartBattery")
	if err != nil {
		r.Note = noteFor("darwin", 0, false) + " " + failIoreg
		return
	}

	pairs := parseIoregPairs(out)
	b, ok := batteryFromIoreg(pairs)
	if ok && (b.HealthPercent > 0 || b.ChargePercent >= 0) {
		r.Batteries = append(r.Batteries, b)
	}

	if external, present := pairs["ExternalConnected"]; present {
		r.HasAC = true
		r.OnAC = strings.EqualFold(strings.TrimSpace(external), "Yes")
	}

	health := len(r.Batteries) > 0 && r.Batteries[0].HealthPercent > 0
	r.Note = noteFor("darwin", len(r.Batteries), health)
}

// run executes a system tool and returns its stdout. Every caller treats a
// failure as "this operating system did not tell us".
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
