package subnet

import (
	"net/netip"
	"strconv"
	"testing"

	"chit/internal/core"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // prefix as typed, host bits kept
	}{
		{"cidr", "192.168.1.10/24", "192.168.1.10/24"},
		{"cidr network address", "10.0.0.0/8", "10.0.0.0/8"},
		{"space separated mask", "192.168.1.10 255.255.255.0", "192.168.1.10/24"},
		{"slash separated mask", "192.168.1.10/255.255.128.0", "192.168.1.10/17"},
		{"tab separated mask", "192.168.1.10\t255.255.255.252", "192.168.1.10/30"},
		{"extra whitespace", "  192.168.1.10   255.255.0.0  ", "192.168.1.10/16"},
		{"bare address is a host route", "8.8.8.8", "8.8.8.8/32"},
		{"mask 0.0.0.0", "1.2.3.4 0.0.0.0", "1.2.3.4/0"},
		{"mask 255.255.255.255", "1.2.3.4 255.255.255.255", "1.2.3.4/32"},
		{"prefix 0", "1.2.3.4/0", "1.2.3.4/0"},
		{"prefix 31", "10.1.1.4/31", "10.1.1.4/31"},
		{"ipv6 cidr", "2001:db8::1/64", "2001:db8::1/64"},
		{"bare ipv6 is a host route", "2001:db8::1", "2001:db8::1/128"},
		{"ipv6 zone is dropped", "fe80::1%eth0/10", "fe80::1/10"},
		{"ipv4-mapped ipv6 is unmapped", "::ffff:192.168.1.10/24", "192.168.1.10/24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tt.input, err)
			}
			if got.String() != tt.want {
				t.Errorf("Parse(%q) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct{ name, input string }{
		{"empty", ""},
		{"whitespace only", "   "},
		{"not an address", "hello"},
		{"octet out of range", "192.168.1.256/24"},
		{"three octets", "192.168.1/24"},
		{"leading zeros", "010.1.1.1/24"},
		{"prefix too long for ipv4", "192.168.1.1/33"},
		{"negative prefix", "192.168.1.1/-1"},
		{"prefix not a number", "192.168.1.1/abc"},
		{"prefix too long for ipv6", "2001:db8::1/129"},
		{"non-contiguous mask", "192.168.1.1 255.0.255.0"},
		{"non-contiguous mask high bits", "192.168.1.1 255.255.0.255"},
		{"mask octet out of range", "192.168.1.1 255.255.256.0"},
		{"dotted mask on ipv6", "2001:db8::1 255.255.255.0"},
		{"trailing junk", "192.168.1.1/24 please"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(tt.input)
			if err == nil {
				t.Fatalf("Parse(%q) = %s, want an error", tt.input, p)
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("Parse(%q) error code = %s, want %s", tt.input, code, core.CodeInvalidInput)
			}
			if msg := core.MessageOf(err); msg == "" {
				t.Errorf("Parse(%q) returned an empty message", tt.input)
			}
		})
	}
}

// Values here are the ones a published subnet calculator prints for the same
// input, so a regression shows up as a mismatch rather than as a plausible
// looking number.
func TestDescribe(t *testing.T) {
	tests := []struct {
		input     string
		network   string
		netmask   string
		wildcard  string
		broadcast string
		firstHost string
		lastHost  string
		total     string
		usable    string
		hostBits  int
		class     string
		scope     string
		private   bool
	}{
		{
			input: "192.168.1.10/24", network: "192.168.1.0", netmask: "255.255.255.0",
			wildcard: "0.0.0.255", broadcast: "192.168.1.255", firstHost: "192.168.1.1",
			lastHost: "192.168.1.254", total: "256", usable: "254", hostBits: 8,
			class: "C", scope: "Private (RFC 1918)", private: true,
		},
		{
			input: "172.16.5.1/16", network: "172.16.0.0", netmask: "255.255.0.0",
			wildcard: "0.0.255.255", broadcast: "172.16.255.255", firstHost: "172.16.0.1",
			lastHost: "172.16.255.254", total: "65536", usable: "65534", hostBits: 16,
			class: "B", scope: "Private (RFC 1918)", private: true,
		},
		{
			input: "10.20.30.40/8", network: "10.0.0.0", netmask: "255.0.0.0",
			wildcard: "0.255.255.255", broadcast: "10.255.255.255", firstHost: "10.0.0.1",
			lastHost: "10.255.255.254", total: "16777216", usable: "16777214", hostBits: 24,
			class: "A", scope: "Private (RFC 1918)", private: true,
		},
		{
			input: "192.168.1.10/30", network: "192.168.1.8", netmask: "255.255.255.252",
			wildcard: "0.0.0.3", broadcast: "192.168.1.11", firstHost: "192.168.1.9",
			lastHost: "192.168.1.10", total: "4", usable: "2", hostBits: 2,
			class: "C", scope: "Private (RFC 1918)", private: true,
		},
		{
			// RFC 3021: both addresses usable, no broadcast.
			input: "10.1.1.5/31", network: "10.1.1.4", netmask: "255.255.255.254",
			wildcard: "0.0.0.1", broadcast: "", firstHost: "10.1.1.4",
			lastHost: "10.1.1.5", total: "2", usable: "2", hostBits: 1,
			class: "A", scope: "Private (RFC 1918)", private: true,
		},
		{
			input: "8.8.8.8/32", network: "8.8.8.8", netmask: "255.255.255.255",
			wildcard: "0.0.0.0", broadcast: "", firstHost: "8.8.8.8",
			lastHost: "8.8.8.8", total: "1", usable: "1", hostBits: 0,
			class: "A", scope: "Public",
		},
		{
			input: "0.0.0.0/0", network: "0.0.0.0", netmask: "0.0.0.0",
			wildcard: "255.255.255.255", broadcast: "255.255.255.255", firstHost: "0.0.0.1",
			lastHost: "255.255.255.254", total: "4294967296", usable: "4294967294", hostBits: 32,
			class: "A", scope: "This network (RFC 1122)",
		},
		{
			// Boundary: the last address of the RFC 1918 172.16/12 block.
			input: "172.31.255.254/12", network: "172.16.0.0", netmask: "255.240.0.0",
			wildcard: "0.15.255.255", broadcast: "172.31.255.255", firstHost: "172.16.0.1",
			lastHost: "172.31.255.254", total: "1048576", usable: "1048574", hostBits: 20,
			class: "B", scope: "Private (RFC 1918)", private: true,
		},
		{
			// Boundary: one address past that block is public.
			input: "172.32.0.1/12", network: "172.32.0.0", netmask: "255.240.0.0",
			wildcard: "0.15.255.255", broadcast: "172.47.255.255", firstHost: "172.32.0.1",
			lastHost: "172.47.255.254", total: "1048576", usable: "1048574", hostBits: 20,
			class: "B", scope: "Public",
		},
		{
			input: "169.254.13.7/16", network: "169.254.0.0", netmask: "255.255.0.0",
			wildcard: "0.0.255.255", broadcast: "169.254.255.255", firstHost: "169.254.0.1",
			lastHost: "169.254.255.254", total: "65536", usable: "65534", hostBits: 16,
			class: "B", scope: "Link-local (APIPA)",
		},
		{
			input: "100.64.0.1/10", network: "100.64.0.0", netmask: "255.192.0.0",
			wildcard: "0.63.255.255", broadcast: "100.127.255.255", firstHost: "100.64.0.1",
			lastHost: "100.127.255.254", total: "4194304", usable: "4194302", hostBits: 22,
			class: "A", scope: "Carrier-grade NAT (RFC 6598)",
		},
		{
			input: "127.0.0.1/8", network: "127.0.0.0", netmask: "255.0.0.0",
			wildcard: "0.255.255.255", broadcast: "127.255.255.255", firstHost: "127.0.0.1",
			lastHost: "127.255.255.254", total: "16777216", usable: "16777214", hostBits: 24,
			class: "A", scope: "Loopback",
		},
		{
			input: "203.0.113.5/24", network: "203.0.113.0", netmask: "255.255.255.0",
			wildcard: "0.0.0.255", broadcast: "203.0.113.255", firstHost: "203.0.113.1",
			lastHost: "203.0.113.254", total: "256", usable: "254", hostBits: 8,
			class: "C", scope: "Documentation (TEST-NET-3)",
		},
		{
			input: "239.1.2.3/32", network: "239.1.2.3", netmask: "255.255.255.255",
			wildcard: "0.0.0.0", broadcast: "", firstHost: "239.1.2.3",
			lastHost: "239.1.2.3", total: "1", usable: "1", hostBits: 0,
			class: "D (multicast)", scope: "Multicast",
		},
		{
			// IPv6 has no broadcast, so every address counts as usable.
			input: "2001:db8::1/64", network: "2001:db8::", netmask: "", wildcard: "",
			broadcast: "", firstHost: "2001:db8::", lastHost: "2001:db8::ffff:ffff:ffff:ffff",
			total: "18446744073709551616", usable: "18446744073709551616", hostBits: 64,
			class: "", scope: "Documentation",
		},
		{
			input: "fe80::1234/10", network: "fe80::", netmask: "", wildcard: "",
			broadcast: "", firstHost: "fe80::", lastHost: "febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
			total: "332306998946228968225951765070086144", usable: "332306998946228968225951765070086144",
			hostBits: 118, class: "", scope: "Link-local",
		},
		{
			input: "fd00:1234::5/128", network: "fd00:1234::5", netmask: "", wildcard: "",
			broadcast: "", firstHost: "fd00:1234::5", lastHost: "fd00:1234::5",
			total: "1", usable: "1", hostBits: 0, class: "", scope: "Unique local (ULA)",
			private: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Calculate(tt.input)
			if err != nil {
				t.Fatalf("Calculate(%q) failed: %v", tt.input, err)
			}
			check := func(field, got, want string) {
				t.Helper()
				if got != want {
					t.Errorf("%s = %q, want %q", field, got, want)
				}
			}
			check("network", got.Network, tt.network)
			check("netmask", got.Netmask, tt.netmask)
			check("wildcard", got.Wildcard, tt.wildcard)
			check("broadcast", got.Broadcast, tt.broadcast)
			check("firstHost", got.FirstHost, tt.firstHost)
			check("lastHost", got.LastHost, tt.lastHost)
			check("totalAddresses", got.TotalAddresses, tt.total)
			check("usableHosts", got.UsableHosts, tt.usable)
			check("class", got.Class, tt.class)
			check("scope", got.Scope, tt.scope)
			if got.HostBits != tt.hostBits {
				t.Errorf("hostBits = %d, want %d", got.HostBits, tt.hostBits)
			}
			if got.Private != tt.private {
				t.Errorf("private = %v, want %v", got.Private, tt.private)
			}
			if got.Input != tt.input {
				t.Errorf("input = %q, want %q", got.Input, tt.input)
			}
		})
	}
}

func TestDescribeFlags(t *testing.T) {
	tests := []struct {
		input                               string
		private, loopback, linkLocal, cgnat bool
	}{
		{"10.0.0.1/8", true, false, false, false},
		{"192.168.0.1/16", true, false, false, false},
		{"192.167.255.255/16", false, false, false, false},
		{"192.169.0.1/16", false, false, false, false},
		{"9.255.255.255/8", false, false, false, false},
		{"11.0.0.0/8", false, false, false, false},
		{"127.255.255.254/8", false, true, false, false},
		{"128.0.0.1/8", false, false, false, false},
		{"169.254.0.0/16", false, false, true, false},
		{"169.253.255.255/16", false, false, false, false},
		{"100.63.255.255/10", false, false, false, false},
		{"100.127.255.255/10", false, false, false, true},
		{"100.128.0.0/10", false, false, false, false},
		{"::1/128", false, true, false, false},
		{"fe80::1/64", false, false, true, false},
		{"fc00::1/7", true, false, false, false},
		{"2606:4700::1111/32", false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Calculate(tt.input)
			if err != nil {
				t.Fatalf("Calculate(%q) failed: %v", tt.input, err)
			}
			if got.Private != tt.private || got.Loopback != tt.loopback ||
				got.LinkLocal != tt.linkLocal || got.CGNAT != tt.cgnat {
				t.Errorf("flags = private:%v loopback:%v linkLocal:%v cgnat:%v, want private:%v loopback:%v linkLocal:%v cgnat:%v",
					got.Private, got.Loopback, got.LinkLocal, got.CGNAT,
					tt.private, tt.loopback, tt.linkLocal, tt.cgnat)
			}
		})
	}
}

func TestDescribeVersionAndCIDR(t *testing.T) {
	got, err := Calculate("192.168.1.10 255.255.255.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 4 {
		t.Errorf("version = %d, want 4", got.Version)
	}
	if got.Address != "192.168.1.10" {
		t.Errorf("address = %q, want 192.168.1.10", got.Address)
	}
	if got.CIDR != "192.168.1.0/24" {
		t.Errorf("cidr = %q, want 192.168.1.0/24", got.CIDR)
	}
	if got.PrefixLength != 24 {
		t.Errorf("prefixLength = %d, want 24", got.PrefixLength)
	}

	v6, err := Calculate("2001:db8::1/64")
	if err != nil {
		t.Fatal(err)
	}
	if v6.Version != 6 {
		t.Errorf("version = %d, want 6", v6.Version)
	}
	if v6.CIDR != "2001:db8::/64" {
		t.Errorf("cidr = %q, want 2001:db8::/64", v6.CIDR)
	}
}

func TestNote(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"10.0.0.1/24", ""},
		{"10.0.0.1/30", "A /30 gives 2 usable addresses, the classic point-to-point subnet."},
		{"10.0.0.1/31", "A /31 is a point-to-point link (RFC 3021). Both addresses are usable and there is no broadcast address."},
		{"10.0.0.1/32", "A /32 is one address, a single host route. There is no network or broadcast address."},
	} {
		got, err := Calculate(tt.input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Note != tt.want {
			t.Errorf("Calculate(%q).Note = %q, want %q", tt.input, got.Note, tt.want)
		}
	}
}

func TestNetmaskAndWildcard(t *testing.T) {
	for _, tt := range []struct {
		bits     int
		netmask  string
		wildcard string
	}{
		{0, "0.0.0.0", "255.255.255.255"},
		{1, "128.0.0.0", "127.255.255.255"},
		{8, "255.0.0.0", "0.255.255.255"},
		{12, "255.240.0.0", "0.15.255.255"},
		{19, "255.255.224.0", "0.0.31.255"},
		{24, "255.255.255.0", "0.0.0.255"},
		{25, "255.255.255.128", "0.0.0.127"},
		{30, "255.255.255.252", "0.0.0.3"},
		{31, "255.255.255.254", "0.0.0.1"},
		{32, "255.255.255.255", "0.0.0.0"},
	} {
		if got := Netmask(tt.bits).String(); got != tt.netmask {
			t.Errorf("Netmask(%d) = %s, want %s", tt.bits, got, tt.netmask)
		}
		if got := Wildcard(tt.bits).String(); got != tt.wildcard {
			t.Errorf("Wildcard(%d) = %s, want %s", tt.bits, got, tt.wildcard)
		}
	}
}

// Every dotted mask must survive the round trip back to its prefix length.
func TestMaskRoundTrip(t *testing.T) {
	for bits := 0; bits <= 32; bits++ {
		mask := Netmask(bits).String()
		p, err := Parse("10.0.0.1 " + mask)
		if err != nil {
			t.Fatalf("Parse(10.0.0.1 %s) failed: %v", mask, err)
		}
		if p.Bits() != bits {
			t.Errorf("Parse(10.0.0.1 %s).Bits() = %d, want %d", mask, p.Bits(), bits)
		}
	}
}

func TestBroadcastAbsent(t *testing.T) {
	for _, input := range []string{"10.0.0.1/31", "10.0.0.1/32", "2001:db8::1/64", "2001:db8::1/128"} {
		p := netip.MustParsePrefix(input)
		if addr, ok := Broadcast(p); ok {
			t.Errorf("Broadcast(%s) = %s, want none", input, addr)
		}
	}
}

func TestHostCountsAcrossAllIPv4Prefixes(t *testing.T) {
	for bits := 0; bits <= 32; bits++ {
		p := netip.MustParsePrefix("10.0.0.0/" + strconv.Itoa(bits))
		total := TotalAddresses(p).Uint64()
		usable := UsableHosts(p).Uint64()
		want := uint64(1) << (32 - bits)
		if total != want {
			t.Errorf("TotalAddresses(/%d) = %d, want %d", bits, total, want)
		}
		switch {
		case bits >= 31:
			if usable != want {
				t.Errorf("UsableHosts(/%d) = %d, want %d", bits, usable, want)
			}
		default:
			if usable != want-2 {
				t.Errorf("UsableHosts(/%d) = %d, want %d", bits, usable, want-2)
			}
		}
	}
}
