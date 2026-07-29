//go:build darwin

package netstat

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// collect prefers lsof, which is the only unprivileged way to learn which
// program owns a socket on macOS, and falls back to netstat, which knows the
// sockets and nothing about the programs.
//
// Both argument lists are compile-time constants. No user input reaches either.
func collect(r *Report) {
	if out, err := run("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP", "-FpcfnPT"); err == nil {
		hidden := fromLsof(r, out)
		if len(r.Entries) > 0 {
			r.Note = noteFor("darwin", true, hidden)
			return
		}
	}

	found := false
	for _, protocol := range []string{"tcp", "udp"} {
		out, err := run("netstat", "-an", "-p", protocol)
		if err != nil {
			continue
		}
		found = true
		for _, line := range strings.Split(out, "\n") {
			if entry, ok := parseNetstatLine(line); ok {
				r.Entries = append(r.Entries, entry)
			}
		}
	}

	if !found {
		r.Note = noteFor("darwin", false, 0) + failDarwin
		return
	}
	r.Note = noteFor("darwin", false, 0)
}

// fromLsof turns lsof's field output into entries and returns how many sockets
// came back with no program name.
func fromLsof(r *Report, out string) int {
	hidden := 0
	for _, file := range parseLsofFields(out) {
		protocol := ""
		switch file.Protocol {
		case "TCP":
			// lsof was asked for listeners only, but a UDP selector widens the
			// query, so the state is checked rather than trusted.
			if file.State != "LISTEN" {
				continue
			}
			protocol = "tcp"
		case "UDP":
			protocol = "udp"
		default:
			continue
		}

		v6 := strings.Contains(file.Name, "[")
		address, port, ok := splitLsofName(file.Name, v6)
		if !ok {
			continue
		}
		if v6 {
			protocol += "6"
		}
		if file.Command == "" {
			hidden++
		}
		r.Entries = append(r.Entries, Entry{
			Protocol: protocol,
			Address:  address,
			Port:     port,
			PID:      file.PID,
			Process:  file.Command,
			Source:   "lsof",
		})
	}
	return hidden
}

// run executes a system tool and returns its stdout. Every caller treats a
// failure as "this operating system did not tell us", so the error detail is not
// needed.
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
