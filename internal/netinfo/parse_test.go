package netinfo

import (
	"reflect"
	"testing"
)

const procNetRoute = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlan0	00000000	0101A8C0	0003	0	0	600	00000000	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
wlan0	0001A8C0	00000000	0001	0	0	600	00FFFFFF	0	0	0
eth0	00000000	FE01A8C0	0003	0	0	100	00000000	0	0	0
`

func TestParseProcNetRoute(t *testing.T) {
	defaultIface, gateways := parseProcNetRoute(procNetRoute)
	if defaultIface != "eth0" {
		t.Errorf("default iface = %q, want eth0 (lowest metric wins)", defaultIface)
	}
	want := map[string]string{"wlan0": "192.168.1.1", "eth0": "192.168.1.254"}
	if !reflect.DeepEqual(gateways, want) {
		t.Errorf("gateways = %v, want %v", gateways, want)
	}
}

func TestParseProcNetRouteNoDefault(t *testing.T) {
	data := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n" +
		"docker0\t000011AC\t00000000\t0001\t0\t0\t0\t0000FFFF\n"
	defaultIface, gateways := parseProcNetRoute(data)
	if defaultIface != "" || len(gateways) != 0 {
		t.Errorf("got %q %v, want no default route", defaultIface, gateways)
	}
}

func TestParseResolvConf(t *testing.T) {
	data := `# generated
nameserver 192.168.1.1
nameserver 2001:4860:4860::8888
nameserver not-an-ip
search home.lan corp.example
options edns0
`
	servers, search := parseResolvConf(data)
	wantServers := []string{"192.168.1.1", "2001:4860:4860::8888"}
	wantSearch := []string{"home.lan", "corp.example"}
	if !reflect.DeepEqual(servers, wantServers) {
		t.Errorf("servers = %v, want %v", servers, wantServers)
	}
	if !reflect.DeepEqual(search, wantSearch) {
		t.Errorf("search = %v, want %v", search, wantSearch)
	}
}

func TestParseResolvConfStub(t *testing.T) {
	// The systemd-resolved stub that hides the real servers.
	servers, search := parseResolvConf("nameserver 127.0.0.53\nsearch .\n")
	if !allLoopback(servers) {
		t.Errorf("servers = %v, want all loopback", servers)
	}
	if len(search) != 0 {
		t.Errorf("search = %v, want the bare dot dropped", search)
	}
	if allLoopback(nil) {
		t.Error("allLoopback(nil) = true, want false")
	}
	if allLoopback([]string{"127.0.0.53", "192.168.1.1"}) {
		t.Error("a mixed list must not count as loopback only")
	}
}

const resolvectlStatus = `Global
       Protocols: LLMNR=resolve -mDNS -DNSOverTLS DNSSEC=no/unsupported
resolv.conf mode: stub

Link 2 (eth0)
    Current Scopes: none
         Protocols: -DefaultRoute

Link 3 (wlan0)
    Current Scopes: DNS LLMNR/IPv4
         Protocols: +DefaultRoute
Current DNS Server: 192.168.1.1
       DNS Servers: 192.168.1.1 8.8.8.8
                    fe80::1%wlan0
        DNS Domain: home.lan
`

func TestParseResolvectlStatus(t *testing.T) {
	got := parseResolvectlStatus(resolvectlStatus)
	want := map[string][]string{"wlan0": {"192.168.1.1", "8.8.8.8", "fe80::1%wlan0"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("per-link DNS = %v, want %v", got, want)
	}
}

func TestParseIPAddrDynamic(t *testing.T) {
	data := `1: lo    inet 127.0.0.1/8 scope host lo\       valid_lft forever preferred_lft forever
3: wlan0    inet 192.168.1.244/24 metric 600 brd 192.168.1.255 scope global dynamic wlan0\       valid_lft 35919sec preferred_lft 35919sec
4: docker0    inet 172.17.0.1/16 brd 172.17.255.255 scope global docker0\       valid_lft forever preferred_lft forever
`
	got := parseIPAddrDynamic(data)
	want := map[string]bool{"wlan0": true, "docker0": false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dynamic = %v, want %v (loopback host scope is skipped)", got, want)
	}
}

const netstatDarwin = `Routing tables

Internet:
Destination        Gateway            Flags        Netif Expire
default            192.168.1.1        UGScg          en0
default            10.0.0.1           UGScIg         en5
127                127.0.0.1          UCS            lo0
192.168.1          link#12            UCS            en0      !
`

func TestParseNetstatDefaults(t *testing.T) {
	defaultIface, gateways := parseNetstatDefaults(netstatDarwin)
	if defaultIface != "en0" {
		t.Errorf("default iface = %q, want en0", defaultIface)
	}
	want := map[string]string{"en0": "192.168.1.1", "en5": "10.0.0.1"}
	if !reflect.DeepEqual(gateways, want) {
		t.Errorf("gateways = %v, want %v", gateways, want)
	}
}

const scutilDNS = `DNS configuration

resolver #1
  search domain[0] : home.lan
  nameserver[0] : 192.168.1.1
  nameserver[1] : 8.8.8.8
  if_index : 12 (en0)
  flags    : Request A records, Request AAAA records
  reach    : 0x00020002 (Reachable,Directly Reachable Address)

resolver #2
  domain   : local
  options  : mdns
  timeout  : 5
  flags    : Request A records

DNS configuration (for scoped queries)

resolver #1
  nameserver[0] : 10.0.0.1
  if_index : 15 (en5)
  flags    : Scoped, Request A records
`

func TestParseScutilDNS(t *testing.T) {
	system, search, byIface := parseScutilDNS(scutilDNS)
	if want := []string{"192.168.1.1", "8.8.8.8"}; !reflect.DeepEqual(system, want) {
		t.Errorf("system DNS = %v, want %v", system, want)
	}
	if want := []string{"home.lan"}; !reflect.DeepEqual(search, want) {
		t.Errorf("search = %v, want %v", search, want)
	}
	want := map[string][]string{
		"en0": {"192.168.1.1", "8.8.8.8"},
		"en5": {"10.0.0.1"},
	}
	if !reflect.DeepEqual(byIface, want) {
		t.Errorf("per-interface DNS = %v, want %v", byIface, want)
	}
}

func TestParseGetpacket(t *testing.T) {
	lease := `op = BOOTREPLY
htype = 1
yiaddr = 192.168.1.42
server_identifier (ip): 192.168.1.1
`
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"dhcp lease", lease, DHCPDynamic},
		{"static prints nothing", "", DHCPStatic},
		{"blank lines only", "\n  \n", DHCPStatic},
		{"unexpected output stays unknown", "some other tool output", ""},
	}
	for _, tt := range tests {
		if got := parseGetpacket(tt.in); got != tt.want {
			t.Errorf("%s: parseGetpacket = %q, want %q", tt.name, got, tt.want)
		}
	}
}
