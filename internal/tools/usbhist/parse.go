package usbhist

import (
	"encoding/binary"
	"strconv"
	"strings"
	"time"
)

// The exact sentence for each operating system. It is always shown, because a
// tech who does not read it will take an empty history to mean nothing was ever
// plugged in.
const (
	noteWindows = "Windows records USB storage devices it has seen before, so the list includes sticks and drives that are not plugged in now. It is a good record of storage, and a patchy one of everything else: a mouse or a dongle may not appear once it is unplugged. Nothing here proves a device was used, only that it was connected."
	noteLinux   = "Linux does not keep a list a normal user can read of devices that have been unplugged, so this is only what is connected right now. Plug the device in and press Refresh to see it."
	noteDarwin  = "macOS does not keep a list of devices that have been unplugged, so this is only what is connected right now. Plug the device in and press Refresh to see it."
	noteOther   = "CHIT does not know how to list USB devices on this operating system."
)

// Sentences appended when a source failed, so the operating system's own
// explanation is never replaced by the failure.
const (
	failWindows  = " This computer would not let CHIT read the device list out of the registry, so nothing is shown. That is unusual and normally means the machine is locked down."
	failProfiler = " macOS did not answer within ten seconds, so no devices are listed. Try Refresh."
	failSysfs    = " This machine has no /sys/bus/usb, so no USB devices can be listed. That happens inside some containers."
)

// noteFor is the sentence under the count line.
func noteFor(os string) string {
	switch os {
	case "windows":
		return noteWindows
	case "linux":
		return noteLinux
	case "darwin":
		return noteDarwin
	}
	return noteOther
}

// timeLayout is how every timestamp crossing to the page is written.
const timeLayout = time.RFC3339

// filetimeEpoch is the number of 100 ns ticks between 1601-01-01 and the Unix
// epoch. The value was computed with python3 from the two definitions, not
// derived from this package, so an encoder and its own inverse cannot agree on
// the same mistake.
const filetimeEpoch = 116444736000000000

// filetimeToTime converts the 8 byte little-endian Windows FILETIME that the
// device install-time registry value holds.
func filetimeToTime(raw []byte) (time.Time, bool) {
	if len(raw) != 8 {
		return time.Time{}, false
	}
	ticks := binary.LittleEndian.Uint64(raw)
	if ticks == 0 {
		return time.Time{}, false
	}
	// 100 ns ticks: ten to the microsecond.
	micros := (int64(ticks) - filetimeEpoch) / 10
	return time.UnixMicro(micros).UTC(), true
}

// parseUSBStorKey reads the manufacturer, product and revision out of a USBSTOR
// subkey name, which Windows writes as
// "Disk&Ven_SanDisk&Prod_Cruzer_Blade&Rev_1.00".
func parseUSBStorKey(key string) (manufacturer, product, revision string) {
	for _, part := range strings.Split(key, "&") {
		switch {
		case strings.HasPrefix(part, "Ven_"):
			manufacturer = underscoresToSpaces(strings.TrimPrefix(part, "Ven_"))
		case strings.HasPrefix(part, "Prod_"):
			product = underscoresToSpaces(strings.TrimPrefix(part, "Prod_"))
		case strings.HasPrefix(part, "Rev_"):
			revision = underscoresToSpaces(strings.TrimPrefix(part, "Rev_"))
		}
	}
	return manufacturer, product, revision
}

func underscoresToSpaces(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "_", " "))
}

// parseVidPidKey reads the vendor and product ids out of a USB subkey name,
// which Windows writes as "VID_1D6B&PID_0002" and sometimes with an interface
// number on the end.
func parseVidPidKey(key string) (vendor, product string) {
	for _, part := range strings.Split(key, "&") {
		switch {
		case strings.HasPrefix(part, "VID_"):
			vendor = normaliseHex(strings.TrimPrefix(part, "VID_"))
		case strings.HasPrefix(part, "PID_"):
			product = normaliseHex(strings.TrimPrefix(part, "PID_"))
		}
	}
	return vendor, product
}

// parseDeviceDesc pulls the readable half out of a Windows device description,
// which is written as "@usbstor.inf,%disk_devdesc%;Disk drive".
func parseDeviceDesc(value string) string {
	if at := strings.LastIndex(value, ";"); at >= 0 {
		return strings.TrimSpace(value[at+1:])
	}
	return strings.TrimSpace(value)
}

// kindFromService maps the Windows driver name onto the shared Kind column.
func kindFromService(service string) string {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "usbstor", "disk", "uaspstor":
		return KindStorage
	case "hidusb", "kbdhid", "mouhid", "hidclass":
		return KindInput
	case "usbaudio", "usbaudio2":
		return KindAudio
	case "usbvideo":
		return KindVideo
	case "usbhub", "usbhub3", "usbxhci", "usbehci":
		return KindHub
	case "usbprint":
		return KindPrinter
	case "rndismp", "usbncm", "rndismp6":
		return KindNetwork
	}
	return KindOther
}

// kindFromClass maps a USB class code onto the shared Kind column.
//
// Classes 00 and ef return an empty string, which tells the caller to look at
// the device's first interface instead, because the class is declared there
// rather than on the device. Class ef is "miscellaneous", the marker a
// composite device uses when it carries an Interface Association Descriptor,
// and nearly every modern webcam, fingerprint reader and Bluetooth radio is
// one. Treating ef as "other" reported a real webcam as Other rather than
// Camera, which is what this rule exists to prevent.
func kindFromClass(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "08":
		return KindStorage
	case "03":
		return KindInput
	case "01":
		return KindAudio
	case "0e":
		return KindVideo
	case "09":
		return KindHub
	case "07":
		return KindPrinter
	case "02", "0a":
		return KindNetwork
	case "00", "ef", "":
		return ""
	}
	return KindOther
}

// parseSysfsHex reads one of the four-hex-digit files under /sys/bus/usb.
func parseSysfsHex(text string) string {
	return normaliseHex(text)
}

func normaliseHex(text string) string {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	if trimmed == "" {
		return ""
	}
	if _, err := strconv.ParseUint(trimmed, 16, 64); err != nil {
		return ""
	}
	return trimmed
}

// parseProfilerHex reads the macOS form of an id, which system_profiler writes
// as "0x05ac  (Apple Inc.)" or sometimes as a name with no number at all.
func parseProfilerHex(value string) (id, name string) {
	text := strings.TrimSpace(value)
	if open := strings.Index(text, "("); open >= 0 {
		if close := strings.LastIndex(text, ")"); close > open {
			name = strings.TrimSpace(text[open+1 : close])
		}
		text = strings.TrimSpace(text[:open])
	}
	if !strings.HasPrefix(strings.ToLower(text), "0x") {
		return "", name
	}
	return normaliseHex(strings.TrimPrefix(strings.ToLower(text), "0x")), name
}
