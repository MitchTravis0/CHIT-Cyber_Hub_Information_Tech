//go:build windows

package usbhist

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	enumUSBStor = `SYSTEM\CurrentControlSet\Enum\USBSTOR`
	enumUSB     = `SYSTEM\CurrentControlSet\Enum\USB`

	// installTimeKey is where Windows records when a device instance was first
	// installed, as an 8 byte FILETIME under the device property store.
	installTimeKey = `Properties\{83da6326-97a6-4088-9453-a1923f573b29}\0064`
)

// collect reads the two device enumeration keys. Both are readable by
// authenticated users on a standard install; a hardened build may restrict
// them, which is handled and explained rather than treated as an error.
func collect(r *Report) {
	r.History = true
	r.Note = noteWindows

	storage := collectStor(r)
	general := collectUSB(r)

	if !storage && !general {
		r.markUnsupported(FieldHistory)
		r.Note += failWindows
	}
}

// collectStor walks USBSTOR, which is the good record: every mass-storage
// device Windows has seen, keyed by its serial number.
func collectStor(r *Report) bool {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, enumUSBStor, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer root.Close()

	models, err := root.ReadSubKeyNames(0)
	if err != nil {
		return false
	}

	for _, model := range models {
		manufacturer, product, _ := parseUSBStorKey(model)

		instances, err := subKeyNames(enumUSBStor + `\` + model)
		if err != nil {
			continue
		}
		for _, instance := range instances {
			path := enumUSBStor + `\` + model + `\` + instance
			device := Device{
				Manufacturer: manufacturer,
				Name:         product,
				Serial:       instanceSerial(instance),
				Kind:         KindStorage,
				Connected:    instanceConnected(path),
				Source:       "USBSTOR",
			}
			if key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE); err == nil {
				if friendly, _, err := key.GetStringValue("FriendlyName"); err == nil && strings.TrimSpace(friendly) != "" {
					device.Name = parseDeviceDesc(friendly)
				}
				key.Close()
			}
			if device.Name == "" {
				device.Name = manufacturer
			}
			if strings.TrimSpace(device.Name) == "" {
				continue
			}
			device.FirstSeen = firstSeen(r, path)
			r.Devices = append(r.Devices, device)
		}
	}
	return true
}

// collectUSB walks the general enumeration key, which covers everything else
// and is a patchier record than USBSTOR.
func collectUSB(r *Report) bool {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, enumUSB, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer root.Close()

	models, err := root.ReadSubKeyNames(0)
	if err != nil {
		return false
	}

	for _, model := range models {
		vendorID, productID := parseVidPidKey(model)

		instances, err := subKeyNames(enumUSB + `\` + model)
		if err != nil {
			continue
		}
		for _, instance := range instances {
			path := enumUSB + `\` + model + `\` + instance
			key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			desc, _, _ := key.GetStringValue("DeviceDesc")
			mfg, _, _ := key.GetStringValue("Mfg")
			service, _, _ := key.GetStringValue("Service")
			key.Close()

			device := Device{
				Name:         parseDeviceDesc(desc),
				Manufacturer: parseDeviceDesc(mfg),
				VendorID:     vendorID,
				ProductID:    productID,
				Serial:       instanceSerial(instance),
				Kind:         kindFromService(service),
				Connected:    instanceConnected(path),
				Source:       "USB",
			}
			if device.Name == "" && vendorID != "" && productID != "" {
				device.Name = vendorID + ":" + productID
			}
			if strings.TrimSpace(device.Name) == "" {
				continue
			}
			device.FirstSeen = firstSeen(r, path)
			r.Devices = append(r.Devices, device)
		}
	}
	return true
}

func subKeyNames(path string) ([]string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer key.Close()
	return key.ReadSubKeyNames(0)
}

// instanceSerial is the instance key's own name, which for a device that
// reports one is its serial number. Windows appends "&0" to a made-up id for a
// device with no serial of its own, and that is not worth showing as one.
func instanceSerial(instance string) string {
	if strings.Contains(instance, "&") {
		return ""
	}
	return instance
}

// instanceConnected reports whether the device is present now. Windows creates
// a Control subkey holding ActiveService while a device is attached and removes
// it when the device goes. Where it cannot be told, the answer is "not
// connected": showing a disconnected device as connected would be a lie, while
// the other way round merely understates.
func instanceConnected(path string) bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path+`\Control`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	_, _, err = key.GetStringValue("ActiveService")
	return err == nil
}

// firstSeen reads the device install timestamp, or returns "" and records the
// gap so the column is empty rather than wrong.
func firstSeen(r *Report, path string) string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path+`\`+installTimeKey, registry.QUERY_VALUE)
	if err != nil {
		r.markUnsupported(FieldFirstSeen)
		return ""
	}
	defer key.Close()

	raw, _, err := key.GetBinaryValue("")
	if err != nil {
		r.markUnsupported(FieldFirstSeen)
		return ""
	}
	at, ok := filetimeToTime(raw)
	if !ok {
		r.markUnsupported(FieldFirstSeen)
		return ""
	}
	return at.Format(timeLayout)
}
