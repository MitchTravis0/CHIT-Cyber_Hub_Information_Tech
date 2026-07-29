// Package netscan discovers live hosts on an IPv4 range without needing
// administrator rights. See Sweep for the discovery chain.
package netscan

import (
	"encoding/binary"
	"net/netip"
	"strings"

	"chit/internal/core"
)

// MaxAddresses caps one sweep at a /16. Anything larger is a mistake far more
// often than an intention, and it would take hours.
const MaxAddresses = 65536

// Range is a contiguous, inclusive block of IPv4 addresses to scan.
type Range struct {
	Start netip.Addr `json:"start"`
	End   netip.Addr `json:"end"`
	Count int        `json:"count"`
	Text  string     `json:"text"`
}

// ParseRange accepts the four shapes a tech is likely to type:
//
//	192.168.1.0/24              CIDR
//	192.168.1.1-192.168.1.254   start and end
//	192.168.1.1-254             start and a short end (last octet only)
//	192.168.1.50                a single address
//
// For a CIDR of /30 or wider the network and broadcast addresses are left out,
// so 192.168.1.0/24 means the 254 usable addresses.
func ParseRange(input string) (Range, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return Range{}, core.Errorf(core.CodeInvalidInput,
			"Type something to scan, for example 192.168.1.0/24 or 192.168.1.1-254.")
	}

	switch {
	case strings.Contains(s, "/"):
		return parseCIDR(s)
	case strings.Contains(s, "-"):
		return parseStartEnd(s)
	default:
		return parseSingle(s)
	}
}

// newRange builds the range and enforces the size cap. Counting in int64 keeps
// a /0 from wrapping on a 32 bit build.
func newRange(start, end netip.Addr, text string) (Range, error) {
	count := int64(toU32(end)) - int64(toU32(start)) + 1
	if count > MaxAddresses {
		return Range{}, core.Errorf(core.CodeInvalidInput,
			"That covers %d addresses. Scan at most %d at a time (a /16), for example 192.168.1.0/24.",
			count, MaxAddresses)
	}
	return Range{Start: start, End: end, Count: int(count), Text: text}, nil
}

func parseCIDR(s string) (Range, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return Range{}, core.Errorf(core.CodeInvalidInput,
			"%q is not a valid network. Use a form like 192.168.1.0/24.", s)
	}
	if !p.Addr().Is4() {
		return Range{}, errIPv4Only(s)
	}

	p = p.Masked()
	first := toU32(p.Addr())
	last := first | ^maskBits(p.Bits())

	// A /31 is a two-address point-to-point link and a /32 is one host, so
	// there is nothing to trim there.
	if p.Bits() <= 30 {
		first++
		last--
	}
	return newRange(fromU32(first), fromU32(last), p.String())
}

func parseStartEnd(s string) (Range, error) {
	rawStart, rawEnd, _ := strings.Cut(s, "-")
	start, err := parseV4(strings.TrimSpace(rawStart), s)
	if err != nil {
		return Range{}, err
	}

	rawEnd = strings.TrimSpace(rawEnd)
	if rawEnd == "" {
		return Range{}, core.Errorf(core.CodeInvalidInput,
			"%q is missing the end of the range. Try 192.168.1.1-254.", s)
	}

	var end netip.Addr
	if strings.Contains(rawEnd, ".") {
		if end, err = parseV4(rawEnd, s); err != nil {
			return Range{}, err
		}
	} else {
		octet, ok := parseOctet(rawEnd)
		if !ok {
			return Range{}, core.Errorf(core.CodeInvalidInput,
				"%q is not a valid end for the range. Use a last number from 0 to 255, or a full address.", rawEnd)
		}
		b := start.As4()
		b[3] = byte(octet)
		end = netip.AddrFrom4(b)
	}

	if toU32(end) < toU32(start) {
		return Range{}, core.Errorf(core.CodeInvalidInput,
			"%s comes before %s, so that range is backwards. Swap the two addresses.", end, start)
	}
	return newRange(start, end, start.String()+"-"+end.String())
}

func parseSingle(s string) (Range, error) {
	addr, err := parseV4(s, s)
	if err != nil {
		return Range{}, err
	}
	return Range{Start: addr, End: addr, Count: 1, Text: addr.String()}, nil
}

// parseOctet takes the canonical decimal form only. strconv.Atoi would accept
// "+5" and "0255", neither of which netip would accept inside an address, and
// the frontend copy of this parser rejects both.
func parseOctet(s string) (int, bool) {
	if s == "" || len(s) > 3 || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	if n > 255 {
		return 0, false
	}
	return n, true
}

func parseV4(s, input string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, core.Errorf(core.CodeInvalidInput,
			"%q is not an address CHIT understands. Try 192.168.1.0/24, 192.168.1.1-254 or 192.168.1.50.", input)
	}
	if !addr.Is4() {
		return netip.Addr{}, errIPv4Only(input)
	}
	return addr, nil
}

func errIPv4Only(input string) error {
	return core.Errorf(core.CodeInvalidInput,
		"%q looks like IPv6. The scanner works on IPv4 ranges such as 192.168.1.0/24.", input)
}

// Addresses lists every address in the range, in order.
func (r Range) Addresses() []netip.Addr {
	if !r.Start.IsValid() || !r.End.IsValid() || r.Count <= 0 {
		return nil
	}
	first, last := toU32(r.Start), toU32(r.End)
	out := make([]netip.Addr, 0, r.Count)
	for v := first; ; v++ {
		out = append(out, fromU32(v))
		if v == last {
			break
		}
	}
	return out
}

func maskBits(bits int) uint32 {
	if bits == 0 {
		return 0
	}
	return ^uint32(0) << (32 - bits)
}

func toU32(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

func fromU32(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}
