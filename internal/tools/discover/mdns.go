package discover

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// ServiceTypes are the DNS-SD service types CHIT asks about. The first entry
// is the service enumeration meta-query, which asks a responder to name every
// service type it offers.
var ServiceTypes = []string{
	"_services._dns-sd._udp.local.",
	"_http._tcp.local.",
	"_https._tcp.local.",
	"_ipp._tcp.local.",
	"_ipps._tcp.local.",
	"_printer._tcp.local.",
	"_pdl-datastream._tcp.local.",
	"_scanner._tcp.local.",
	"_uscan._tcp.local.",
	"_smb._tcp.local.",
	"_afpovertcp._tcp.local.",
	"_ssh._tcp.local.",
	"_sftp-ssh._tcp.local.",
	"_rfb._tcp.local.",
	"_workstation._tcp.local.",
	"_device-info._tcp.local.",
	"_googlecast._tcp.local.",
	"_airplay._tcp.local.",
	"_raop._tcp.local.",
	"_spotify-connect._tcp.local.",
	"_hap._tcp.local.",
	"_daap._tcp.local.",
	"_companion-link._tcp.local.",
}

// classINWithUnicast is the question class with the mDNS QU bit set. It asks
// responders to answer directly to the source port rather than to the multicast
// group, which is what lets CHIT hear them without binding port 5353 (already
// owned by mDNSResponder on macOS and Avahi on Linux).
const classINWithUnicast = 0x8001

// txtKeysWorthShowing are the TXT keys that carry something a tech would read.
// Every service type invents its own keys, so anything not listed is dropped
// rather than dumped on screen.
var txtKeysWorthShowing = []string{"ty", "product", "model", "md", "usb_mdl", "note", "rp", "fn"}

// mdnsQuery builds one message asking for every service type at once, which is
// one packet instead of twenty-three.
func mdnsQuery() ([]byte, error) {
	out := make([]byte, headerBytes)
	// Transaction id 0: mDNS does not match responses by id, and a fixed zero
	// makes the packet identical every run, which a test can pin.
	binary.BigEndian.PutUint16(out[4:], uint16(len(ServiceTypes)))

	for _, service := range ServiceTypes {
		name, err := encodeName(service)
		if err != nil {
			return nil, err
		}
		out = append(out, name...)
		out = binary.BigEndian.AppendUint16(out, typePTR)
		out = binary.BigEndian.AppendUint16(out, classINWithUnicast)
	}
	return out, nil
}

// devicesFromMDNS turns one response into devices. src is the address the packet
// came from, used whenever the responder did not include an A record, so a
// device that answered is never listed without an address.
func devicesFromMDNS(payload []byte, src, adapter string) []Device {
	msg, err := decodeMessage(payload)
	if err != nil && len(msg.records) == 0 {
		return nil
	}

	// Collect what the message says, then assemble. A responder may put the
	// records in any section and any order.
	addresses := map[string]string{}
	ports := map[string]int{}
	targets := map[string]string{}
	details := map[string]string{}
	instances := map[string]bool{}

	for _, r := range msg.records {
		switch r.rtype {
		case typeA:
			if len(r.data) == 4 {
				addresses[r.name] = fmt.Sprintf("%d.%d.%d.%d", r.data[0], r.data[1], r.data[2], r.data[3])
			}
		case typePTR:
			if target, err := parsePTR(payload, r); err == nil && isInstanceOf(target, r.name) {
				instances[target] = true
			}
		case typeSRV:
			if service, err := parseSRV(payload, r); err == nil {
				instances[r.name] = true
				ports[r.name] = service.port
				targets[r.name] = service.target
			}
		case typeTXT:
			if values, err := parseTXT(r.data); err == nil {
				if line := detailLine(values); line != "" {
					details[r.name] = line
				}
			}
		}
	}

	names := make([]string, 0, len(instances))
	for name := range instances {
		names = append(names, name)
	}
	// Sorted so one packet always produces the same order, whatever the map
	// iteration did.
	sort.Strings(names)

	out := make([]Device, 0, len(names))
	for _, instance := range names {
		label, service := splitInstance(instance)
		ip := src
		if host := targets[instance]; host != "" {
			if a, ok := addresses[host]; ok {
				ip = a
			}
		}
		out = append(out, newDevice(Device{
			Protocol: ProtocolMDNS,
			IP:       ip,
			Name:     label,
			Service:  service,
			Host:     targets[instance],
			Port:     ports[instance],
			Details:  details[instance],
			Adapter:  adapter,
		}))
	}
	return out
}

// isInstanceOf reports whether target is an instance of the service type owner,
// which is what separates a real answer from the meta-query's list of service
// types. "_services._dns-sd._udp.local" answers name service types, not devices.
func isInstanceOf(target, owner string) bool {
	if owner == strings.TrimSuffix(ServiceTypes[0], ".") {
		return false
	}
	return strings.HasSuffix(target, "."+owner) && len(target) > len(owner)+1
}

// splitInstance separates "Brother HL-L2350DW._ipp._tcp.local" into the name a
// person reads and the service type.
func splitInstance(instance string) (name, service string) {
	trimmed := strings.TrimSuffix(instance, ".local")
	i := strings.Index(trimmed, "._")
	if i < 0 {
		return trimmed, ""
	}
	return trimmed[:i], trimmed[i+1:]
}

// detailLine picks the TXT values worth showing and joins them. A device can
// publish twenty keys of internal state, and none of it belongs on screen.
func detailLine(values []string) string {
	var parts []string
	for _, want := range txtKeysWorthShowing {
		for _, value := range values {
			key, rest, ok := strings.Cut(value, "=")
			if !ok || !strings.EqualFold(key, want) || rest == "" {
				continue
			}
			parts = append(parts, rest)
			break
		}
	}
	return strings.Join(parts, ", ")
}
