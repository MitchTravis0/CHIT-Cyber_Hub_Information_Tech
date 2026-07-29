//go:build linux

package wifi

import (
	"context"
	"os/exec"
	"time"
)

// collect asks iw. /proc/net/wireless is deliberately not used, not even as a
// fallback: on this machine it lists no interfaces at all despite wlan0 being
// associated, which makes it a source that silently reports nothing, and a
// fallback that returns an empty list is indistinguishable from a real absence.
//
// nmcli is not used as a second source either: it reports NetworkManager's view,
// which is absent on a machine using systemd-networkd or wpa_supplicant
// directly, so it would swap one gap for another while doubling the parsers.
func collect(r *Report) {
	// iw does not report which security the connection is using, and guessing
	// would be worse than the gap.
	r.markUnsupported(FieldSecurity)

	out, err := run("iw", "dev")
	if err != nil {
		r.Note = noteNoIw
		return
	}

	for _, name := range parseIwDev(out) {
		// name comes from iw's own output and is passed as its own argument, so
		// nothing is interpolated into a shell string.
		linkOut, err := run("iw", "dev", name, "link")
		if err != nil {
			continue
		}
		link := parseIwLink(linkOut)
		link.Interface = name

		if link.Connected {
			if infoOut, err := run("iw", "dev", name, "info"); err == nil {
				channel, mhz, width := parseIwInfo(infoOut)
				link.Channel = channel
				link.WidthMHz = width
				if link.FrequencyMHz == 0 {
					link.FrequencyMHz = mhz
				}
			}
		}
		r.Links = append(r.Links, link)
	}

	r.Note = noteFor("linux", len(r.Links), true)
}

// run executes a system tool and returns its stdout. Every caller treats a
// failure as "this operating system did not tell us".
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
