package triage

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"chit/internal/netinfo"
)

// minPortTimeout keeps each gateway port probe usable once one rung's budget is
// split three ways.
const minPortTimeout = 250 * time.Millisecond

// adapterRung asks whether this computer has a usable IPv4 address at all.
func adapterRung(_ context.Context, p Probes, _ time.Duration) Rung {
	report, err := p.Adapters()
	if err != nil {
		return Rung{
			Status: StatusFail,
			Detail: "this computer would not list its adapters",
			Advice: "Check that the network service is running on this computer.",
		}
	}

	var apipa netinfo.Adapter
	var apipaIP string
	var ipv6Only netinfo.Adapter
	for _, a := range report.Adapters {
		if !a.Up || a.Loopback {
			continue
		}
		for _, ip := range a.IPv4 {
			if isAPIPA(ip.IP) {
				if apipaIP == "" {
					apipa, apipaIP = a, ip.IP
				}
				continue
			}
			// A real address wins outright, even on a machine that also has a
			// self-assigned one on another adapter.
			return Rung{
				Status: StatusOK,
				Target: a.Name,
				Detail: fmt.Sprintf("%s on %s", ip.IP, a.Name),
				Advice: "This computer has an address on the network.",
			}
		}
		if len(a.IPv4) == 0 && len(a.IPv6) > 0 && ipv6Only.Name == "" {
			ipv6Only = a
		}
	}

	if apipaIP != "" {
		return Rung{
			Status: StatusFail,
			Target: apipa.Name,
			Detail: fmt.Sprintf("%s on %s, which is a self-assigned address", apipaIP, apipa.Name),
			Advice: "No DHCP server answered, so Windows gave this computer an address of its own that reaches nothing. Check the cable, the switch port, and whether the DHCP server is running.",
		}
	}
	if ipv6Only.Name != "" {
		return Rung{
			Status: StatusFail,
			Target: ipv6Only.Name,
			Detail: fmt.Sprintf("%s has an IPv6 address but no IPv4 address", ipv6Only.Name),
			Advice: "CHIT checks IPv4 only. This network appears to be IPv6-only, which is rare outside a lab.",
		}
	}
	return Rung{
		Status: StatusFail,
		Detail: "no adapter has an address",
		Advice: "Nothing is connected. Plug in the cable or join a Wi-Fi network, then run this again.",
	}
}

// isAPIPA reports a 169.254.x.x address, which is what an operating system
// gives itself when no DHCP server answered.
func isAPIPA(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.Is4() {
		return false
	}
	return addr.AsSlice()[0] == 169 && addr.AsSlice()[1] == 254
}

// gatewayRung asks whether the router answers. A gateway that ignores
// everything is a warn, not a fail: plenty of business firewalls are set up
// that way on purpose and the ladder must carry on past it.
func gatewayRung(ctx context.Context, p Probes, timeout time.Duration) Rung {
	report, err := p.Adapters()
	if err != nil {
		return Rung{
			Status: StatusWarn,
			Detail: "this computer will not say what its gateway is",
			Advice: "The gateway could not be checked on this operating system, so the run carried on. The next steps still tell you whether traffic is getting out.",
		}
	}

	gateway := gatewayOf(report)
	if gateway == "" {
		if unsupported(report, netinfo.FieldGateway) {
			return Rung{
				Status: StatusWarn,
				Detail: "this computer will not say what its gateway is",
				Advice: "The gateway could not be checked on this operating system, so the run carried on. The next steps still tell you whether traffic is getting out.",
			}
		}
		return Rung{
			Status: StatusFail,
			Detail: "this computer has no default gateway",
			Advice: "Without a gateway nothing leaves this network. If the address came from DHCP, the DHCP server is handing out an incomplete configuration; if it was typed in by hand, the gateway line is missing.",
		}
	}

	if ms, ok := p.Ping(ctx, gateway, timeout); ok {
		return Rung{
			Status: StatusOK,
			Target: gateway,
			Detail: fmt.Sprintf("%s answered ping in %s", gateway, fmtMS(ms)),
			Advice: "The router or firewall on this network is reachable.",
		}
	}

	// The three ports share one rung's budget rather than each getting the whole
	// of it. A gateway that ignores everything is common, and without this the
	// step alone burns four times the timeout before the ladder moves on.
	// internal/netscan.TCPProbe splits its budget the same way.
	perPort := timeout / time.Duration(len(GatewayPorts))
	if perPort < minPortTimeout {
		perPort = minPortTimeout
	}
	for _, port := range GatewayPorts {
		addr := gateway + ":" + strconv.Itoa(port)
		if ms, ok := p.Dial(ctx, addr, perPort); ok {
			return Rung{
				Status: StatusOK,
				Target: gateway,
				Detail: fmt.Sprintf("%s ignored ping but accepted a connection on port %d in %s",
					gateway, port, fmtMS(ms)),
				Advice: "The router is there. It is set not to answer ping, which is normal on a business firewall.",
			}
		}
	}

	return Rung{
		Status: StatusWarn,
		Target: gateway,
		Detail: fmt.Sprintf("%s did not answer ping or a connection on ports %s",
			gateway, joinPorts(GatewayPorts)),
		Advice: "Plenty of gateways are set to ignore everything, so this alone does not mean the network is down. The next steps will tell you.",
	}
}

func gatewayOf(report netinfo.Report) string {
	for _, a := range report.Adapters {
		if a.Primary && a.Gateway != "" {
			return a.Gateway
		}
	}
	for _, a := range report.Adapters {
		if a.Gateway != "" {
			return a.Gateway
		}
	}
	return ""
}

func unsupported(report netinfo.Report, field string) bool {
	for _, f := range report.Unsupported {
		if f == field {
			return true
		}
	}
	return false
}

func joinPorts(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, strconv.Itoa(p))
	}
	if len(parts) < 2 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " or " + parts[len(parts)-1]
}

// dnsRung tells a broken DNS server apart from a dead internet by asking a
// public resolver the same question when the system one fails.
func dnsRung(ctx context.Context, p Probes, timeout time.Duration) Rung {
	started := time.Now()
	addrs, err := p.Resolve(ctx, "", DNSTestName, timeout)
	elapsed := milliseconds(time.Since(started))

	if err == nil && len(addrs) > 0 {
		return Rung{
			Status: StatusOK,
			Target: DNSTestName,
			Detail: fmt.Sprintf("%s resolved to %s in %s", DNSTestName, addrs[0], fmtMS(elapsed)),
			Advice: "Name lookups are working.",
		}
	}
	if err == nil {
		return Rung{
			Status: StatusFail,
			Target: DNSTestName,
			Detail: fmt.Sprintf("this computer's DNS server answered about %s with no addresses", DNSTestName),
			Advice: "A DNS server is answering but giving empty replies, which is what a misconfigured internal DNS server or a blocking filter does.",
		}
	}

	fallbackStart := time.Now()
	fallback, fallbackErr := p.Resolve(ctx, FallbackResolver, DNSTestName, timeout)
	fallbackMS := milliseconds(time.Since(fallbackStart))
	if fallbackErr == nil && len(fallback) > 0 {
		return Rung{
			Status: StatusFail,
			Target: DNSTestName,
			Detail: fmt.Sprintf("this computer's DNS server could not resolve %s, but %s resolved it in %s",
				DNSTestName, FallbackResolver, fmtMS(fallbackMS)),
			Advice: "The internet itself is reachable: the fault is this computer's DNS server. Check the DNS addresses on the adapter, and whether the DNS server or domain controller is running.",
		}
	}
	return Rung{
		Status: StatusFail,
		Target: DNSTestName,
		Detail: fmt.Sprintf("neither this computer's DNS server nor %s could resolve %s",
			FallbackResolver, DNSTestName),
		Advice: "Nothing is getting out to port 53. That is either a firewall in the way or the network is down further out. The next steps are skipped because they would all fail for the same reason.",
	}
}

// internetRung reaches raw IP addresses, so it works even when name lookups do
// not. It is the step that separates "DNS is broken" from "nothing gets out".
func internetRung(ctx context.Context, p Probes, timeout time.Duration) Rung {
	for _, target := range InternetTargets {
		if ms, ok := p.Dial(ctx, target, timeout); ok {
			return Rung{
				Status: StatusOK,
				Target: target,
				Detail: fmt.Sprintf("%s answered in %s", target, fmtMS(ms)),
				Advice: "Traffic is reaching the internet. No DNS was involved in this step, so it works even when name lookups do not.",
			}
		}
	}
	return Rung{
		Status: StatusFail,
		Target: strings.Join(InternetTargets, " and "),
		Detail: fmt.Sprintf("neither %s answered", strings.Join(InternetTargets, " nor ")),
		Advice: "Nothing is getting out of this network. Check the router or firewall, and whether the line to the internet is up.",
	}
}

// httpsRung proves secure web traffic works end to end.
func httpsRung(ctx context.Context, p Probes, timeout time.Duration) Rung {
	started := time.Now()
	status, _, body, err := p.Get(ctx, HTTPSTestURL, timeout)
	elapsed := milliseconds(time.Since(started))

	target := hostOf(HTTPSTestURL)
	if err != nil {
		return Rung{
			Status: StatusFail,
			Target: target,
			Detail: "the connection failed before an answer came back",
			Advice: "HTTPS is not getting through. If everything above passed, something is inspecting or blocking secure traffic on this network.",
		}
	}
	if status == 204 && strings.TrimSpace(body) == "" {
		return Rung{
			Status: StatusOK,
			Target: target,
			Detail: fmt.Sprintf("answered 204 in %s", fmtMS(elapsed)),
			Advice: "Secure web traffic is working end to end.",
		}
	}
	return Rung{
		Status: StatusFail,
		Target: target,
		Detail: fmt.Sprintf("answered %d instead of 204", status),
		Advice: "Something is answering for a site it should not be, which usually means a filtering proxy is in the way. Check the proxy settings on this computer.",
	}
}

// portalRung is the step people miss: a network that hands out a perfectly good
// address and then intercepts everything until you sign in.
func portalRung(ctx context.Context, p Probes, timeout time.Duration) Rung {
	status, location, body, err := p.Get(ctx, PortalTestURL, timeout)
	target := hostOf(PortalTestURL)

	switch {
	case err != nil:
		return Rung{
			Status: StatusWarn,
			Target: target,
			Detail: "the check could not be made: the connection failed",
			Advice: "This one step could not be completed, so a login page cannot be ruled out. Everything else passed, so try a browser.",
		}
	case status == 200 && bodyMatches(body, PortalTestBody):
		return Rung{
			Status: StatusOK,
			Target: target,
			Detail: "answered 200 with the expected text",
			Advice: "Nothing is intercepting plain web traffic. This network is not asking you to sign in.",
		}
	case status >= 300 && status < 400:
		where := location
		if where == "" {
			where = "somewhere else"
		}
		return Rung{
			Status: StatusFail,
			Target: target,
			Detail: fmt.Sprintf("answered %d and sent us to %s", status, where),
			Advice: "A login page is in the way. This network wants you to accept its terms or sign in before it will let anything through. Open a browser and go to any plain http:// page and the login screen will appear.",
		}
	default:
		return Rung{
			Status: StatusFail,
			Target: target,
			Detail: fmt.Sprintf("answered %d but with different text from the expected one", status),
			Advice: "Something is rewriting plain web pages, which is what a captive portal or a filtering proxy does. Open a browser and go to any plain http:// page to see what appears.",
		}
	}
}

// hostOf is enough URL parsing for a label. The two URLs are constants in this
// package, so there is nothing hostile to defend against.
func hostOf(url string) string {
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	}
	if i := strings.IndexAny(url, "/?"); i >= 0 {
		url = url[:i]
	}
	return url
}
