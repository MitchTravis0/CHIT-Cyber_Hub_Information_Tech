package netinfo

import (
	"net"
	"regexp"
	"strconv"
	"strings"
)

// The parsers live here, away from the platform files, so every one of them is
// testable on any OS from a captured fixture.

// parseProcNetRoute reads Linux /proc/net/route and returns the interface that
// owns the lowest-metric default route plus the default gateway of every
// interface that has one.
func parseProcNetRoute(data string) (defaultIface string, gateways map[string]string) {
	gateways = map[string]string{}
	best := -1
	for i, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if i == 0 || len(fields) < 8 {
			continue
		}
		iface, dest, gw := fields[0], fields[1], fields[2]
		if dest != "00000000" {
			continue
		}
		ip := hexToIPv4(gw)
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		metric, err := strconv.Atoi(fields[6])
		if err != nil {
			continue
		}
		if _, seen := gateways[iface]; !seen {
			gateways[iface] = ip.String()
		}
		if best == -1 || metric < best {
			best, defaultIface = metric, iface
		}
	}
	return defaultIface, gateways
}

// parseResolvConf pulls the nameservers and search domains out of a
// resolv.conf(5) file.
func parseResolvConf(data string) (servers []string, search []string) {
	for _, line := range strings.Split(data, "\n") {
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "nameserver":
			if net.ParseIP(fields[1]) != nil {
				servers = append(servers, fields[1])
			}
		case "search", "domain":
			for _, d := range fields[1:] {
				if d != "." {
					search = append(search, d)
				}
			}
		}
	}
	return servers, search
}

// allLoopback reports whether every server in the list is a local stub
// resolver, which means the real upstream servers are somewhere else.
func allLoopback(servers []string) bool {
	if len(servers) == 0 {
		return false
	}
	for _, s := range servers {
		ip := net.ParseIP(s)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	return true
}

var (
	resolvectlLink = regexp.MustCompile(`^Link\s+\d+\s+\(([^)]+)\)`)
	// A label line is "Some Words: value". IPv6 continuation lines never match
	// because the character after the first colon is another colon or a digit.
	labelLine = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9 ./-]*):(\s.*)?$`)
)

// parseResolvectlStatus maps interface name to its DNS servers, using the
// output of "resolvectl status" (systemd-resolved). Machines running
// systemd-resolved show only a 127.0.0.53 stub in resolv.conf, so this is the
// only way to report the servers a tech actually cares about.
func parseResolvectlStatus(data string) map[string][]string {
	out := map[string][]string{}
	link := ""
	collecting := false
	for _, line := range strings.Split(data, "\n") {
		if m := resolvectlLink.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			link, collecting = m[1], false
			continue
		}
		if strings.TrimSpace(line) == "Global" {
			link, collecting = "", false
			continue
		}
		if m := labelLine.FindStringSubmatch(line); m != nil {
			collecting = strings.TrimSpace(m[1]) == "DNS Servers"
			if collecting && link != "" {
				out[link] = append(out[link], dnsFields(m[2])...)
			}
			continue
		}
		if collecting && link != "" {
			out[link] = append(out[link], dnsFields(line)...)
		}
	}
	return out
}

// dnsFields keeps the whitespace-separated tokens that really are IP
// addresses, allowing for a "%zone" or "#port" suffix.
func dnsFields(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		host, _, _ := strings.Cut(f, "%")
		host, _, _ = strings.Cut(host, "#")
		if net.ParseIP(host) != nil {
			out = append(out, f)
		}
	}
	return out
}

// parseIPAddrDynamic reads "ip -4 -o addr show" and reports, per interface,
// whether its global IPv4 address carries the kernel's "dynamic" flag. That
// flag is set only for a leased address, so it is a fact rather than a guess.
// Interfaces with no global IPv4 address are absent from the map.
func parseIPAddrDynamic(data string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[2] != "inet" {
			continue
		}
		iface := fields[1]
		attrs := fields[3:]
		global, dynamic := false, false
		for i, f := range attrs {
			switch f {
			case "scope":
				global = i+1 < len(attrs) && attrs[i+1] == "global"
			case "dynamic":
				dynamic = true
			}
		}
		if !global {
			continue
		}
		out[iface] = out[iface] || dynamic
	}
	return out
}

// parseNetstatDefaults reads "netstat -rn -f inet" (macOS and the BSDs) and
// returns the interface holding the first default route plus the gateway of
// every interface that has a default route.
func parseNetstatDefaults(data string) (defaultIface string, gateways map[string]string) {
	gateways = map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "default" {
			continue
		}
		if net.ParseIP(fields[1]) == nil {
			continue
		}
		iface := fields[3]
		if _, seen := gateways[iface]; !seen {
			gateways[iface] = fields[1]
		}
		if defaultIface == "" {
			defaultIface = iface
		}
	}
	return defaultIface, gateways
}

var scutilIfIndex = regexp.MustCompile(`^if_index\s*:\s*\d+\s+\(([^)]+)\)`)

// parseScutilDNS reads "scutil --dns" (macOS) and returns the system-wide
// resolvers (those of resolver #1), the search domains, and the per-interface
// resolvers taken from the scoped resolver blocks.
func parseScutilDNS(data string) (system []string, search []string, byIface map[string][]string) {
	byIface = map[string][]string{}
	var servers, domains []string
	iface := ""
	flush := func() {
		if len(servers) == 0 {
			return
		}
		if iface != "" {
			if _, seen := byIface[iface]; !seen {
				byIface[iface] = servers
			}
		}
		if system == nil {
			system, search = servers, domains
		}
	}
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "resolver #") {
			flush()
			servers, domains, iface = nil, nil, ""
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch {
		case strings.HasPrefix(key, "nameserver["):
			if net.ParseIP(value) != nil {
				servers = append(servers, value)
			}
		case strings.HasPrefix(key, "search domain["), key == "domain":
			if value != "" {
				domains = append(domains, value)
			}
		case key == "if_index":
			if m := scutilIfIndex.FindStringSubmatch(trimmed); m != nil {
				iface = m[1]
			}
		}
	}
	flush()
	return system, search, byIface
}

// parseGetpacket reads "ipconfig getpacket <iface>" (macOS). A DHCP-configured
// interface prints its lease packet; a statically configured one prints
// nothing. Anything else is reported as unknown rather than guessed at.
func parseGetpacket(out string) string {
	trimmed := strings.TrimSpace(out)
	switch {
	case trimmed == "":
		return DHCPStatic
	case strings.Contains(out, "op = BOOTREPLY"), strings.Contains(out, "yiaddr"):
		return DHCPDynamic
	default:
		return ""
	}
}
