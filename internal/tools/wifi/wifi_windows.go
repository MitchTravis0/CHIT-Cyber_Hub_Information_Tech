//go:build windows

package wifi

import (
	"context"
	"os/exec"
	"time"
)

// collect asks netsh. Its argument list is a compile-time constant and no user
// input reaches it.
//
// netsh's output is localised: on a German or French Windows every key name is
// translated and none of them matches, so the parser returns no links rather
// than wrong ones, and the note names this case explicitly. The alternative is
// the wlanapi COM surface, which is a much larger piece of Windows code nobody
// here can execute.
func collect(r *Report) {
	// netsh reports the signal as a percentage and never in dBm, and it does not
	// report the channel width at all.
	r.markUnsupported(FieldSignalDBm, FieldWidth)

	out, err := run("netsh", "wlan", "show", "interfaces")
	if err != nil {
		r.Note = noteFor("windows", 0, false)
		return
	}

	r.Links = append(r.Links, parseNetshInterfaces(out)...)
	r.Note = noteFor("windows", len(r.Links), true)
}

func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
