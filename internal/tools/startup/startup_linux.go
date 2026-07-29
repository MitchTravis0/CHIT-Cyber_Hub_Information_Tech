//go:build linux

package startup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// collect reads systemd's service list and the XDG autostart folders. Nothing
// here needs root: systemctl reports the configuration to any user.
func collect(r *Report) {
	collectSystemd(r, false, "systemd")
	collectSystemd(r, true, "systemd (user)")
	collectAutostart(r, xdgUserAutostart(), "~/.config/autostart")
	collectAutostart(r, "/etc/xdg/autostart", "/etc/xdg/autostart")
}

func collectSystemd(r *Report, user bool, source string) {
	args := []string{"list-unit-files", "--type=service", "--no-pager", "--no-legend", "--plain"}
	unitArgs := []string{"list-units", "--type=service", "--no-pager", "--no-legend", "--plain", "--all"}
	if user {
		args = append([]string{"--user"}, args...)
		unitArgs = append([]string{"--user"}, unitArgs...)
	}

	out, err := run("systemctl", args...)
	if err != nil {
		// Only the system bus failing is worth a sentence: a machine with no
		// user session simply has no user units, which is not a problem.
		if !user {
			r.markUnsupported(FieldServices)
			r.addNote("This machine does not use systemd, so no services are listed. The autostart entries below are still complete.")
		}
		return
	}

	states := map[string]Unit{}
	if unitsOut, err := run("systemctl", unitArgs...); err == nil {
		for _, unit := range parseUnits(unitsOut) {
			states[unit.Name] = unit
		}
	}

	for _, file := range parseUnitFiles(out) {
		mode := unitStartMode(file.State)
		item := Item{
			Name:      file.Name,
			Kind:      KindService,
			Source:    source,
			StartMode: mode,
			Enabled:   mode != StartDisabled && mode != "",
		}
		if unit, ok := states[file.Name]; ok {
			item.State = unitState(unit.Active)
		}
		r.Items = append(r.Items, item)
	}
}

func xdgUserAutostart() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "autostart")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "autostart")
}

func collectAutostart(r *Report, dir, source string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, file := range entries {
		if file.IsDir() || filepath.Ext(file.Name()) != ".desktop" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			continue
		}
		entry := parseDesktopEntry(string(data), file.Name())
		mode := StartAutomatic
		if !entry.Enabled {
			mode = StartDisabled
		}
		r.Items = append(r.Items, Item{
			Name:      entry.Name,
			Kind:      KindStartup,
			Source:    source,
			Command:   entry.Exec,
			StartMode: mode,
			Enabled:   entry.Enabled,
		})
	}
}

// run executes a system tool and returns its stdout. Every caller treats a
// failure as "this operating system did not tell us". The argument lists are
// compile-time constants; no user input ever reaches here.
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
