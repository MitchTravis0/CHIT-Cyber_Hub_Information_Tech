//go:build windows

package sysinfo

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// memoryStatusEx mirrors MEMORYSTATUSEX. GlobalMemoryStatusEx is not wrapped by
// golang.org/x/sys/windows, so it is called through the lazy DLL loader that
// package already provides.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount64     = kernel32.NewProc("GetTickCount64")
)

// collect fills in everything Windows will report without administrator
// rights. Registry reads cover the operating system, the processor and the
// machine model; the serial number needs WMI and is the one field that may be
// missing.
func collect(r *Report) {
	collectVersion(r)
	collectCPU(r)
	collectMemory(r)
	collectUptime(r)
	collectMachine(r)
	collectDisks(r)
}

func collectVersion(r *Report) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer key.Close()

	product, _, _ := key.GetStringValue("ProductName")
	buildText, _, _ := key.GetStringValue("CurrentBuildNumber")
	ubr, _, _ := key.GetIntegerValue("UBR")
	major, _, _ := key.GetIntegerValue("CurrentMajorVersionNumber")
	minor, _, _ := key.GetIntegerValue("CurrentMinorVersionNumber")

	build := atoiSafe(buildText)
	r.OSName = windowsOSName(product, build)
	r.OSVersion = windowsVersionString(int(major), int(minor), build, int(ubr))

	// A "DisplayVersion" of 24H2 is what a tech reads off winver, so it goes on
	// the end of the name where there is one.
	if display, _, err := key.GetStringValue("DisplayVersion"); err == nil && display != "" && r.OSName != "" {
		r.OSName += " " + display
	}
}

func collectCPU(r *Report) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		r.markUnsupported(FieldCPUModel)
		return
	}
	defer key.Close()

	name, _, err := key.GetStringValue("ProcessorNameString")
	if err != nil || strings.TrimSpace(name) == "" {
		r.markUnsupported(FieldCPUModel)
		return
	}
	r.CPUModel = strings.TrimSpace(name)
}

func collectMemory(r *Report) {
	status := memoryStatusEx{}
	status.Length = uint32(unsafe.Sizeof(status))
	ret, _, _ := procGlobalMemoryStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		r.markUnsupported(FieldMemoryFree)
		return
	}
	r.MemoryTotal = int64(status.TotalPhys)
	r.MemoryFree = int64(status.AvailPhys)
}

func collectUptime(r *Report) {
	ticks, _, _ := procGetTickCount64.Call()
	if ticks == 0 {
		r.markUnsupported(FieldUptime, FieldBootTime)
		return
	}
	secs := int64(ticks / 1000)
	r.UptimeS = secs
	r.BootTime = time.Now().Add(-time.Duration(secs) * time.Second).UTC().Format(time.RFC3339)
}

func collectMachine(r *Report) {
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE); err == nil {
		r.Manufacturer, _, _ = key.GetStringValue("SystemManufacturer")
		r.Model, _, _ = key.GetStringValue("SystemProductName")
		key.Close()
	}
	if strings.TrimSpace(r.Manufacturer) == "" {
		r.Manufacturer = ""
		r.markUnsupported(FieldManufacturer)
	}
	if strings.TrimSpace(r.Model) == "" {
		r.Model = ""
		r.markUnsupported(FieldModel)
	}

	// The serial number lives in WMI and nowhere in the registry. PowerShell is
	// on every supported Windows; when policy blocks it the field stays empty
	// rather than being guessed.
	if serial := biosSerial(); serial != "" {
		r.Serial = serial
	} else {
		r.markUnsupported(FieldSerial)
	}
}

func biosSerial() string {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive",
		"-Command", "(Get-CimInstance -ClassName Win32_BIOS).SerialNumber")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	serial := strings.TrimSpace(string(out))
	// Virtual machines and some boards report a placeholder rather than
	// admitting they have no serial. Showing it would send a tech looking up a
	// warranty for "To Be Filled By O.E.M.".
	switch strings.ToLower(serial) {
	case "", "default string", "to be filled by o.e.m.", "system serial number", "none", "0":
		return ""
	}
	return serial
}

func collectDisks(r *Report) {
	buf := make([]uint16, 256)
	n, err := windows.GetLogicalDriveStrings(uint32(len(buf)), &buf[0])
	if err != nil || n == 0 {
		return
	}

	for _, root := range splitDriveStrings(buf[:n]) {
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		// A drive type of 4 is a network share and 5 a CD-ROM. Neither is disk
		// space a tech can free up on this machine.
		switch windows.GetDriveType(rootPtr) {
		case windows.DRIVE_REMOTE, windows.DRIVE_CDROM, windows.DRIVE_UNKNOWN, windows.DRIVE_NO_ROOT_DIR:
			continue
		}

		var freeToCaller, total, totalFree uint64
		if err := windows.GetDiskFreeSpaceEx(rootPtr, &freeToCaller, &total, &totalFree); err != nil {
			continue
		}
		if total == 0 {
			continue
		}
		r.Disks = append(r.Disks, diskFrom(
			strings.TrimSuffix(root, `\`), volumeFS(rootPtr),
			int64(total), int64(freeToCaller), int64(total-totalFree)))
	}
}

// volumeFS names the filesystem on a drive (NTFS, exFAT, FAT32). An empty
// string means Windows would not say, and the column is left blank.
func volumeFS(root *uint16) string {
	fsName := make([]uint16, 32)
	err := windows.GetVolumeInformation(root, nil, 0, nil, nil, nil, &fsName[0], uint32(len(fsName)))
	if err != nil {
		return ""
	}
	return windows.UTF16ToString(fsName)
}

// splitDriveStrings unpacks the NUL-separated, NUL-terminated list
// GetLogicalDriveStrings writes.
func splitDriveStrings(buf []uint16) []string {
	var out []string
	start := 0
	for i, c := range buf {
		if c != 0 {
			continue
		}
		if i > start {
			out = append(out, windows.UTF16ToString(buf[start:i]))
		}
		start = i + 1
	}
	return out
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
