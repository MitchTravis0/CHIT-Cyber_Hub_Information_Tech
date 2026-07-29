//go:build darwin

package usbhist

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// collect asks system_profiler for the USB tree.
//
// macOS keeps no record of devices that have been unplugged, so this reports
// only what is connected and the note says so. system_profiler is slow (two to
// six seconds is normal), which is why the timeout is ten.
func collect(r *Report) {
	r.History = false
	r.markUnsupported(FieldHistory)
	r.Note = noteDarwin

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "system_profiler", "-json", "SPUSBDataType").Output()
	if err != nil {
		r.Note += failProfiler
		return
	}

	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		r.Note += failProfiler
		return
	}

	items, _ := doc["SPUSBDataType"].([]any)
	walkProfiler(r, items)
}

// walkProfiler descends the tree system_profiler prints, in which hubs hold
// their children under _items to an arbitrary depth.
func walkProfiler(r *Report, items []any) {
	for _, raw := range items {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		children, isHub := node["_items"].([]any)

		vendorID, vendorName := parseProfilerHex(profilerString(node, "vendor_id"))
		productID, _ := parseProfilerHex(profilerString(node, "product_id"))

		device := Device{
			Name:         profilerString(node, "_name"),
			Manufacturer: profilerString(node, "manufacturer"),
			VendorID:     vendorID,
			ProductID:    productID,
			Serial:       profilerString(node, "serial_num"),
			Connected:    true,
			Source:       "system_profiler",
		}
		if device.Manufacturer == "" {
			device.Manufacturer = vendorName
		}
		switch {
		case node["Media"] != nil:
			device.Kind = KindStorage
		case isHub:
			device.Kind = KindHub
		default:
			device.Kind = KindOther
		}
		if device.Name == "" {
			if vendorID != "" && productID != "" {
				device.Name = vendorID + ":" + productID
			}
		}
		if strings.TrimSpace(device.Name) != "" {
			r.Devices = append(r.Devices, device)
		}

		if isHub {
			walkProfiler(r, children)
		}
	}
}

func profilerString(node map[string]any, key string) string {
	value, _ := node[key].(string)
	return strings.TrimSpace(value)
}
