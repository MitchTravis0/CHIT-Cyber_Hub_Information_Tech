// Package subnet does IPv4 and basic IPv6 subnet arithmetic: network and
// broadcast addresses, usable host ranges, masks, address scope and splitting.
//
// Everything works on netip.Prefix. A prefix parsed here keeps the host bits
// the user typed (192.168.1.10/24 stays 192.168.1.10/24), so callers can show
// both the address and its network; call Masked() for the network itself.
//
// Nothing in the app calls this package yet: the Subnet Calculator recomputes
// on every keystroke, so its arithmetic runs in the frontend
// (frontend/src/tools/subnet-calculator/subnet.ts). This package is the second,
// independent implementation that copy is checked against, and Info's JSON tags
// are its field names, so a tool that needs subnet maths in the backend can bind
// Calculate as it stands. testdata/subnet-cases.json is what holds the two in
// step; see golden_test.go.
package subnet

import (
	"math/big"
	"math/bits"
	"net/netip"
	"strconv"
	"strings"

	"chit/internal/core"
)

// Info is the full description of one address and its network, shaped for the
// UI. Counts are strings because an IPv6 prefix holds more addresses than a
// float64 (or an int64) can represent.
type Info struct {
	Input          string `json:"input"`
	Version        int    `json:"version"`
	Address        string `json:"address"`
	CIDR           string `json:"cidr"`
	PrefixLength   int    `json:"prefixLength"`
	Netmask        string `json:"netmask"`
	Wildcard       string `json:"wildcard"`
	Network        string `json:"network"`
	Broadcast      string `json:"broadcast"`
	FirstHost      string `json:"firstHost"`
	LastHost       string `json:"lastHost"`
	HostBits       int    `json:"hostBits"`
	TotalAddresses string `json:"totalAddresses"`
	UsableHosts    string `json:"usableHosts"`
	Class          string `json:"class"`
	Scope          string `json:"scope"`
	Private        bool   `json:"private"`
	Loopback       bool   `json:"loopback"`
	LinkLocal      bool   `json:"linkLocal"`
	CGNAT          bool   `json:"cgnat"`
	Note           string `json:"note"`
}

// Calculate parses input and describes it in one step.
func Calculate(input string) (Info, error) {
	p, err := Parse(input)
	if err != nil {
		return Info{}, err
	}
	info := Describe(p)
	info.Input = strings.TrimSpace(input)
	return info, nil
}

// Parse accepts the forms a tech actually types:
//
//	192.168.1.10/24            address and prefix length
//	192.168.1.10 255.255.255.0 address and dotted-decimal mask
//	192.168.1.10/255.255.255.0 the same, slash separated
//	192.168.1.10               a single host (/32)
//	2001:db8::1/64             IPv6
func Parse(input string) (netip.Prefix, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return netip.Prefix{}, core.Errorf(core.CodeInvalidInput, "Enter an IP address, for example 192.168.1.10/24.")
	}

	host, mask := s, ""
	if i := strings.IndexAny(s, "/ \t"); i >= 0 {
		host, mask = s[:i], strings.TrimSpace(s[i+1:])
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Prefix{}, core.Errorf(core.CodeInvalidInput, "%q is not a valid IP address. Try something like 192.168.1.10/24.", host)
	}
	// Zones and the ::ffff:1.2.3.4 form are not networks of their own, so they
	// are reduced to the address the mask applies to.
	addr = addr.WithZone("").Unmap()

	maxBits := addr.BitLen()
	if mask == "" {
		return netip.PrefixFrom(addr, maxBits), nil
	}

	if strings.ContainsAny(mask, ".:") {
		bits, err := maskBits(mask)
		if err != nil {
			return netip.Prefix{}, err
		}
		if !addr.Is4() {
			return netip.Prefix{}, core.Errorf(core.CodeInvalidInput, "IPv6 networks use a prefix length such as /64, not a mask like %s.", mask)
		}
		return netip.PrefixFrom(addr, bits), nil
	}

	n, err := strconv.Atoi(mask)
	if err != nil || n < 0 || n > maxBits {
		version := 4
		if addr.Is6() {
			version = 6
		}
		return netip.Prefix{}, core.Errorf(core.CodeInvalidInput, "%q is not a valid prefix length. For IPv%d it must be between /0 and /%d.", mask, version, maxBits)
	}
	return netip.PrefixFrom(addr, n), nil
}

// maskBits converts a dotted-decimal mask to a prefix length.
func maskBits(mask string) (int, error) {
	m, err := netip.ParseAddr(mask)
	if err != nil || !m.Is4() {
		return 0, core.Errorf(core.CodeInvalidInput, "%q is not a valid subnet mask. Use something like 255.255.255.0.", mask)
	}
	b := m.As4()
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	ones := bits.LeadingZeros32(^v)
	if v != ^uint32(0)<<(32-ones) {
		return 0, core.Errorf(core.CodeInvalidInput, "%s is not a valid subnet mask. A mask is a run of 1 bits followed by 0 bits, like 255.255.240.0.", mask)
	}
	return ones, nil
}

// Describe fills in every field of Info for p.
func Describe(p netip.Prefix) Info {
	network := p.Masked()
	addr := p.Addr()
	info := Info{
		Input:          p.String(),
		Version:        4,
		Address:        addr.String(),
		CIDR:           network.String(),
		PrefixLength:   p.Bits(),
		Network:        network.Addr().String(),
		HostBits:       addr.BitLen() - p.Bits(),
		TotalAddresses: TotalAddresses(p).String(),
		UsableHosts:    UsableHosts(p).String(),
	}
	if addr.Is6() {
		info.Version = 6
	} else {
		info.Netmask = Netmask(p.Bits()).String()
		info.Wildcard = Wildcard(p.Bits()).String()
		info.Class = Class(addr)
	}
	if b, ok := Broadcast(p); ok {
		info.Broadcast = b.String()
	}
	info.FirstHost = FirstHost(p).String()
	info.LastHost = LastHost(p).String()
	info.Scope, info.Private, info.Loopback, info.LinkLocal, info.CGNAT = scopeOf(addr)
	info.Note = noteFor(p)
	return info
}

// LastAddress is the highest address in the prefix (the broadcast address on
// an ordinary IPv4 network).
func LastAddress(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().AsSlice()
	for i := range b {
		switch high := i * 8; {
		case high >= p.Bits():
			b[i] = 0xff
		case p.Bits()-high < 8:
			b[i] |= 0xff >> (p.Bits() - high)
		}
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

// Broadcast reports the broadcast address. There is none on an IPv6 network,
// on a /31 (RFC 3021) or on a /32.
func Broadcast(p netip.Prefix) (netip.Addr, bool) {
	if !p.Addr().Is4() || p.Bits() >= 31 {
		return netip.Addr{}, false
	}
	return LastAddress(p), true
}

// FirstHost is the lowest address a host can use. On a /31 and a /32, and on
// every IPv6 network, that is the first address in the range itself.
func FirstHost(p netip.Prefix) netip.Addr {
	network := p.Masked().Addr()
	if !p.Addr().Is4() || p.Bits() >= 31 {
		return network
	}
	return network.Next()
}

// LastHost is the highest address a host can use.
func LastHost(p netip.Prefix) netip.Addr {
	last := LastAddress(p)
	if !p.Addr().Is4() || p.Bits() >= 31 {
		return last
	}
	return last.Prev()
}

// TotalAddresses is every address in the range, network and broadcast included.
func TotalAddresses(p netip.Prefix) *big.Int {
	return new(big.Int).Lsh(big.NewInt(1), uint(p.Addr().BitLen()-p.Bits()))
}

// UsableHosts is how many addresses can be assigned to a device. IPv4 networks
// of /30 and wider lose the network and broadcast addresses; a /31 is a
// point-to-point link with both addresses usable (RFC 3021) and a /32 is a
// single host. IPv6 has no broadcast address, so nothing is subtracted.
func UsableHosts(p netip.Prefix) *big.Int {
	total := TotalAddresses(p)
	if p.Addr().Is4() && p.Bits() <= 30 {
		return total.Sub(total, big.NewInt(2))
	}
	return total
}

// Netmask renders an IPv4 prefix length as a dotted-decimal mask.
func Netmask(prefixLen int) netip.Addr {
	v := ^uint32(0) << (32 - prefixLen)
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// Wildcard renders an IPv4 prefix length as the inverse mask used in ACLs.
func Wildcard(prefixLen int) netip.Addr {
	v := ^(^uint32(0) << (32 - prefixLen))
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// Class is the legacy IPv4 class letter. Classful routing is long dead but the
// letters still turn up in vendor documentation.
func Class(addr netip.Addr) string {
	if !addr.Is4() {
		return ""
	}
	switch first := addr.As4()[0]; {
	case first < 128:
		return "A"
	case first < 192:
		return "B"
	case first < 224:
		return "C"
	case first < 240:
		return "D (multicast)"
	default:
		return "E (reserved)"
	}
}

type scopeRule struct {
	prefix                              netip.Prefix
	label                               string
	private, loopback, linkLocal, cgnat bool
}

// Most specific first: the first match wins.
var scopeRules = []scopeRule{
	{netip.MustParsePrefix("0.0.0.0/8"), "This network (RFC 1122)", false, false, false, false},
	{netip.MustParsePrefix("10.0.0.0/8"), "Private (RFC 1918)", true, false, false, false},
	{netip.MustParsePrefix("100.64.0.0/10"), "Carrier-grade NAT (RFC 6598)", false, false, false, true},
	{netip.MustParsePrefix("127.0.0.0/8"), "Loopback", false, true, false, false},
	{netip.MustParsePrefix("169.254.0.0/16"), "Link-local (APIPA)", false, false, true, false},
	{netip.MustParsePrefix("172.16.0.0/12"), "Private (RFC 1918)", true, false, false, false},
	{netip.MustParsePrefix("192.0.2.0/24"), "Documentation (TEST-NET-1)", false, false, false, false},
	{netip.MustParsePrefix("192.168.0.0/16"), "Private (RFC 1918)", true, false, false, false},
	{netip.MustParsePrefix("198.18.0.0/15"), "Benchmarking (RFC 2544)", false, false, false, false},
	{netip.MustParsePrefix("198.51.100.0/24"), "Documentation (TEST-NET-2)", false, false, false, false},
	{netip.MustParsePrefix("203.0.113.0/24"), "Documentation (TEST-NET-3)", false, false, false, false},
	{netip.MustParsePrefix("255.255.255.255/32"), "Limited broadcast", false, false, false, false},
	{netip.MustParsePrefix("224.0.0.0/4"), "Multicast", false, false, false, false},
	{netip.MustParsePrefix("240.0.0.0/4"), "Reserved (RFC 1112)", false, false, false, false},
	{netip.MustParsePrefix("::/128"), "Unspecified", false, false, false, false},
	{netip.MustParsePrefix("::1/128"), "Loopback", false, true, false, false},
	{netip.MustParsePrefix("2001:db8::/32"), "Documentation", false, false, false, false},
	{netip.MustParsePrefix("fc00::/7"), "Unique local (ULA)", true, false, false, false},
	{netip.MustParsePrefix("fe80::/10"), "Link-local", false, false, true, false},
	{netip.MustParsePrefix("ff00::/8"), "Multicast", false, false, false, false},
}

func scopeOf(addr netip.Addr) (label string, private, loopback, linkLocal, cgnat bool) {
	for _, r := range scopeRules {
		if r.prefix.Contains(addr) {
			return r.label, r.private, r.loopback, r.linkLocal, r.cgnat
		}
	}
	if addr.Is4() {
		return "Public", false, false, false, false
	}
	return "Global unicast", false, false, false, false
}

func noteFor(p netip.Prefix) string {
	if !p.Addr().Is4() {
		if p.Bits() == 128 {
			return "A /128 is one address, a single host."
		}
		return "IPv6 has no broadcast address, so every address in the range is usable."
	}
	switch p.Bits() {
	case 32:
		return "A /32 is one address, a single host route. There is no network or broadcast address."
	case 31:
		return "A /31 is a point-to-point link (RFC 3021). Both addresses are usable and there is no broadcast address."
	case 30:
		return "A /30 gives 2 usable addresses, the classic point-to-point subnet."
	}
	return ""
}
