// Package swlist lists what is installed on this machine, with versions, so an
// intake record or a "what version of the VPN client is on this box?" question
// can be answered without screenshotting Add or Remove Programs.
//
// Everything here is read only: nothing is installed, updated or removed.
package swlist

import (
	"runtime"
	"strings"

	"chit/internal/core"
)

// Ids used in Report.Unsupported.
const (
	FieldInstalledOn = "installedOn"
	FieldSize        = "size"
	FieldPublisher   = "publisher"
)

// Source labels, in words a tech would recognise rather than a command name.
const (
	SourceWindowsAll   = "Windows (all users)"
	SourceWindows32    = "Windows (all users, 32-bit)"
	SourceWindowsUser  = "Windows (this user)"
	SourcePacman       = "pacman"
	SourceDpkg         = "dpkg"
	SourceRPM          = "rpm"
	SourceFlatpak      = "flatpak"
	SourceApplications = "Applications"
)

// commandTimeout caps the Linux package managers. system_profiler is genuinely
// slow and gets its own, longer, budget.
const (
	commandTimeout         = 20 // seconds
	profilerCommandTimeout = 30 // seconds
)

// Program is one installed thing.
type Program struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Publisher string `json:"publisher"`
	// InstalledOn is an RFC3339 date (no time) or "" when this source does not
	// record one.
	InstalledOn string `json:"installedOn"`
	// SizeBytes is zero when this source does not record a size.
	SizeBytes int64 `json:"sizeBytes"`
	// Source is where CHIT read it.
	Source string `json:"source"`
}

type Report struct {
	OS       string    `json:"os"`
	Programs []Program `json:"programs"`
	// Sources lists what was actually read, in the order it was read, so the
	// count line and the filter chips can name them.
	Sources     []string `json:"sources"`
	Unsupported []string `json:"unsupported"`
	// Note always has a sentence in it.
	Note string `json:"note"`
}

// List reads what this operating system knows is installed.
func List() (Report, error) {
	r := Report{
		OS: runtime.GOOS,
		// All three slices are initialised so none marshals as null.
		Programs:    []Program{},
		Sources:     []string{},
		Unsupported: []string{},
	}

	collect(&r)

	r.Programs = dedupe(r.Programs)

	dates, sizes := false, false
	for _, p := range r.Programs {
		if p.InstalledOn != "" {
			dates = true
		}
		if p.SizeBytes > 0 {
			sizes = true
		}
	}
	if len(r.Programs) > 0 {
		if !dates {
			r.markUnsupported(FieldInstalledOn)
		}
		if !sizes {
			r.markUnsupported(FieldSize)
		}
	}

	if r.Note == "" {
		return r, core.Errorf(core.CodeInternal,
			"Could not read this machine's installed software. Try Refresh, and if it keeps happening this computer is refusing something CHIT normally gets without admin rights.")
	}
	return r, nil
}

// dedupe collapses the same name and version seen twice, keeping the source it
// was read from first. Windows genuinely returns the same entry from the 64-bit
// and 32-bit views of one registry key on some machines.
func dedupe(programs []Program) []Program {
	seen := map[string]bool{}
	out := make([]Program, 0, len(programs))
	for _, p := range programs {
		key := p.Name + "\x00" + p.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// addSource records a source that produced at least one row, once.
func (r *Report) addSource(name string) {
	for _, existing := range r.Sources {
		if existing == name {
			return
		}
	}
	r.Sources = append(r.Sources, name)
}

// markUnsupported records a field no source on this machine reported, once.
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

// The per-OS notes, in one place so a test can pin every branch.
const (
	noteWindows   = "This is what Add or Remove Programs shows, read straight from the registry. Windows updates and components of larger products are left out on purpose. Apps from the Microsoft Store are not in this list: Windows keeps those somewhere else."
	noteLinux     = "This is what this machine's package manager knows about. Anything installed by unpacking a tarball, by a script, or into a home directory is not in the list, because nothing recorded it."
	noteLinuxNone = "CHIT could not find a package manager on this machine. It looks for pacman, dpkg, rpm and flatpak."
	noteDarwin    = "This lists the applications macOS knows about, which is not the same as everything installed: command-line tools, Homebrew packages and drivers are not applications and do not appear."
	noteOther     = "CHIT does not know how to list installed software on this operating system."

	failWindows  = " This computer would not let CHIT read the software list out of the registry, which is unusual and normally means the machine is locked down."
	failProfiler = " macOS did not answer within thirty seconds, so no applications are listed. Try Refresh."
)

// noteFor picks the sentence for this operating system and what it managed to
// read.
func noteFor(os string, sources []string) string {
	switch os {
	case "windows":
		return noteWindows
	case "linux":
		if len(sources) == 0 {
			return noteLinuxNone
		}
		return noteLinux
	case "darwin":
		return noteDarwin
	}
	return noteOther
}

// SourceList writes the sources as a sentence fragment: "pacman and flatpak".
// The page and the note both need it, so it lives in one place.
func SourceList(sources []string) string {
	switch len(sources) {
	case 0:
		return ""
	case 1:
		return sources[0]
	case 2:
		return sources[0] + " and " + sources[1]
	}
	return strings.Join(sources[:len(sources)-1], ", ") + " and " + sources[len(sources)-1]
}
