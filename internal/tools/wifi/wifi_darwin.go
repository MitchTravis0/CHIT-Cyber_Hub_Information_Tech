//go:build darwin

package wifi

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// collect asks system_profiler. The airport command-line tool is deliberately
// not used: Apple removed it in macOS 14, and a tool that works only on old
// versions is worse than one that says what it can see.
func collect(r *Report) {
	out, err := run("system_profiler", "-json", "SPAirPortDataType")
	if err != nil {
		r.Note = noteFor("darwin", 0, false)
		return
	}

	ssid := false
	for _, link := range linksFromProfiler([]byte(out)) {
		if link.SSID != "" {
			ssid = true
		}
		r.Links = append(r.Links, link)
	}
	if !ssid && len(r.Links) > 0 {
		// Recent macOS hides the network name from an app that has not been
		// granted location access. Everything else on the page is still real.
		r.markUnsupported(FieldSSID)
	}

	r.Note = noteFor("darwin", len(r.Links), ssid)
}

// linksFromProfiler walks the JSON tree defensively, exactly as usbhist walks
// SPUSBDataType: every key may be absent and none of them may be trusted to be
// the type it usually is.
func linksFromProfiler(raw []byte) []Link {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	top, _ := doc["SPAirPortDataType"].([]any)

	var links []Link
	for _, node := range top {
		item, _ := node.(map[string]any)
		interfaces, _ := item["spairport_airport_interfaces"].([]any)
		for _, entry := range interfaces {
			iface, _ := entry.(map[string]any)
			if iface == nil {
				continue
			}
			link := Link{
				Interface: stringAt(iface, "_name"),
				Source:    "system_profiler",
			}
			current, _ := iface["spairport_current_network_information"].(map[string]any)
			if current != nil {
				link.Connected = true
				link.SSID = stringAt(current, "_name")
				link.BSSID = stringAt(current, "spairport_network_bssid")
				link.Security = stringAt(current, "spairport_security_mode")
				link.SignalDBm = parseSignalNoise(stringAt(current, "spairport_signal_noise"))
				link.Channel, link.Band, link.WidthMHz =
					parseAirportChannel(stringAt(current, "spairport_network_channel"))
				// macOS reports one rate rather than a send and a receive one.
				rate := leadingFloat(stringAt(current, "spairport_network_rate"))
				link.RxMbps, link.TxMbps = rate, rate
			}
			if link.Interface != "" {
				links = append(links, link)
			}
		}
	}
	return links
}

func stringAt(item map[string]any, key string) string {
	if value, ok := item[key].(string); ok {
		return value
	}
	return ""
}

func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), profilerCommandTimeout*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
