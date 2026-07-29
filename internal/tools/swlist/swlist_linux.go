//go:build linux

package swlist

import (
	"context"
	"os/exec"
	"time"
)

// managers are the package managers CHIT knows, in the order it asks. A missing
// binary means that distro's manager is not this distro's, so it is skipped
// silently rather than reported as a failure.
//
// snap is deliberately absent: "snap list" prints a human table with no stable
// machine-readable form and no --format flag, so parsing it means guessing at
// column widths. That is named in the spec rather than half built.
var managers = []struct {
	source  string
	command string
	args    []string
	parse   func(string) []Program
}{
	{SourcePacman, "pacman", []string{"-Qi"}, parsePacman},
	{SourceDpkg, "dpkg-query", []string{"-W", `-f=${Package}\t${Version}\t${Maintainer}\t${Installed-Size}\n`}, parseDpkg},
	{SourceRPM, "rpm", []string{"-qa", "--qf", `%{NAME}\t%{VERSION}-%{RELEASE}\t%{VENDOR}\t%{SIZE}\t%{INSTALLTIME}\n`}, parseRPM},
	{SourceFlatpak, "flatpak", []string{"list", "--columns=application,version,origin"}, parseFlatpak},
}

// collect runs whichever managers exist and merges what they say.
func collect(r *Report) {
	for _, manager := range managers {
		out, err := run(manager.command, manager.args...)
		if err != nil {
			continue
		}
		found := manager.parse(out)
		if len(found) == 0 {
			continue
		}
		r.addSource(manager.source)
		r.Programs = append(r.Programs, found...)
	}
	r.Note = noteFor("linux", r.Sources)
}

// run executes a package manager and returns its stdout. The argument lists are
// compile-time constants; no user input ever reaches here.
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
