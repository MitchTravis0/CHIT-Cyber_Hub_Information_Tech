// Package netstat lists what is listening on this machine, on which address and
// port, and which program owns it. It is the direct answer to "address already
// in use" and to "what is serving on 8080?".
//
// Everything here is read only: no port is closed, no program is stopped, and
// nothing needs administrator rights. The price of that last promise is that on
// Linux and macOS a normal user only sees the names of their own processes, and
// the rest are labelled rather than left blank.
package netstat

import (
	"net/netip"
	"runtime"
	"sort"
	"strings"

	"chit/internal/core"
)

// Reach values, computed from the bound address.
const (
	ReachEverywhere = "everywhere"
	ReachLocal      = "local"
	ReachOne        = "one"
)

// Ids used in Report.Unsupported.
const (
	FieldProcess = "process"
	FieldUDP     = "udp"
)

// commandTimeout caps lsof and netstat on macOS, which are the only external
// commands this package runs.
const commandTimeout = 10 // seconds

// Entry is one listening socket.
type Entry struct {
	// Protocol is "tcp", "tcp6", "udp" or "udp6".
	Protocol string `json:"protocol"`
	// Address is the local address it is bound to, as text.
	Address string `json:"address"`
	Port    int    `json:"port"`
	// Reach is one of the three constants above.
	Reach string `json:"reach"`
	// PID is zero when this operating system did not say.
	PID int `json:"pid"`
	// Process is empty when the name is not visible to a normal user. The page
	// shows a label in that case rather than a blank cell.
	Process string `json:"process"`
	// Service is the well-known name for the port, or "" when there is none.
	Service string `json:"service"`
	// Source is where CHIT read it.
	Source string `json:"source"`
}

type Report struct {
	OS      string  `json:"os"`
	Entries []Entry `json:"entries"`
	// ProcessNames is true when this operating system gave a name for at least
	// one row. False means the whole column is empty and the page says why.
	ProcessNames bool     `json:"processNames"`
	Unsupported  []string `json:"unsupported"`
	// Note always has a sentence in it.
	Note string `json:"note"`
}

// List reads every listening socket this operating system will report.
func List() (Report, error) {
	r := Report{
		OS: runtime.GOOS,
		// Both slices are initialised so they marshal as [] rather than null.
		Entries:     []Entry{},
		Unsupported: []string{},
	}

	collect(&r)

	for i := range r.Entries {
		if r.Entries[i].Service == "" {
			r.Entries[i].Service = ServiceName(r.Entries[i].Port)
		}
		if r.Entries[i].Reach == "" {
			r.Entries[i].Reach = ReachFor(r.Entries[i].Address)
		}
		if r.Entries[i].Process != "" {
			r.ProcessNames = true
		}
	}
	if !r.ProcessNames {
		r.markUnsupported(FieldProcess)
	}
	SortEntries(r.Entries)

	if r.Note == "" {
		return r, core.Errorf(core.CodeInternal,
			"Could not read this machine's listening ports. Try Refresh, and if it keeps happening this computer is refusing something CHIT normally gets without admin rights.")
	}
	return r, nil
}

// ReachFor says how far a bound address can be reached from. An address that
// cannot be parsed is reported as "one address", which is the safe direction:
// claiming "local only" for something unreadable would understate the exposure.
func ReachFor(address string) string {
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return ReachOne
	}
	switch {
	case addr.IsUnspecified():
		return ReachEverywhere
	case addr.IsLoopback():
		return ReachLocal
	}
	return ReachOne
}

// SortEntries puts the list in the order a tech scans it: by port, because they
// arrive knowing the number.
func SortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		if familyRank(a.Protocol) != familyRank(b.Protocol) {
			return familyRank(a.Protocol) < familyRank(b.Protocol)
		}
		return a.Address < b.Address
	})
}

// familyRank orders TCP before UDP and IPv4 before IPv6 within each.
func familyRank(protocol string) int {
	switch protocol {
	case "tcp":
		return 0
	case "tcp6":
		return 1
	case "udp":
		return 2
	case "udp6":
		return 3
	}
	return 4
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

// The per-OS notes, in one place so a test can pin every branch. A tech who does
// not read the note will take a missing program name for a broken tool.
const (
	noteLinuxHidden = "Linux only shows the program behind a port to the user who started it, so the rows without a name belong to another user or to the system. Nothing here needs administrator rights, and CHIT never asks for them."
	noteLinuxAll    = "Every listening socket on this machine is shown. A port bound to 127.0.0.1 can only be reached from this computer."
	noteWindows     = "Windows reports the owning program for every listening port without administrator rights, so this list is complete. A port bound to 0.0.0.0 is reachable from other machines on the network."
	noteDarwinLsof  = "macOS only shows the program behind a port to the user who started it, so the rows without a name belong to another user or to the system."
	noteDarwinPlain = "This Mac does not have lsof, so the ports are listed without the program that owns them."
	noteOther       = "CHIT does not know how to list listening ports on this operating system."

	failProc    = " This machine has no readable /proc/net, so nothing could be listed. That happens inside some containers."
	failWindows = " Windows would not answer the request for the port list, which is unusual and normally means the machine is locked down."
	failDarwin  = " Neither lsof nor netstat is on this Mac, so nothing could be listed."
)

// noteFor picks the sentence for this operating system and what it managed to
// read. hidden counts sockets whose owning process could not be named.
func noteFor(os string, processNames bool, hidden int) string {
	switch os {
	case "linux":
		if hidden > 0 {
			return noteLinuxHidden
		}
		return noteLinuxAll
	case "windows":
		return noteWindows
	case "darwin":
		if processNames {
			return noteDarwinLsof
		}
		return noteDarwinPlain
	}
	return noteOther
}

// ServiceName is the well-known name for a port, or "" when there is none.
func ServiceName(port int) string {
	return services[port]
}

// hostPort splits an address that a per-OS reader has already normalised, so
// every collector produces the same shape.
func trimBrackets(address string) string {
	return strings.TrimSuffix(strings.TrimPrefix(address, "["), "]")
}
