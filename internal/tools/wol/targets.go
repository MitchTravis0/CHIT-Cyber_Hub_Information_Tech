package wol

import (
	"net"

	"chit/internal/core"
	"chit/internal/netinfo"
)

// LimitedBroadcast is the address every IPv4 host on the wire listens to. Some
// network cards act only on this one, others only on the subnet broadcast, so
// both are sent.
const LimitedBroadcast = "255.255.255.255"

// fallbackPort is what older BIOSes listen on. Port 9 (discard) is the usual
// one, port 7 (echo) is the one that rescues an old desktop.
const fallbackPort = 7

// Target is one place a magic packet will be sent from and to.
type Target struct {
	Adapter string `json:"adapter"`
	// From is the local address the socket binds to, which is what decides
	// which network card the packet actually leaves by.
	From string `json:"from"`
	To   string `json:"to"`
	Port int    `json:"port"`
}

// SelectTargets works out where to send. override, when not empty, wins and is
// used verbatim from every eligible adapter. Otherwise every eligible adapter
// contributes its own subnet broadcast plus the limited broadcast.
func SelectTargets(adapters []netinfo.Adapter, override string, port int) ([]Target, error) {
	if override != "" && !isIPv4(override) {
		return nil, core.Errorf(core.CodeInvalidInput,
			"\"%s\" is not an address to send to. Use something like 192.168.1.255, or leave it empty to let CHIT work it out.",
			override)
	}

	targets := make([]Target, 0, 4)
	for _, a := range adapters {
		if !a.Up || a.Loopback || a.Virtual {
			continue
		}
		for _, entry := range a.IPv4 {
			if !eligible(entry) {
				continue
			}
			if override != "" {
				targets = append(targets, Target{Adapter: a.Name, From: entry.IP, To: override, Port: port})
				continue
			}
			targets = append(targets,
				Target{Adapter: a.Name, From: entry.IP, To: entry.Broadcast, Port: port},
				Target{Adapter: a.Name, From: entry.IP, To: LimitedBroadcast, Port: port})
		}
	}

	if port == defaultPort {
		onNine := targets
		for _, t := range onNine {
			t.Port = fallbackPort
			targets = append(targets, t)
		}
	}

	targets = dedupe(targets)
	if len(targets) == 0 {
		return nil, core.Errorf(core.CodeNetwork,
			"This machine is not on a network it can send a wake-up packet from. Connect it to the same network as the machine you want to wake.")
	}
	return targets, nil
}

// eligible reports whether an address can carry a broadcast a sleeping machine
// on the same wire will hear. An APIPA address means the adapter never got a
// lease, and a /31 or /32 has no broadcast address at all.
func eligible(entry netinfo.IPv4) bool {
	ip := net.ParseIP(entry.IP)
	if ip == nil || ip.To4() == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	if entry.Prefix < 1 || entry.Prefix > 30 {
		return false
	}
	return isIPv4(entry.Broadcast)
}

func isIPv4(text string) bool {
	ip := net.ParseIP(text)
	return ip != nil && ip.To4() != nil
}

func dedupe(targets []Target) []Target {
	type key struct {
		from string
		to   string
		port int
	}
	seen := make(map[key]bool, len(targets))
	out := make([]Target, 0, len(targets))
	for _, t := range targets {
		k := key{from: t.From, to: t.To, port: t.Port}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}
