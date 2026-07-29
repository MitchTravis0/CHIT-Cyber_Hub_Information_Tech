//go:build linux

package usbhist

import (
	"os"
	"path/filepath"
	"strings"
)

const sysfsUSB = "/sys/bus/usb/devices"

// collect reads /sys/bus/usb/devices, which needs no privileges at all.
//
// Linux keeps no record a normal user can read of devices that have been
// unplugged: it is in the kernel ring buffer (often restricted by
// kernel.dmesg_restrict) or the journal (needs a group), and both roll over.
// Reading those would produce an incomplete history that looks complete, which
// is worse than none, so this reports only what is connected and says so.
func collect(r *Report) {
	r.History = false
	r.markUnsupported(FieldHistory)
	r.Note = noteLinux

	entries, err := os.ReadDir(sysfsUSB)
	if err != nil {
		r.Note += failSysfs
		return
	}

	for _, entry := range entries {
		// A name holding a colon is one interface of a device, not a device.
		if strings.Contains(entry.Name(), ":") {
			continue
		}
		dir := filepath.Join(sysfsUSB, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "idVendor")); err != nil {
			continue
		}

		device := Device{
			VendorID:     parseSysfsHex(readSysfs(dir, "idVendor")),
			ProductID:    parseSysfsHex(readSysfs(dir, "idProduct")),
			Manufacturer: readSysfs(dir, "manufacturer"),
			Name:         readSysfs(dir, "product"),
			Serial:       readSysfs(dir, "serial"),
			Kind:         deviceKind(dir, entry.Name()),
			Connected:    true,
			Source:       "/sys/bus/usb",
		}
		if device.Name == "" {
			device.Name = fallbackName(device, entry.Name())
		}
		if device.Name == "" {
			continue
		}
		r.Devices = append(r.Devices, device)
	}
}

// deviceKind takes the device class where there is one, and otherwise the class
// of the device's first interface, which is where a composite device declares
// what it actually is.
func deviceKind(dir, name string) string {
	if kind := kindFromClass(readSysfs(dir, "bDeviceClass")); kind != "" {
		return kind
	}
	if kind := kindFromClass(readSysfs(dir, name+":1.0", "bInterfaceClass")); kind != "" {
		return kind
	}
	return KindOther
}

// fallbackName keeps a row from ever rendering nameless.
func fallbackName(d Device, key string) string {
	if d.VendorID != "" && d.ProductID != "" {
		return d.VendorID + ":" + d.ProductID
	}
	return strings.TrimSpace(key)
}

// readSysfs joins the parts and returns the trimmed contents, or "".
func readSysfs(parts ...string) string {
	data, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
