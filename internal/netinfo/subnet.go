package netinfo

import (
	"fmt"
	"net"
	"strings"
)

// maskString renders an IPv4 mask the way Windows and macOS show it,
// e.g. 255.255.255.0. Non-IPv4 masks render as an empty string.
func maskString(mask net.IPMask) string {
	if len(mask) != net.IPv4len {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}

// prefixLen returns the CIDR prefix of a contiguous mask, or -1 if the mask is
// not contiguous (which no sane adapter reports, but the API allows).
func prefixLen(mask net.IPMask) int {
	ones, bits := mask.Size()
	if bits == 0 {
		return -1
	}
	return ones
}

// networkAddress zeroes the host bits of ip.
func networkAddress(ip net.IP, mask net.IPMask) net.IP {
	ip = normalise(ip)
	if ip == nil || len(ip) != len(mask) {
		return nil
	}
	out := make(net.IP, len(ip))
	for i := range ip {
		out[i] = ip[i] & mask[i]
	}
	return out
}

// broadcastAddress is IPv4 only: IPv6 has no broadcast.
func broadcastAddress(ip net.IP, mask net.IPMask) net.IP {
	network := networkAddress(ip, mask)
	if network == nil || len(network) != net.IPv4len {
		return nil
	}
	out := make(net.IP, net.IPv4len)
	for i := range network {
		out[i] = network[i] | ^mask[i]
	}
	return out
}

// hostRange returns the first and last usable IPv4 host in the subnet. A /31 or
// /32 has no usable host pair, so both come back empty.
func hostRange(ip net.IP, mask net.IPMask) (string, string) {
	network := networkAddress(ip, mask)
	broadcast := broadcastAddress(ip, mask)
	if network == nil || broadcast == nil || prefixLen(mask) > 30 {
		return "", ""
	}
	first := make(net.IP, net.IPv4len)
	last := make(net.IP, net.IPv4len)
	copy(first, network)
	copy(last, broadcast)
	first[3]++
	last[3]--
	return first.String(), last.String()
}

// normalise converts a 16-byte IPv4-mapped address to its 4-byte form so that
// it can be masked against a 4-byte mask.
func normalise(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip.To16()
}

// ipv6Scope labels an IPv6 address the way a tech would describe it.
func ipv6Scope(ip net.IP) string {
	switch {
	case ip.IsLoopback():
		return "loopback"
	case ip.IsLinkLocalUnicast():
		return "link-local"
	case len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc:
		return "unique-local"
	case ip.IsGlobalUnicast():
		return "global"
	default:
		return "other"
	}
}

// usableIPv4 reports whether an address is one a tech would act on: not
// loopback, not a 169.254.x.x APIPA self-assignment.
func usableIPv4(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && !v4.IsLoopback() && !v4.IsLinkLocalUnicast() && !v4.IsUnspecified()
}

// hexToIPv4 decodes the little-endian hex words /proc/net/route uses,
// e.g. "0101A8C0" is 192.168.1.1.
func hexToIPv4(s string) net.IP {
	if len(s) != 8 {
		return nil
	}
	var b [4]byte
	for i := 0; i < 4; i++ {
		var v int
		if _, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &v); err != nil {
			return nil
		}
		b[3-i] = byte(v)
	}
	return net.IPv4(b[0], b[1], b[2], b[3]).To4()
}

// macString formats a hardware address as upper case colon-separated pairs,
// which is what switch and DHCP consoles show.
func macString(hw net.HardwareAddr) string {
	if len(hw) == 0 {
		return ""
	}
	return strings.ToUpper(hw.String())
}
