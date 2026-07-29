// Package netinfo reports the local machine's network adapters: addresses,
// gateway, DNS servers and DHCP state. Fields an operating system will not
// tell us are left empty and listed in Report.Unsupported, never guessed.
package netinfo

import (
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"chit/internal/core"
)

// Values of Adapter.DHCP. An empty string means this OS did not say.
const (
	DHCPDynamic = "dhcp"
	DHCPStatic  = "static"
)

// Ids used in Report.Unsupported, so the UI can say "not available on this OS"
// against the right row instead of showing a blank.
const (
	FieldGateway    = "gateway"
	FieldDNS        = "dns"
	FieldAdapterDNS = "adapterDns"
	FieldDHCP       = "dhcp"
)

// IPv4 is one IPv4 address with the subnet expressed both ways, because
// routers show masks and documentation shows prefixes.
type IPv4 struct {
	IP        string `json:"ip"`
	Mask      string `json:"mask"`
	Prefix    int    `json:"prefix"`
	CIDR      string `json:"cidr"`
	Network   string `json:"network"`
	Broadcast string `json:"broadcast"`
}

type IPv6 struct {
	IP     string `json:"ip"`
	Prefix int    `json:"prefix"`
	CIDR   string `json:"cidr"`
	Scope  string `json:"scope"`
}

type Adapter struct {
	Name string `json:"name"`
	// FriendlyName is the name a user sees in network settings. Windows only,
	// empty elsewhere where Name already is the friendly name.
	FriendlyName string   `json:"friendlyName"`
	Description  string   `json:"description"`
	Index        int      `json:"index"`
	Up           bool     `json:"up"`
	MAC          string   `json:"mac"`
	MTU          int      `json:"mtu"`
	Loopback     bool     `json:"loopback"`
	Virtual      bool     `json:"virtual"`
	Primary      bool     `json:"primary"`
	IPv4         []IPv4   `json:"ipv4"`
	IPv6         []IPv6   `json:"ipv6"`
	Gateway      string   `json:"gateway"`
	DNS          []string `json:"dns"`
	DHCP         string   `json:"dhcp"`
}

type Report struct {
	OS       string    `json:"os"`
	Hostname string    `json:"hostname"`
	Adapters []Adapter `json:"adapters"`
	// DNS and SearchDomains are the system-wide resolver settings, used when an
	// OS cannot break DNS down per adapter.
	DNS           []string `json:"dns"`
	SearchDomains []string `json:"searchDomains"`
	Unsupported   []string `json:"unsupported"`
}

// List returns every adapter on this machine, primary first.
func List() (Report, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return Report{}, core.Errorf(core.CodeInternal,
			"Could not read this machine's network adapters. Check that the network service is running.")
	}

	r := Report{OS: runtime.GOOS, Adapters: make([]Adapter, 0, len(ifaces))}
	r.Hostname, _ = os.Hostname()
	for _, ifi := range ifaces {
		r.Adapters = append(r.Adapters, newAdapter(ifi))
	}

	defaultIface := platformEnrich(&r)
	if i := selectPrimary(r.Adapters, defaultIface); i >= 0 {
		r.Adapters[i].Primary = true
	}
	sortAdapters(r.Adapters)
	return r, nil
}

func newAdapter(ifi net.Interface) Adapter {
	a := Adapter{
		Name:  ifi.Name,
		Index: ifi.Index,
		// An adapter that is enabled but has no carrier (unplugged cable, down
		// bridge) reads as down to a tech, so both flags have to be set.
		Up:       ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagRunning != 0,
		MAC:      macString(ifi.HardwareAddr),
		MTU:      ifi.MTU,
		Loopback: ifi.Flags&net.FlagLoopback != 0,
	}
	a.Virtual = !a.Loopback && isVirtual(a.Name, "")

	addrs, err := ifi.Addrs()
	if err != nil {
		return a
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if v4 := ipnet.IP.To4(); v4 != nil {
			a.IPv4 = append(a.IPv4, newIPv4(v4, ipnet.Mask))
			continue
		}
		a.IPv6 = append(a.IPv6, newIPv6(ipnet.IP, ipnet.Mask))
	}
	return a
}

func newIPv4(ip net.IP, mask net.IPMask) IPv4 {
	if len(mask) == net.IPv6len {
		mask = mask[12:]
	}
	prefix := prefixLen(mask)
	out := IPv4{IP: ip.String(), Mask: maskString(mask), Prefix: prefix}
	if prefix < 0 {
		return out
	}
	out.CIDR = out.IP + "/" + strconv.Itoa(prefix)
	if network := networkAddress(ip, mask); network != nil {
		out.Network = network.String() + "/" + strconv.Itoa(prefix)
	}
	if b := broadcastAddress(ip, mask); b != nil {
		out.Broadcast = b.String()
	}
	return out
}

func newIPv6(ip net.IP, mask net.IPMask) IPv6 {
	prefix := prefixLen(mask)
	out := IPv6{IP: ip.String(), Prefix: prefix, Scope: ipv6Scope(ip)}
	if prefix >= 0 {
		out.CIDR = out.IP + "/" + strconv.Itoa(prefix)
	}
	return out
}

// Interface names that are always software constructs.
var virtualPrefixes = []string{
	"br-", "docker", "veth", "virbr", "vmnet", "vboxnet", "lxc", "lxd",
	"cni", "flannel", "kube", "tun", "tap", "utun", "wg", "zt", "ipsec",
	"gif", "stf", "awdl", "llw", "anpi", "ap1", "bridge", "nordlynx", "ham",
}

// Words that only ever appear on a virtual adapter's name or description.
var virtualKeywords = []string{
	"virtual", "vmware", "virtualbox", "hyper-v", "vethernet", "tap-windows",
	"wan miniport", "wireguard", "zerotier", "tailscale", "openvpn", "wintun",
	"pseudo-interface", "npcap", "teredo", "6to4", "isatap", "docker",
}

// isVirtual reports whether an adapter is software rather than hardware. A
// wrong answer only changes whether the card starts collapsed, so keyword
// matching is good enough where the OS does not tell us outright.
func isVirtual(name, description string) bool {
	n := strings.ToLower(name)
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	hay := n + " " + strings.ToLower(description)
	for _, k := range virtualKeywords {
		if strings.Contains(hay, k) {
			return true
		}
	}
	return false
}

// sortAdapters puts the adapters in the order a tech reads them: the primary
// one, then other working adapters, then the junk a laptop accumulates.
func sortAdapters(adapters []Adapter) {
	sort.SliceStable(adapters, func(i, j int) bool {
		ri, rj := adapterRank(adapters[i]), adapterRank(adapters[j])
		if ri != rj {
			return ri < rj
		}
		return adapters[i].Name < adapters[j].Name
	})
}

func adapterRank(a Adapter) int {
	switch {
	case a.Primary:
		return 0
	case a.Up && !a.Virtual && !a.Loopback && hasUsableIPv4(a):
		return 1
	case a.Up && hasUsableIPv4(a):
		return 2
	case a.Up && !a.Loopback:
		return 3
	default:
		return 4
	}
}

func hasUsableIPv4(a Adapter) bool {
	for _, v4 := range a.IPv4 {
		if ip := net.ParseIP(v4.IP); ip != nil && usableIPv4(ip) {
			return true
		}
	}
	return false
}

// markUnsupported records a field this OS cannot report, once.
func (r *Report) markUnsupported(fields ...string) {
	for _, f := range fields {
		found := false
		for _, existing := range r.Unsupported {
			if existing == f {
				found = true
				break
			}
		}
		if !found {
			r.Unsupported = append(r.Unsupported, f)
		}
	}
}
