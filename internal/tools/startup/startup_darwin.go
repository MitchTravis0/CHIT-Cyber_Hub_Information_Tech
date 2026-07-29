//go:build darwin

package startup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// collect reads the three standard launchd folders and asks launchctl what is
// loaded. Nothing here needs administrator rights: the daemon folder is world
// readable even though only root may change it.
func collect(r *Report) {
	running := map[string]bool{}
	if out, err := run("launchctl", "list"); err == nil {
		running = parseLaunchctlList(out)
	} else {
		r.markUnsupported(FieldState)
		r.addNote("CHIT could not ask launchd what is running, so the \"Now\" column is empty.")
	}

	binary := 0
	home, _ := os.UserHomeDir()
	folders := []struct{ dir, source string }{
		{filepath.Join(home, "Library", "LaunchAgents"), "LaunchAgents (user)"},
		{"/Library/LaunchAgents", "LaunchAgents"},
		{"/Library/LaunchDaemons", "LaunchDaemons"},
	}

	for _, folder := range folders {
		if folder.dir == "" {
			continue
		}
		entries, err := os.ReadDir(folder.dir)
		if err != nil {
			continue
		}
		for _, file := range entries {
			if file.IsDir() || filepath.Ext(file.Name()) != ".plist" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(folder.dir, file.Name()))
			if err != nil {
				continue
			}
			job, ok := parsePlist(string(data))
			if !ok {
				binary++
				continue
			}

			name := job.Label
			if name == "" {
				name = file.Name()
			}
			mode := StartManual
			if job.RunAtLoad {
				mode = StartAutomatic
			}
			if job.Disabled {
				mode = StartDisabled
			}
			item := Item{
				Name:      name,
				Kind:      KindStartup,
				Source:    folder.source,
				Command:   job.Program,
				StartMode: mode,
				Enabled:   !job.Disabled,
			}
			if loaded, known := running[job.Label]; known {
				item.State = StateStopped
				if loaded {
					item.State = StateRunning
				}
			}
			r.Items = append(r.Items, item)
		}
	}

	if binary > 0 {
		r.addNote(count(binary, "startup file is", "startup files are") +
			" in Apple's binary format, which CHIT cannot read, so they are not in the list.")
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
