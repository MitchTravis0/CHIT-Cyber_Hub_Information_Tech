package netinfo

import "testing"

func adapter(name string, opts ...func(*Adapter)) Adapter {
	a := Adapter{Name: name, Up: true, IPv4: []IPv4{{IP: "192.168.1.10", Prefix: 24, CIDR: "192.168.1.10/24"}}}
	for _, o := range opts {
		o(&a)
	}
	return a
}

func withGateway(gw string) func(*Adapter) { return func(a *Adapter) { a.Gateway = gw } }
func down() func(*Adapter)                 { return func(a *Adapter) { a.Up = false } }
func virtual() func(*Adapter)              { return func(a *Adapter) { a.Virtual = true } }
func loopback() func(*Adapter) {
	return func(a *Adapter) {
		a.Loopback = true
		a.IPv4 = []IPv4{{IP: "127.0.0.1", Prefix: 8, CIDR: "127.0.0.1/8"}}
	}
}
func noIP() func(*Adapter) { return func(a *Adapter) { a.IPv4 = nil } }
func apipa() func(*Adapter) {
	return func(a *Adapter) {
		a.IPv4 = []IPv4{{IP: "169.254.7.7", Prefix: 16, CIDR: "169.254.7.7/16"}}
	}
}

func TestSelectPrimary(t *testing.T) {
	tests := []struct {
		name         string
		adapters     []Adapter
		defaultIface string
		want         string
	}{
		{
			name:         "the OS default route wins even against a nicer looking adapter",
			adapters:     []Adapter{adapter("eth0", withGateway("192.168.1.1")), adapter("wlan0")},
			defaultIface: "wlan0",
			want:         "wlan0",
		},
		{
			name:     "a gateway beats no gateway when the OS did not say",
			adapters: []Adapter{adapter("eth0"), adapter("wlan0", withGateway("192.168.1.1"))},
			want:     "wlan0",
		},
		{
			name:     "physical beats virtual when neither has a gateway",
			adapters: []Adapter{adapter("docker0", virtual()), adapter("eth0")},
			want:     "eth0",
		},
		{
			name:     "a virtual adapter with a gateway still beats a physical one without",
			adapters: []Adapter{adapter("eth0"), adapter("tun0", virtual(), withGateway("10.8.0.1"))},
			want:     "tun0",
		},
		{
			name:         "down, loopback, address-less and APIPA adapters are never primary",
			adapters:     []Adapter{adapter("eth0", down(), withGateway("192.168.1.1")), adapter("lo", loopback()), adapter("eth1", noIP()), adapter("eth2", apipa())},
			defaultIface: "eth0",
			want:         "",
		},
		{
			name:         "a stale default route name does not force a bad pick",
			adapters:     []Adapter{adapter("eth9", noIP()), adapter("wlan0", withGateway("192.168.1.1"))},
			defaultIface: "eth9",
			want:         "wlan0",
		},
		{
			name:     "no adapters at all",
			adapters: nil,
			want:     "",
		},
	}
	for _, tt := range tests {
		i := selectPrimary(tt.adapters, tt.defaultIface)
		got := ""
		if i >= 0 {
			got = tt.adapters[i].Name
		}
		if got != tt.want {
			t.Errorf("%s: primary = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSortAdapters(t *testing.T) {
	adapters := []Adapter{
		adapter("lo", loopback()),
		adapter("docker0", virtual()),
		adapter("eth1", down()),
		adapter("wlan0", withGateway("192.168.1.1")),
		adapter("br0", virtual()),
	}
	adapters[3].Primary = true
	sortAdapters(adapters)

	want := []string{"wlan0", "br0", "docker0", "eth1", "lo"}
	for i, name := range want {
		if adapters[i].Name != name {
			t.Fatalf("order = %v, want %v", names(adapters), want)
		}
	}
}

func names(adapters []Adapter) []string {
	out := make([]string, len(adapters))
	for i, a := range adapters {
		out[i] = a.Name
	}
	return out
}

func TestHasUsableIPv4(t *testing.T) {
	if hasUsableIPv4(adapter("eth0", apipa())) {
		t.Error("a 169.254 address is not a usable one")
	}
	if !hasUsableIPv4(adapter("eth0")) {
		t.Error("a normal LAN address should count")
	}
}
