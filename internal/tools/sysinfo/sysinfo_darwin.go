//go:build darwin

package sysinfo

import (
	"strconv"
	"strings"
	"syscall"
	"time"
)

// collect fills in everything macOS will report. sw_vers, sysctl and ioreg all
// ship with macOS and none of them needs administrator rights.
func collect(r *Report) {
	if out, err := run("sw_vers"); err == nil {
		r.OSName, r.OSVersion = parseSwVers(out)
	}

	if out, err := run("sysctl", "-n", "machdep.cpu.brand_string"); err == nil {
		r.CPUModel = strings.TrimSpace(out)
	}
	if r.CPUModel == "" {
		r.markUnsupported(FieldCPUModel)
	}

	if out, err := run("sysctl", "-n", "hw.memsize"); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64); err == nil {
			r.MemoryTotal = n
		}
	}
	// vm_stat reports pages rather than bytes and its "free" excludes the file
	// cache, which would understate what an application can actually get. There
	// is no cheap macOS equivalent of MemAvailable, so the field is left empty.
	r.markUnsupported(FieldMemoryFree)

	if out, err := run("sysctl", "-n", "kern.boottime"); err == nil {
		if boot, ok := parseKernBoottime(out); ok {
			r.BootTime = boot.UTC().Format(time.RFC3339)
			r.UptimeS = int64(time.Since(boot).Seconds())
		}
	}
	if r.BootTime == "" {
		r.markUnsupported(FieldUptime, FieldBootTime)
	}

	r.Manufacturer = "Apple"
	if out, err := run("sysctl", "-n", "hw.model"); err == nil {
		r.Model = strings.TrimSpace(out)
	}
	if r.Model == "" {
		r.markUnsupported(FieldModel)
	}

	if out, err := run("ioreg", "-l", "-d", "2", "-c", "IOPlatformExpertDevice"); err == nil {
		r.Serial = parseIoregSerial(out)
	}
	if r.Serial == "" {
		r.markUnsupported(FieldSerial)
	}

	// MNT_NOWAIT (0x2) returns the cached figures instead of asking every
	// filesystem, so a disconnected network share cannot stall the whole page.
	// The standard library's darwin syscall package does not export the name.
	const mntNoWait = 0x2

	buf := make([]syscall.Statfs_t, 64)
	n, err := syscall.Getfsstat(buf, mntNoWait)
	if err != nil {
		return
	}
	for _, fs := range buf[:n] {
		name := goString(fs.Fstypename[:])
		if pseudoFS[name] {
			continue
		}
		bsize := int64(fs.Bsize)
		total := int64(fs.Blocks) * bsize
		if total <= 0 {
			continue
		}
		free := int64(fs.Bavail) * bsize
		used := int64(fs.Blocks-fs.Bfree) * bsize
		r.Disks = append(r.Disks, diskFrom(goString(fs.Mntonname[:]), name, total, free, used))
	}
}

// goString turns one of the fixed-size C character arrays in Statfs_t into a
// Go string, stopping at the first NUL.
func goString(b []int8) string {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}
