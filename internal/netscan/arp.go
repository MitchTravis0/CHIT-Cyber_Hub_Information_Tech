package netscan

import (
	"context"
	"net/netip"
	"os/exec"
	"strings"
	"time"
)

// arpCommandTimeout bounds the helper commands. Reading a cache is instant; if
// it is not, the machine has bigger problems than a missing MAC address.
const arpCommandTimeout = 3 * time.Second

// ARPEntry is one IPv4 to MAC mapping read back from the system ARP cache.
type ARPEntry struct {
	IP  string `json:"ip"`
	MAC string `json:"mac"`
}

// normalizeMAC returns the address as upper case colon separated pairs, or ""
// if it is not a six byte MAC. macOS prints octets without a leading zero, so
// single character groups are padded.
func normalizeMAC(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	sep := ":"
	if !strings.Contains(s, ":") && strings.Contains(s, "-") {
		sep = "-"
	}
	parts := strings.Split(s, sep)
	if len(parts) != 6 {
		return ""
	}
	out := make([]string, 6)
	for i, p := range parts {
		if len(p) == 0 || len(p) > 2 {
			return ""
		}
		for _, c := range p {
			if !isHex(c) {
				return ""
			}
		}
		if len(p) == 1 {
			p = "0" + p
		}
		out[i] = strings.ToUpper(p)
	}
	return strings.Join(out, ":")
}

func isHex(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// usableMAC drops the entries that never identify a real device: an empty or
// incomplete cache line, the broadcast address and IPv4/IPv6 multicast.
func usableMAC(mac string) bool {
	switch {
	case mac == "":
		return false
	case mac == "00:00:00:00:00:00", mac == "FF:FF:FF:FF:FF:FF":
		return false
	case strings.HasPrefix(mac, "01:00:5E"), strings.HasPrefix(mac, "33:33"):
		return false
	}
	return true
}

// parseProcNetARP reads the Linux /proc/net/arp table. Entries with flag 0x0
// are incomplete lookups, not devices.
func parseProcNetARP(text string) []ARPEntry {
	var out []ARPEntry
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[0] == "IP" {
			continue
		}
		if f[2] == "0x0" {
			continue
		}
		out = appendEntry(out, f[0], f[3])
	}
	return out
}

// parseIPNeigh reads "ip neigh" output, the modern replacement for
// /proc/net/arp. IPv6 neighbours are ignored; this scanner is IPv4 only.
func parseIPNeigh(text string) []ARPEntry {
	var out []ARPEntry
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if strings.Contains(line, "FAILED") || strings.Contains(line, "INCOMPLETE") {
			continue
		}
		mac := ""
		for i, tok := range f {
			if tok == "lladdr" && i+1 < len(f) {
				mac = f[i+1]
				break
			}
		}
		out = appendEntry(out, f[0], mac)
	}
	return out
}

// parseARPCommand reads "arp -a" output. Windows, macOS and the Linux
// net-tools build all print a different layout, so instead of three parsers
// each line is scanned for the first address and the first MAC it contains.
//
//	Windows   192.168.1.1           aa-bb-cc-dd-ee-ff     dynamic
//	macOS     ? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]
//	Linux     router (192.168.1.1) at aa:bb:cc:dd:ee:ff [ether] on eth0
func parseARPCommand(text string) []ARPEntry {
	var out []ARPEntry
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Interface:") {
			continue
		}
		var ip, mac string
		for _, tok := range strings.Fields(line) {
			tok = strings.Trim(tok, "(),")
			if ip == "" {
				if a, err := netip.ParseAddr(tok); err == nil && a.Is4() {
					ip = tok
					continue
				}
			}
			if mac == "" {
				mac = normalizeMAC(tok)
			}
		}
		out = appendEntry(out, ip, mac)
	}
	return out
}

func appendEntry(out []ARPEntry, ip, mac string) []ARPEntry {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !addr.Is4() {
		return out
	}
	m := normalizeMAC(mac)
	if !usableMAC(m) {
		return out
	}
	return append(out, ARPEntry{IP: addr.String(), MAC: m})
}

func runARPCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, arpCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
