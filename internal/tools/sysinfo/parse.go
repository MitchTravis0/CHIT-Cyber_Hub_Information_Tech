package sysinfo

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Mount is one line of /proc/mounts worth keeping.
type Mount struct {
	Device string
	Path   string
	FS     string
}

// pseudoFS are filesystems that live in memory or in the kernel rather than on
// a disk. Reporting them as drives would tell a tech that /run is 16 GB and
// half full, which is true and useless.
var pseudoFS = map[string]bool{
	"autofs": true, "binfmt_misc": true, "bpf": true, "cgroup": true,
	"cgroup2": true, "configfs": true, "debugfs": true, "devpts": true,
	"devtmpfs": true, "efivarfs": true, "fuse.gvfsd-fuse": true,
	"fuse.portal": true, "fusectl": true, "hugetlbfs": true, "mqueue": true,
	"nsfs": true, "overlay": true, "proc": true, "pstore": true,
	"ramfs": true, "rpc_pipefs": true, "securityfs": true, "selinuxfs": true,
	"squashfs": true, "sysfs": true, "tmpfs": true, "tracefs": true,
	"devfs": true, "map": true,
}

// parseOSRelease pulls the display name and version out of /etc/os-release.
// PRETTY_NAME is what a distribution wants shown; NAME plus VERSION_ID is the
// fallback for the handful that omit it.
func parseOSRelease(text string) (name, versionID string) {
	var plain, version string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = unquote(strings.TrimSpace(value))
		switch strings.TrimSpace(key) {
		case "PRETTY_NAME":
			name = value
		case "NAME":
			plain = value
		case "VERSION_ID":
			version = value
		}
	}
	if name == "" && plain != "" {
		name = strings.TrimSpace(plain + " " + version)
	}
	return name, version
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// parseMeminfo reads MemTotal and MemAvailable out of /proc/meminfo. The "kB"
// in that file means 1024 bytes, not 1000. hasAvailable is false on kernels
// before 3.14, which have no MemAvailable line at all.
func parseMeminfo(text string) (total, available int64, hasAvailable bool) {
	for _, line := range strings.Split(text, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		kb, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "MemTotal":
			total = kb * 1024
		case "MemAvailable":
			available, hasAvailable = kb*1024, true
		}
	}
	return total, available, hasAvailable
}

// parseProcUptime reads the seconds this machine has been up out of the first
// field of /proc/uptime.
func parseProcUptime(text string) (int64, bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return 0, false
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || secs < 0 || math.IsInf(secs, 0) || math.IsNaN(secs) {
		return 0, false
	}
	return int64(secs), true
}

// parseCPUInfo returns the processor's marketing name from /proc/cpuinfo. Every
// core repeats it, so the first one wins. ARM boards have no "model name" line
// at all and get an empty string rather than a guess.
func parseCPUInfo(text string) string {
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "model name" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// parseProcMounts keeps the mounts that are really on a disk. A device mounted
// twice is kept twice, because both paths genuinely exist.
func parseProcMounts(text string) []Mount {
	out := []Mount{}
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		fs := fields[2]
		if pseudoFS[fs] || strings.HasPrefix(fs, "fuse.") {
			continue
		}
		path := unescapeMountPath(fields[1])
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, Mount{Device: unescapeMountPath(fields[0]), Path: path, FS: fs})
	}
	return out
}

// unescapeMountPath undoes the octal escapes the kernel writes into
// /proc/mounts, so "/mnt/my\040disk" reads as "/mnt/my disk".
func unescapeMountPath(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseSwVers reads the macOS name and build out of sw_vers output.
func parseSwVers(text string) (name, build string) {
	var product, version string
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "ProductName":
			product = value
		case "ProductVersion":
			version = value
		case "BuildVersion":
			build = value
		}
	}
	return strings.TrimSpace(product + " " + version), build
}

// parseKernBoottime reads the boot instant out of "sysctl -n kern.boottime",
// which prints it as: { sec = 1753612800, usec = 123456 } Sun Jul 27 ...
func parseKernBoottime(text string) (time.Time, bool) {
	_, rest, ok := strings.Cut(text, "sec =")
	if !ok {
		return time.Time{}, false
	}
	rest = strings.TrimSpace(rest)
	end := strings.IndexAny(rest, ", }")
	if end < 0 {
		end = len(rest)
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(rest[:end]), 10, 64)
	if err != nil || secs <= 0 {
		return time.Time{}, false
	}
	return time.Unix(secs, 0), true
}

// parseIoregSerial pulls the serial number out of ioreg output, which prints it
// as: "IOPlatformSerialNumber" = "C02XY1234567"
func parseIoregSerial(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "IOPlatformSerialNumber") {
			continue
		}
		_, rest, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if len(rest) >= 2 && rest[0] == '"' && rest[len(rest)-1] == '"' {
			return rest[1 : len(rest)-1]
		}
	}
	return ""
}

// windowsOSName corrects the name Windows 11 still reports for itself. The
// registry's ProductName was never updated, so every Windows 11 machine calls
// itself "Windows 10 <edition>" and only the build number tells them apart.
// Build 22000 is the first Windows 11 release.
func windowsOSName(productName string, build int) string {
	if productName == "" {
		return ""
	}
	if build >= 22000 && strings.HasPrefix(productName, "Windows 10") {
		return "Windows 11" + strings.TrimPrefix(productName, "Windows 10")
	}
	return productName
}

// windowsVersionString assembles the four-part build a tech reads out of winver.
// The update build revision is omitted when it is zero, matching winver.
func windowsVersionString(major, minor, build, ubr int) string {
	base := strconv.Itoa(major) + "." + strconv.Itoa(minor) + "." + strconv.Itoa(build)
	if ubr == 0 {
		return base
	}
	return base + "." + strconv.Itoa(ubr)
}

// diskFrom turns raw block counts into the row the page shows, matching what df
// prints so the two can be compared. Used counts every block in use including
// the filesystem's own reserve, free counts only what an ordinary user could
// claim, and the percentage is used over used-plus-free. That is why a "100%
// full" disk can still have blocks left: they belong to root.
func diskFrom(mount, fs string, total, free, used int64) Disk {
	d := Disk{Mount: mount, FS: fs, Total: total, Free: free, Used: used}
	if used+free > 0 {
		d.UsedPct = math.Round(float64(used)/float64(used+free)*1000) / 10
	}
	return d
}
