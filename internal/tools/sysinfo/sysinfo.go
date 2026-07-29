// Package sysinfo describes the machine CHIT is running on: operating system,
// processor, memory, drives, uptime and serial number. Fields an operating
// system will not hand over without administrator rights are returned empty and
// named in Report.Unsupported, never guessed.
package sysinfo

import (
	"os"
	"os/user"
	"runtime"

	"chit/internal/core"
)

// Ids used in Report.Unsupported, so the page can say "not available on this
// OS" against the right row instead of showing a blank.
const (
	FieldSerial       = "serial"
	FieldModel        = "model"
	FieldManufacturer = "manufacturer"
	FieldCPUModel     = "cpuModel"
	FieldMemoryFree   = "memoryFree"
	FieldUptime       = "uptime"
	FieldBootTime     = "bootTime"
	FieldDisks        = "disks"
	FieldUser         = "user"
)

// commandTimeout caps every external command. A timeout is "this operating
// system did not say", never an error.
const commandTimeout = 3 // seconds

// Disk is one mounted filesystem with space on it.
type Disk struct {
	Mount   string  `json:"mount"`
	FS      string  `json:"fs"`
	Total   int64   `json:"total"`
	Used    int64   `json:"used"`
	Free    int64   `json:"free"`
	UsedPct float64 `json:"usedPct"`
}

// Report is the whole snapshot. Every string field is empty rather than guessed
// when the operating system will not say, and the field id is listed in
// Unsupported so the page can explain the gap.
type Report struct {
	Hostname     string `json:"hostname"`
	User         string `json:"user"`
	OS           string `json:"os"`
	OSName       string `json:"osName"`
	OSVersion    string `json:"osVersion"`
	Arch         string `json:"arch"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	CPUModel     string `json:"cpuModel"`
	CPUCores     int    `json:"cpuCores"`
	MemoryTotal  int64  `json:"memoryTotal"`
	MemoryFree   int64  `json:"memoryFree"`
	UptimeS      int64  `json:"uptimeS"`
	BootTime     string `json:"bootTime"`
	// AppVersion is filled in by the bindings layer, because this package must
	// not import package main.
	AppVersion  string   `json:"appVersion"`
	Disks       []Disk   `json:"disks"`
	Unsupported []string `json:"unsupported"`
}

// New reads everything this operating system will report about the machine.
// It never fails for a source that is unavailable: that source is left empty
// and named in Unsupported.
func New() (Report, error) {
	r := Report{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
		// Both slices are initialised so they marshal as [] rather than null.
		Disks:       []Disk{},
		Unsupported: []string{},
	}
	r.Hostname, _ = os.Hostname()

	if u, err := user.Current(); err == nil {
		r.User = u.Username
	} else {
		r.markUnsupported(FieldUser)
	}

	collect(&r)

	if len(r.Disks) == 0 {
		r.markUnsupported(FieldDisks)
	}
	if r.Hostname == "" && r.OSName == "" && r.CPUCores == 0 {
		return r, core.Errorf(core.CodeInternal,
			"Could not read this machine's details. That is unusual: try Refresh, and if it keeps happening the operating system is refusing something CHIT normally gets for free.")
	}
	return r, nil
}

// markUnsupported records a field this operating system would not report, once,
// keeping insertion order so the page reads them in the order they were found.
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
