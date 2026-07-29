package netinfo

import (
	"net"
	"testing"
)

func TestNewIPv4(t *testing.T) {
	tests := []struct {
		ip        string
		prefix    int
		wantMask  string
		wantNet   string
		wantBcast string
	}{
		{"192.168.1.244", 24, "255.255.255.0", "192.168.1.0/24", "192.168.1.255"},
		{"10.4.7.9", 8, "255.0.0.0", "10.0.0.0/8", "10.255.255.255"},
		{"172.20.30.40", 22, "255.255.252.0", "172.20.28.0/22", "172.20.31.255"},
		{"169.254.3.4", 16, "255.255.0.0", "169.254.0.0/16", "169.254.255.255"},
		{"203.0.113.7", 32, "255.255.255.255", "203.0.113.7/32", "203.0.113.7"},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip).To4()
		got := newIPv4(ip, net.CIDRMask(tt.prefix, 32))
		if got.Mask != tt.wantMask || got.Prefix != tt.prefix {
			t.Errorf("%s/%d: mask %q prefix %d, want %q %d", tt.ip, tt.prefix, got.Mask, got.Prefix, tt.wantMask, tt.prefix)
		}
		if got.Network != tt.wantNet {
			t.Errorf("%s/%d: network %q, want %q", tt.ip, tt.prefix, got.Network, tt.wantNet)
		}
		if got.Broadcast != tt.wantBcast {
			t.Errorf("%s/%d: broadcast %q, want %q", tt.ip, tt.prefix, got.Broadcast, tt.wantBcast)
		}
	}
}

// The kernel hands out IPv4 addresses as 16-byte values in places, and a
// 16-byte mask must not be mistaken for an IPv6 one.
func TestNewIPv4From16ByteMask(t *testing.T) {
	got := newIPv4(net.ParseIP("192.168.1.5"), net.IPMask(net.ParseIP("255.255.255.0")))
	if got.Prefix != 24 || got.Mask != "255.255.255.0" || got.CIDR != "192.168.1.5/24" {
		t.Errorf("got %+v, want a /24", got)
	}
}

func TestHostRange(t *testing.T) {
	tests := []struct {
		ip          string
		prefix      int
		first, last string
	}{
		{"192.168.1.244", 24, "192.168.1.1", "192.168.1.254"},
		{"10.0.0.5", 30, "10.0.0.5", "10.0.0.6"},
		{"10.0.0.5", 31, "", ""},
		{"10.0.0.5", 32, "", ""},
	}
	for _, tt := range tests {
		first, last := hostRange(net.ParseIP(tt.ip).To4(), net.CIDRMask(tt.prefix, 32))
		if first != tt.first || last != tt.last {
			t.Errorf("%s/%d: got %q-%q, want %q-%q", tt.ip, tt.prefix, first, last, tt.first, tt.last)
		}
	}
}

func TestIPv6Scope(t *testing.T) {
	tests := map[string]string{
		"::1":                  "loopback",
		"fe80::1c2b:3d4e:5f60": "link-local",
		"fd00:1234::1":         "unique-local",
		"2001:4860:4860::8888": "global",
		"ff02::1":              "other",
	}
	for in, want := range tests {
		if got := ipv6Scope(net.ParseIP(in)); got != want {
			t.Errorf("%s: scope %q, want %q", in, got, want)
		}
	}
}

func TestHexToIPv4(t *testing.T) {
	tests := map[string]string{
		"0101A8C0": "192.168.1.1",
		"00000000": "0.0.0.0",
		"FE01A8C0": "192.168.1.254",
		"short":    "",
		"":         "",
	}
	for in, want := range tests {
		ip := hexToIPv4(in)
		got := ""
		if ip != nil {
			got = ip.String()
		}
		if got != want {
			t.Errorf("hexToIPv4(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUsableIPv4(t *testing.T) {
	tests := map[string]bool{
		"192.168.1.10": true,
		"10.0.0.1":     true,
		"127.0.0.1":    false,
		"169.254.9.9":  false,
		"0.0.0.0":      false,
		"::1":          false,
	}
	for in, want := range tests {
		if got := usableIPv4(net.ParseIP(in)); got != want {
			t.Errorf("usableIPv4(%s) = %v, want %v", in, got, want)
		}
	}
}

func TestMacString(t *testing.T) {
	hw, err := net.ParseMAC("a4:5e:60:1b:2c:3d")
	if err != nil {
		t.Fatal(err)
	}
	if got := macString(hw); got != "A4:5E:60:1B:2C:3D" {
		t.Errorf("macString = %q, want upper case", got)
	}
	if got := macString(nil); got != "" {
		t.Errorf("macString(nil) = %q, want empty", got)
	}
}

func TestIsVirtual(t *testing.T) {
	tests := []struct {
		name, description string
		want              bool
	}{
		{"eth0", "", false},
		{"enp3s0", "", false},
		{"wlan0", "", false},
		{"en0", "", false},
		{"docker0", "", true},
		{"br-1a2b3c", "", true},
		{"veth9f21", "", true},
		{"utun3", "", true},
		{"Ethernet", "Intel(R) Ethernet Connection I219-LM", false},
		{"Wi-Fi", "Intel(R) Wi-Fi 6 AX201 160MHz", false},
		{"vEthernet (Default Switch)", "Hyper-V Virtual Ethernet Adapter", true},
		{"VMware Network Adapter VMnet8", "VMware Virtual Ethernet Adapter for VMnet8", true},
		{"Local Area Connection", "TAP-Windows Adapter V9", true},
	}
	for _, tt := range tests {
		if got := isVirtual(tt.name, tt.description); got != tt.want {
			t.Errorf("isVirtual(%q, %q) = %v, want %v", tt.name, tt.description, got, tt.want)
		}
	}
}
