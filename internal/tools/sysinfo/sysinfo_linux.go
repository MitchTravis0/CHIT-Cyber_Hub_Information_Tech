//go:build linux

package sysinfo

import (
	"strings"
	"syscall"
	"time"
)

// collect fills in everything Linux will report. Every value is a file read or
// a syscall: nothing here shells out, and nothing needs root.
func collect(r *Report) {
	r.OSName, _ = parseOSRelease(readFile("/etc/os-release"))
	r.OSVersion = strings.TrimSpace(readFile("/proc/sys/kernel/osrelease"))

	r.CPUModel = parseCPUInfo(readFile("/proc/cpuinfo"))
	if r.CPUModel == "" {
		r.markUnsupported(FieldCPUModel)
	}

	total, available, hasAvailable := parseMeminfo(readFile("/proc/meminfo"))
	r.MemoryTotal = total
	if hasAvailable {
		r.MemoryFree = available
	} else {
		r.markUnsupported(FieldMemoryFree)
	}

	if secs, ok := parseProcUptime(readFile("/proc/uptime")); ok {
		r.UptimeS = secs
		r.BootTime = time.Now().Add(-time.Duration(secs) * time.Second).UTC().Format(time.RFC3339)
	} else {
		r.markUnsupported(FieldUptime, FieldBootTime)
	}

	// DMI is how a PC reports its own model. product_serial is mode 0400 on
	// every mainstream distribution, so a normal user gets nothing and the page
	// says so rather than showing a blank.
	r.Manufacturer = strings.TrimSpace(readFile("/sys/class/dmi/id/sys_vendor"))
	r.Model = strings.TrimSpace(readFile("/sys/class/dmi/id/product_name"))
	r.Serial = strings.TrimSpace(readFile("/sys/class/dmi/id/product_serial"))
	if r.Manufacturer == "" {
		r.markUnsupported(FieldManufacturer)
	}
	if r.Model == "" {
		r.markUnsupported(FieldModel)
	}
	if r.Serial == "" {
		r.markUnsupported(FieldSerial)
	}

	for _, m := range parseProcMounts(readFile("/proc/mounts")) {
		var fs syscall.Statfs_t
		if err := syscall.Statfs(m.Path, &fs); err != nil {
			continue
		}
		bsize := int64(fs.Bsize)
		total := int64(fs.Blocks) * bsize
		if total <= 0 {
			continue
		}
		free := int64(fs.Bavail) * bsize
		used := int64(fs.Blocks-fs.Bfree) * bsize
		r.Disks = append(r.Disks, diskFrom(m.Path, m.FS, total, free, used))
	}
}
