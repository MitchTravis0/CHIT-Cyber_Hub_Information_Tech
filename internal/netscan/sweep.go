package netscan

import (
	"context"
	"math"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"chit/internal/core"
)

// KindHost is the result kind used for every Host emitted by a sweep.
const KindHost = "host"

// Host status and discovery method values.
const (
	StatusUp   = "up"
	StatusDown = "down"

	ViaICMP = "icmp"
	ViaTCP  = "tcp"
	ViaARP  = "arp"
)

// icmpFailureBudget is how many ping attempts may fail outright before the
// sweep stops trying. A blocked ICMP socket fails instantly on every host, and
// there is no point opening 65536 sockets to learn that twice.
const icmpFailureBudget = 3

// Host is one address in the scanned range. Every address in the range is
// reported, including the ones that stayed silent, so the UI can draw the whole
// block as free or occupied.
//
// Vendor stays empty unless Options.VendorLookup is supplied: the OUI database
// belongs to the tool layer.
type Host struct {
	IP           string  `json:"ip"`
	Status       string  `json:"status"`
	RespondedVia string  `json:"respondedVia"`
	LatencyMS    float64 `json:"latencyMs"`
	Hostname     string  `json:"hostname"`
	MAC          string  `json:"mac"`
	Vendor       string  `json:"vendor"`
	OpenPorts    []int   `json:"openPorts"`
}

// Summary is the tally handed back when the sweep finishes.
type Summary struct {
	Total         int    `json:"total"`
	Up            int    `json:"up"`
	Down          int    `json:"down"`
	ViaICMP       int    `json:"viaIcmp"`
	ViaTCP        int    `json:"viaTcp"`
	ViaARP        int    `json:"viaArp"`
	WithMAC       int    `json:"withMac"`
	ICMPAvailable bool   `json:"icmpAvailable"`
	ARPAvailable  bool   `json:"arpAvailable"`
	Note          string `json:"note"`
}

// Map renders the summary for core.JobContext.SetSummary.
func (s Summary) Map() map[string]any {
	return map[string]any{
		"total":         s.Total,
		"up":            s.Up,
		"down":          s.Down,
		"viaIcmp":       s.ViaICMP,
		"viaTcp":        s.ViaTCP,
		"viaArp":        s.ViaARP,
		"withMac":       s.WithMAC,
		"icmpAvailable": s.ICMPAvailable,
		"arpAvailable":  s.ARPAvailable,
		"note":          s.Note,
	}
}

// Sweep finds the live hosts in opts.Range and emits each address as a Host as
// soon as it resolves. Three stages back each other up so the scan works
// without administrator rights:
//
//  1. an ICMP echo, unprivileged where the OS allows it,
//  2. TCP connect probes to opts.Ports for anything silent on ping, where a
//     refused connection counts as proof of life,
//  3. a readback of the system ARP cache once the sweep is done, which finds
//     devices that answer nothing at all and supplies the MAC addresses.
//
// Stage 3 can turn an address that was reported down into an up host, so a Host
// may be emitted twice. Consumers must upsert on the ip field.
func Sweep(jc *core.JobContext, opts Options) (Summary, error) {
	o, err := opts.normalize()
	if err != nil {
		return Summary{}, err
	}

	ctx := jc.Ctx()
	addrs := o.Range.Addresses()
	total := len(addrs)
	jc.SetTotal(total)
	jc.Progress(0, total, "Scanning "+o.Range.Text)

	sw := &sweeper{opts: o}
	hosts := make(map[string]*Host, total)

	done := 0
	for h := range core.Pool(ctx, addrs, o.Workers, sw.probe) {
		host := h
		hosts[host.IP] = &host
		done++
		jc.Emit(KindHost, host)
		jc.Progress(done, total, "Scanning "+o.Range.Text)
	}
	if ctx.Err() != nil {
		return Summary{}, ctx.Err()
	}

	arpOK := true
	if !o.SkipARP {
		arpOK = sw.arpReadback(jc, hosts)
		if ctx.Err() != nil {
			return Summary{}, ctx.Err()
		}
	}

	return sw.summarize(hosts, arpOK), nil
}

type sweeper struct {
	opts      Options
	icmpFails atomic.Int32
	icmpOK    atomic.Bool
}

// icmpUsable stops the ping stage once it is clear the OS will not let this
// process send echo requests.
func (s *sweeper) icmpUsable() bool {
	return s.icmpOK.Load() || s.icmpFails.Load() < icmpFailureBudget
}

func (s *sweeper) probe(ctx context.Context, addr netip.Addr) (Host, bool) {
	h := Host{IP: addr.String(), Status: StatusDown, OpenPorts: []int{}}
	if ctx.Err() != nil {
		return h, false
	}

	if !s.opts.SkipICMP && s.icmpUsable() {
		rtt, ok, err := PingOnce(ctx, addr, s.opts.Timeout)
		switch {
		case ok:
			s.icmpOK.Store(true)
			h.Status, h.RespondedVia, h.LatencyMS = StatusUp, ViaICMP, milliseconds(rtt)
		case err != nil && ctx.Err() == nil:
			s.icmpFails.Add(1)
		}
	}

	if h.Status != StatusUp && !s.opts.SkipTCP && ctx.Err() == nil {
		port, rtt, ok := TCPProbe(ctx, addr, s.opts.Ports, s.opts.Timeout)
		if ok {
			h.Status, h.RespondedVia, h.LatencyMS = StatusUp, ViaTCP, milliseconds(rtt)
			if port > 0 {
				h.OpenPorts = []int{port}
			}
		}
	}

	if h.Status == StatusUp && !s.opts.SkipHostnames {
		h.Hostname = reverseLookup(ctx, h.IP, s.opts.DNSTimeout)
	}
	return h, true
}

// arpReadback folds the system ARP cache into the results. It fills in MAC
// addresses and promotes addresses that ignored both earlier stages.
func (s *sweeper) arpReadback(jc *core.JobContext, hosts map[string]*Host) bool {
	ctx := jc.Ctx()
	jc.Progress(len(hosts), len(hosts), "Checking the ARP cache for quiet devices")

	entries, err := arpTable(ctx)
	if err != nil {
		return false
	}

	var promoted []*Host
	for _, e := range entries {
		h, ok := hosts[e.IP]
		if !ok {
			continue
		}
		changed := false
		if h.MAC == "" {
			h.MAC = e.MAC
			if s.opts.VendorLookup != nil {
				h.Vendor = s.opts.VendorLookup(e.MAC)
			}
			changed = true
		}
		if h.Status != StatusUp {
			h.Status, h.RespondedVia = StatusUp, ViaARP
			promoted = append(promoted, h)
			continue
		}
		if changed {
			jc.Emit(KindHost, *h)
		}
	}

	// Newly found hosts still need a name, and doing those lookups one after
	// another would stall the end of the scan.
	workers := min(s.opts.Workers, 32)
	for h := range core.Pool(ctx, promoted, workers, func(c context.Context, h *Host) (*Host, bool) {
		if !s.opts.SkipHostnames {
			h.Hostname = reverseLookup(c, h.IP, s.opts.DNSTimeout)
		}
		return h, true
	}) {
		jc.Emit(KindHost, *h)
	}
	return true
}

func (s *sweeper) summarize(hosts map[string]*Host, arpOK bool) Summary {
	// ICMP counts as available unless it actually failed: a range where nothing
	// answered says nothing about this machine's permissions.
	icmpBlocked := !s.opts.SkipICMP && !s.icmpOK.Load() && s.icmpFails.Load() >= icmpFailureBudget
	sum := Summary{
		Total:         len(hosts),
		ICMPAvailable: !icmpBlocked,
		ARPAvailable:  s.opts.SkipARP || arpOK,
	}
	for _, h := range hosts {
		if h.Status != StatusUp {
			sum.Down++
			continue
		}
		sum.Up++
		switch h.RespondedVia {
		case ViaICMP:
			sum.ViaICMP++
		case ViaTCP:
			sum.ViaTCP++
		case ViaARP:
			sum.ViaARP++
		}
		if h.MAC != "" {
			sum.WithMAC++
		}
	}

	var notes []string
	if !sum.ICMPAvailable {
		notes = append(notes, "This computer is not allowed to send ping (ICMP) packets, so devices were found with TCP checks and the ARP cache instead.")
	}
	if !sum.ARPAvailable {
		notes = append(notes, "The ARP cache could not be read, so some quiet devices and MAC addresses may be missing.")
	}
	sum.Note = strings.Join(notes, " ")
	return sum
}

// reverseLookup gets the PTR name for an address on a short leash of its own,
// so a slow or missing DNS server cannot hold up the sweep.
func reverseLookup(ctx context.Context, ip string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

func milliseconds(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}
