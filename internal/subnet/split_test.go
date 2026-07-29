package subnet

import (
	"net/netip"
	"strconv"
	"testing"

	"chit/internal/core"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		newBits int
		want    []string // "cidr first-last broadcast usable"
	}{
		{
			name: "24 into 26s", input: "192.168.1.0/24", newBits: 26,
			want: []string{
				"192.168.1.0/26 192.168.1.1-192.168.1.62 192.168.1.63 62",
				"192.168.1.64/26 192.168.1.65-192.168.1.126 192.168.1.127 62",
				"192.168.1.128/26 192.168.1.129-192.168.1.190 192.168.1.191 62",
				"192.168.1.192/26 192.168.1.193-192.168.1.254 192.168.1.255 62",
			},
		},
		{
			name: "host bits in the input are masked off", input: "192.168.1.77/24", newBits: 25,
			want: []string{
				"192.168.1.0/25 192.168.1.1-192.168.1.126 192.168.1.127 126",
				"192.168.1.128/25 192.168.1.129-192.168.1.254 192.168.1.255 126",
			},
		},
		{
			name: "16 into 18s", input: "172.16.0.0/16", newBits: 18,
			want: []string{
				"172.16.0.0/18 172.16.0.1-172.16.63.254 172.16.63.255 16382",
				"172.16.64.0/18 172.16.64.1-172.16.127.254 172.16.127.255 16382",
				"172.16.128.0/18 172.16.128.1-172.16.191.254 172.16.191.255 16382",
				"172.16.192.0/18 172.16.192.1-172.16.255.254 172.16.255.255 16382",
			},
		},
		{
			name: "30 into 31s keeps both addresses usable", input: "10.0.0.0/30", newBits: 31,
			want: []string{
				"10.0.0.0/31 10.0.0.0-10.0.0.1  2",
				"10.0.0.2/31 10.0.0.2-10.0.0.3  2",
			},
		},
		{
			name: "31 into 32s", input: "10.0.0.4/31", newBits: 32,
			want: []string{
				"10.0.0.4/32 10.0.0.4-10.0.0.4  1",
				"10.0.0.5/32 10.0.0.5-10.0.0.5  1",
			},
		},
		{
			name: "same prefix returns the network itself", input: "10.0.0.0/24", newBits: 24,
			want: []string{"10.0.0.0/24 10.0.0.1-10.0.0.254 10.0.0.255 254"},
		},
		{
			name: "top of the address space does not wrap", input: "255.255.255.240/28", newBits: 30,
			want: []string{
				"255.255.255.240/30 255.255.255.241-255.255.255.242 255.255.255.243 2",
				"255.255.255.244/30 255.255.255.245-255.255.255.246 255.255.255.247 2",
				"255.255.255.248/30 255.255.255.249-255.255.255.250 255.255.255.251 2",
				"255.255.255.252/30 255.255.255.253-255.255.255.254 255.255.255.255 2",
			},
		},
		{
			name: "default route into halves", input: "0.0.0.0/0", newBits: 1,
			want: []string{
				"0.0.0.0/1 0.0.0.1-127.255.255.254 127.255.255.255 2147483646",
				"128.0.0.0/1 128.0.0.1-255.255.255.254 255.255.255.255 2147483646",
			},
		},
		{
			name: "ipv6 48 into 50s", input: "2001:db8::/48", newBits: 50,
			want: []string{
				"2001:db8::/50 2001:db8::-2001:db8:0:3fff:ffff:ffff:ffff:ffff  302231454903657293676544",
				"2001:db8:0:4000::/50 2001:db8:0:4000::-2001:db8:0:7fff:ffff:ffff:ffff:ffff  302231454903657293676544",
				"2001:db8:0:8000::/50 2001:db8:0:8000::-2001:db8:0:bfff:ffff:ffff:ffff:ffff  302231454903657293676544",
				"2001:db8:0:c000::/50 2001:db8:0:c000::-2001:db8:0:ffff:ffff:ffff:ffff:ffff  302231454903657293676544",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			subs, err := Split(p, tt.newBits)
			if err != nil {
				t.Fatalf("Split(%s, %d) failed: %v", tt.input, tt.newBits, err)
			}
			if len(subs) != len(tt.want) {
				t.Fatalf("Split(%s, %d) returned %d subnets, want %d", tt.input, tt.newBits, len(subs), len(tt.want))
			}
			for i, want := range tt.want {
				got := subs[i].CIDR + " " + subs[i].FirstHost + "-" + subs[i].LastHost + " " +
					subs[i].Broadcast + " " + subs[i].UsableHosts
				if got != want {
					t.Errorf("subnet %d = %q, want %q", i+1, got, want)
				}
				if subs[i].Index != i+1 {
					t.Errorf("subnet %d has index %d", i+1, subs[i].Index)
				}
			}
		})
	}
}

func TestSplitCountsAndContiguity(t *testing.T) {
	p := netip.MustParsePrefix("10.0.0.0/16")
	subs, err := Split(p, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 256 {
		t.Fatalf("got %d subnets, want 256", len(subs))
	}
	for i, s := range subs {
		want := netip.MustParseAddr("10.0." + strconv.Itoa(i) + ".0")
		if s.Network != want.String() {
			t.Fatalf("subnet %d network = %s, want %s", i+1, s.Network, want)
		}
		if s.Netmask != "255.255.255.0" {
			t.Errorf("subnet %d netmask = %s, want 255.255.255.0", i+1, s.Netmask)
		}
		if s.TotalAddresses != "256" || s.UsableHosts != "254" {
			t.Errorf("subnet %d counts = %s/%s, want 256/254", i+1, s.TotalAddresses, s.UsableHosts)
		}
	}
}

func TestSplitInto(t *testing.T) {
	tests := []struct {
		input     string
		count     int
		wantCount int
		wantBits  int
	}{
		{"192.168.0.0/24", 1, 1, 24},
		{"192.168.0.0/24", 2, 2, 25},
		{"192.168.0.0/24", 3, 4, 26}, // rounded up: equal subnets come in powers of two
		{"192.168.0.0/24", 4, 4, 26},
		{"192.168.0.0/24", 5, 8, 27},
		{"192.168.0.0/24", 8, 8, 27},
		{"10.0.0.0/8", 1000, 1024, 18},
		{"2001:db8::/32", 4, 4, 34},
	}

	for _, tt := range tests {
		p := netip.MustParsePrefix(tt.input)
		subs, err := SplitInto(p, tt.count)
		if err != nil {
			t.Fatalf("SplitInto(%s, %d) failed: %v", tt.input, tt.count, err)
		}
		if len(subs) != tt.wantCount {
			t.Errorf("SplitInto(%s, %d) returned %d subnets, want %d", tt.input, tt.count, len(subs), tt.wantCount)
		}
		got := netip.MustParsePrefix(subs[0].CIDR)
		if got.Bits() != tt.wantBits {
			t.Errorf("SplitInto(%s, %d) produced /%d, want /%d", tt.input, tt.count, got.Bits(), tt.wantBits)
		}
	}
}

func TestSplitInvalid(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		newBits int
	}{
		{"shorter than the network", "192.168.1.0/24", 16},
		{"longer than ipv4", "192.168.1.0/24", 33},
		{"negative", "192.168.1.0/24", -1},
		{"too many subnets", "10.0.0.0/8", 21},
		{"too many ipv6 subnets", "2001:db8::/32", 64},
		{"longer than ipv6", "2001:db8::/32", 129},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := netip.MustParsePrefix(tt.prefix)
			if subs, err := Split(p, tt.newBits); err == nil {
				t.Fatalf("Split(%s, %d) returned %d subnets, want an error", tt.prefix, tt.newBits, len(subs))
			} else if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("error code = %s, want %s", code, core.CodeInvalidInput)
			}
		})
	}
}

func TestSplitIntoInvalid(t *testing.T) {
	p := netip.MustParsePrefix("192.168.1.0/24")
	for _, count := range []int{0, -4, 8192} {
		if subs, err := SplitInto(p, count); err == nil {
			t.Errorf("SplitInto(%s, %d) returned %d subnets, want an error", p, count, len(subs))
		}
	}
	// A /24 cannot be cut into 512 pieces: that would need /33.
	if _, err := SplitInto(p, 512); err == nil {
		t.Error("SplitInto(/24, 512) should fail, /33 does not exist")
	}
}
