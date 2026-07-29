// Package usbhist lists the USB devices attached to this machine, and on
// Windows the ones the operating system remembers seeing before. Everything
// here is read only: nothing is ejected, removed or changed.
package usbhist

import (
	"runtime"
	"sort"
	"strings"

	"chit/internal/core"
)

// Kinds. Best effort from the device class or the registry key it came from.
const (
	KindStorage = "storage"
	KindInput   = "input"
	KindAudio   = "audio"
	KindVideo   = "video"
	KindNetwork = "network"
	KindPrinter = "printer"
	KindHub     = "hub"
	KindOther   = "other"
)

// Ids used in Report.Unsupported.
const (
	FieldHistory   = "history"
	FieldFirstSeen = "firstSeen"
	FieldSerial    = "serial"
)

// commandTimeout caps system_profiler, which routinely takes several seconds.
const commandTimeout = 10 // seconds

// Device is one USB device.
type Device struct {
	Name         string `json:"name"`
	Manufacturer string `json:"manufacturer"`
	// VendorID and ProductID are four lowercase hex digits, or "" when the
	// operating system did not say.
	VendorID  string `json:"vendorId"`
	ProductID string `json:"productId"`
	Serial    string `json:"serial"`
	Kind      string `json:"kind"`
	Connected bool   `json:"connected"`
	// FirstSeen is RFC3339, or "" when this OS does not record it.
	FirstSeen string `json:"firstSeen"`
	Source    string `json:"source"`
}

type Report struct {
	OS      string   `json:"os"`
	Devices []Device `json:"devices"`
	// History is true when this operating system records devices that are not
	// connected now. The page hides the "Seen before" filter when it is false.
	History     bool     `json:"history"`
	Unsupported []string `json:"unsupported"`
	// Note always has a sentence in it, explaining what this OS records. A tech
	// who does not read it will take an empty history to mean nothing was ever
	// plugged in.
	Note string `json:"note"`
}

// List reads every USB device this operating system will report.
func List() (Report, error) {
	r := Report{
		OS: runtime.GOOS,
		// Both slices are initialised so they marshal as [] rather than null.
		Devices:     []Device{},
		Unsupported: []string{},
	}

	collect(&r)
	sortDevices(r.Devices)

	if r.Note == "" {
		return r, core.Errorf(core.CodeInternal,
			"Could not read this machine's USB devices. Try Refresh, and if it keeps happening this computer is refusing something CHIT normally gets without admin rights.")
	}
	return r, nil
}

// kindOrder is the order a tech scans the list in: storage first, because that
// is what the question is usually about.
var kindOrder = map[string]int{
	KindStorage: 0, KindInput: 1, KindAudio: 2, KindVideo: 3,
	KindNetwork: 4, KindPrinter: 5, KindHub: 6, KindOther: 7,
}

func sortDevices(devices []Device) {
	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].Connected != devices[j].Connected {
			return devices[i].Connected
		}
		ki, kj := kindRank(devices[i].Kind), kindRank(devices[j].Kind)
		if ki != kj {
			return ki < kj
		}
		return naturalLess(devices[i].Name, devices[j].Name)
	})
}

func kindRank(kind string) int {
	if rank, ok := kindOrder[kind]; ok {
		return rank
	}
	return len(kindOrder)
}

// naturalLess compares two names treating runs of digits as numbers, so Disk 2
// sorts before Disk 10.
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		if isDigit(a[ai]) && isDigit(b[bi]) {
			aStart, bStart := ai, bi
			for ai < len(a) && isDigit(a[ai]) {
				ai++
			}
			for bi < len(b) && isDigit(b[bi]) {
				bi++
			}
			an := strings.TrimLeft(a[aStart:ai], "0")
			bn := strings.TrimLeft(b[bStart:bi], "0")
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			continue
		}
		ca, cb := lowerByte(a[ai]), lowerByte(b[bi])
		if ca != cb {
			return ca < cb
		}
		ai++
		bi++
	}
	return len(a)-ai < len(b)-bi
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
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
