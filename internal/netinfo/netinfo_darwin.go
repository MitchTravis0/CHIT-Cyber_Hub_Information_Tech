package netinfo

// platformEnrich fills in the fields the Go standard library does not expose.
// macOS has no public API for these, so the documented command line tools are
// used: netstat for routes, scutil for DNS, ipconfig for the DHCP lease.
func platformEnrich(r *Report) string {
	defaultIface := ""
	gateways := map[string]string{}
	if out, err := run("netstat", "-rn", "-f", "inet"); err == nil {
		defaultIface, gateways = parseNetstatDefaults(out)
	} else {
		r.markUnsupported(FieldGateway)
	}

	perIface := map[string][]string{}
	if out, err := run("scutil", "--dns"); err == nil {
		r.DNS, r.SearchDomains, perIface = parseScutilDNS(out)
	} else {
		r.markUnsupported(FieldDNS, FieldAdapterDNS)
	}

	dhcpKnown := false
	for i := range r.Adapters {
		a := &r.Adapters[i]
		a.Gateway = gateways[a.Name]
		a.DNS = perIface[a.Name]
		if !a.Up || a.Loopback || a.Virtual || !hasUsableIPv4(*a) {
			continue
		}
		// ipconfig is only worth running on an adapter that could hold a lease.
		out, err := run("ipconfig", "getpacket", a.Name)
		if err != nil {
			continue
		}
		if a.DHCP = parseGetpacket(out); a.DHCP != "" {
			dhcpKnown = true
		}
	}
	if !dhcpKnown {
		r.markUnsupported(FieldDHCP)
	}
	return defaultIface
}
