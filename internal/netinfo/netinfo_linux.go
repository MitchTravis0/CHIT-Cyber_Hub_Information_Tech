package netinfo

import "os"

// platformEnrich fills in the fields the Go standard library does not expose.
// On Linux the gateway comes from /proc/net/route, DNS from resolv.conf (plus
// systemd-resolved when resolv.conf only holds the 127.0.0.53 stub) and the
// DHCP state from the kernel's "dynamic" address flag via the ip command.
func platformEnrich(r *Report) string {
	defaultIface, gateways := parseProcNetRoute(readFile("/proc/net/route"))

	servers, search := parseResolvConf(readFile("/etc/resolv.conf"))
	if allLoopback(servers) {
		if upstream, domains := parseResolvConf(readFile("/run/systemd/resolve/resolv.conf")); len(upstream) > 0 {
			servers = upstream
			if len(domains) > 0 {
				search = domains
			}
		}
	}
	r.DNS, r.SearchDomains = servers, search

	perLink := map[string][]string{}
	if out, err := run("resolvectl", "status", "--no-pager"); err == nil {
		perLink = parseResolvectlStatus(out)
	}

	var dynamic map[string]bool
	if out, err := run("ip", "-4", "-o", "addr", "show"); err == nil {
		dynamic = parseIPAddrDynamic(out)
	} else {
		r.markUnsupported(FieldDHCP)
	}

	for i := range r.Adapters {
		a := &r.Adapters[i]
		a.Gateway = gateways[a.Name]
		a.DNS = perLink[a.Name]
		if !a.Loopback {
			a.Virtual = virtualSysfs(a.Name)
		}
		if leased, known := dynamic[a.Name]; known {
			a.DHCP = DHCPStatic
			if leased {
				a.DHCP = DHCPDynamic
			}
		}
	}
	if len(perLink) == 0 {
		r.markUnsupported(FieldAdapterDNS)
	}
	return defaultIface
}

// virtualSysfs uses the kernel's own answer: only a real NIC has a device
// symlink under /sys/class/net.
func virtualSysfs(name string) bool {
	if _, err := os.Stat("/sys/class/net/" + name); err != nil {
		return isVirtual(name, "")
	}
	_, err := os.Stat("/sys/class/net/" + name + "/device")
	return err != nil
}
