// Package discover listens for the two protocols almost every printer, TV, NAS
// and casting device shouts on the local network: multicast DNS (Bonjour) and
// SSDP (part of UPnP). It is the service layer the Device Discovery page talks
// to.
package discover

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"chit/internal/core"
)

// JobKind is the job manager kind for a discovery run.
const JobKind = "discover"

// KindDevice is the result kind every Device is emitted under.
const KindDevice = "device"

// Protocol values.
const (
	ProtocolMDNS = "mDNS"
	ProtocolSSDP = "SSDP"
)

const (
	// MDNSAddress and SSDPAddress are the two multicast groups. Neither port is
	// bound locally: the queries go out from an ephemeral port and the replies
	// come back to it.
	MDNSAddress = "224.0.0.251:5353"
	SSDPAddress = "239.255.255.250:1900"

	// DefaultTimeoutMS is how long to listen after sending the queries.
	DefaultTimeoutMS = 4000
	minTimeoutMS     = 1000
	maxTimeoutMS     = 30000

	// MaxPacketBytes is a generous ceiling for one datagram.
	MaxPacketBytes = 9000
	// MaxDevices stops a hostile or broken network filling memory. On reaching
	// it the run stops early and the summary says so.
	MaxDevices = 2000
)

// The silence sentence appears in every summary note, whatever happened,
// because it is the one thing a tech must not forget about this tool.
const silenceNote = "Multicast is commonly blocked between VLANs and on guest Wi-Fi, so silence is not proof of absence."

type Params struct {
	TimeoutMS int `json:"timeoutMs"`
}

// Device is one thing heard on the network. The same physical device usually
// produces several of these, one per service it advertises, which is correct: a
// printer that offers IPP and a web page is two answers to two questions.
type Device struct {
	// Key is protocol + ip + service + name, and is what the UI upserts by.
	Key      string `json:"key"`
	Protocol string `json:"protocol"`
	IP       string `json:"ip"`
	Name     string `json:"name"`
	// Service is the mDNS service type without the trailing ".local", or the
	// SSDP search target.
	Service string `json:"service"`
	// Host is the SRV target or the host part of the SSDP LOCATION.
	Host string `json:"host"`
	Port int    `json:"port"`
	// Details is a one line summary of what the device said about itself.
	Details string `json:"details"`
	// Adapter is the interface the reply arrived on.
	Adapter string `json:"adapter"`
}

// newDevice fills in the upsert key. Every Device must go through here, or two
// emissions of the same thing will not collapse in the UI.
func newDevice(d Device) Device {
	d.Key = strings.Join([]string{d.Protocol, d.IP, d.Service, d.Name}, "|")
	return d
}

// Sink is where a run reports to. The job wires it to jc.Emit and jc.Progress;
// a test wires it to a slice, which is the only way to read what a run emitted
// without a Wails runtime.
type Sink struct {
	Emit     func(Device)
	Progress func(done int, message string)
}

// Service owns the discovery entry point. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

func (p Params) normalize() (time.Duration, error) {
	ms := p.TimeoutMS
	if ms == 0 {
		ms = DefaultTimeoutMS
	}
	if ms < minTimeoutMS || ms > maxTimeoutMS {
		return 0, core.Errorf(core.CodeInvalidInput,
			"The listening time must be between 1 and 30 seconds. %d ms is outside that.", ms)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// StartDiscovery begins a run and returns the job id at once. Devices arrive as
// "device" items on job:result as they are heard.
func (s *Service) StartDiscovery(p Params) (string, error) {
	window, err := p.normalize()
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		return runDiscovery(jc, window)
	}), nil
}

// runDiscovery is the body of the job, named so it is not stranded at 0%
// coverage inside an anonymous closure.
func runDiscovery(jc *core.JobContext, window time.Duration) error {
	summary, err := Discover(jc.Ctx(), window, Sink{
		Emit:     func(d Device) { jc.Emit(KindDevice, d) },
		Progress: func(done int, message string) { jc.Progress(done, 0, message) },
	})
	if err != nil {
		return err
	}
	jc.SetSummary(summary)
	return nil
}

// Discover asks on every usable interface at once and reports what answers.
func Discover(ctx context.Context, window time.Duration, out Sink) (map[string]any, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, core.Errorf(core.CodeInternal,
			"Could not read this computer's network adapters. Check that the network service is running.")
	}
	usable := UsableInterfaces(ifaces, interfaceAddrs)
	if len(usable) == 0 {
		return nil, core.Errorf(core.CodeNetwork,
			"This computer has no network adapter that can send multicast, so there is nothing to listen on. Connect to a network and try again.")
	}

	deadline := time.Now().Add(window)
	collector := &collector{out: out, adapters: len(usable)}

	for range core.Pool(ctx, usable, len(usable), func(c context.Context, in Interface) (struct{}, bool) {
		probeInterface(c, in, deadline, collector)
		return struct{}{}, false
	}) {
		// probeInterface reports through the collector, so nothing comes back
		// through the channel. Ranging is how the pool is waited on.
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return collector.summary(), nil
}

// Interface is one adapter worth asking on: its name and the address to send
// from.
type Interface struct {
	Name string
	IP   string
}

// UsableInterfaces picks the adapters that are up, are not loopback, support
// multicast and have an IPv4 address. It takes the address lookup as an
// argument so it can be table tested without this machine's real adapters.
func UsableInterfaces(ifaces []net.Interface, addrs func(net.Interface) ([]net.Addr, error)) []Interface {
	var out []Interface
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 ||
			ifi.Flags&net.FlagLoopback != 0 ||
			ifi.Flags&net.FlagMulticast == 0 {
			continue
		}
		list, err := addrs(ifi)
		if err != nil {
			continue
		}
		for _, addr := range list {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			out = append(out, Interface{Name: ifi.Name, IP: ipnet.IP.String()})
			break
		}
	}
	return out
}

func interfaceAddrs(ifi net.Interface) ([]net.Addr, error) { return ifi.Addrs() }

// probeInterface sends both queries from a socket bound to the adapter's own
// address, then listens on three sockets at once until the deadline.
//
// Three, because the two protocols answer in different places, and a live run
// against a real network proved it:
//
//   - mDNS responders send their answers to the multicast group, not back to
//     the port that asked, even though every question carries the QU bit asking
//     them to. On a real network the unicast socket received **nothing** while
//     a socket joined to the group received replies from a dozen devices.
//   - SSDP M-SEARCH responses do come back unicast to the source port, so that
//     socket still has to be read.
//   - SSDP NOTIFY announcements go to the group, like mDNS.
//
// net.ListenMulticastUDP is standard library (it is in net, not in
// golang.org/x/net) and sets SO_REUSEADDR, so joining the group works alongside
// the mDNSResponder on macOS or the Avahi daemon on Linux that already owns
// port 5353. Binding the send socket to the adapter address is how the outgoing
// interface is chosen: the textbook multicast-interface socket option lives in
// golang.org/x/net/ipv4, which is an indirect dependency CHIT must not make
// direct.
func probeInterface(ctx context.Context, in Interface, deadline time.Time, c *collector) {
	conn, err := net.ListenPacket("udp4", net.JoinHostPort(in.IP, "0"))
	if err != nil {
		c.sendFailed()
		return
	}
	defer conn.Close()

	sent := false
	if query, err := mdnsQuery(); err == nil {
		if _, err := writeTo(conn, query, MDNSAddress); err == nil {
			sent = true
		}
	}
	if _, err := writeTo(conn, []byte(ssdpSearch), SSDPAddress); err == nil {
		sent = true
	}
	if !sent {
		// An adapter that will not send is worth reporting: a VPN client or a
		// virtual adapter blocking multicast is a common and invisible cause of
		// an empty list.
		c.sendFailed()
	}

	sockets := []net.PacketConn{conn}
	for _, group := range []string{MDNSAddress, SSDPAddress} {
		// A group that will not join is not fatal: the other sockets still
		// listen, and on some adapters (a container bridge, a VPN tunnel) the
		// join is expected to fail.
		if joined := joinGroup(in.Name, group); joined != nil {
			sockets = append(sockets, joined)
			defer joined.Close()
		}
	}

	var wg sync.WaitGroup
	for _, socket := range sockets {
		wg.Add(1)
		go func(s net.PacketConn) {
			defer wg.Done()
			listen(ctx, s, in, deadline, c)
		}(socket)
	}
	wg.Wait()
}

// joinGroup opens a socket joined to a multicast group on one interface, or nil
// when the join is not possible.
func joinGroup(name, address string) net.PacketConn {
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return nil
	}
	addr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil
	}
	conn, err := net.ListenMulticastUDP("udp4", ifi, addr)
	if err != nil {
		return nil
	}
	return conn
}

func writeTo(conn net.PacketConn, payload []byte, address string) (int, error) {
	addr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return 0, err
	}
	return conn.WriteTo(payload, addr)
}

// listen reads replies until the deadline, the context ends, or the device cap
// is reached. A malformed packet is skipped: one broken device must never end
// the run.
func listen(ctx context.Context, conn net.PacketConn, in Interface, deadline time.Time, c *collector) {
	buf := make([]byte, MaxPacketBytes)
	for {
		if ctx.Err() != nil || c.full() {
			return
		}
		now := time.Now()
		if !now.Before(deadline) {
			return
		}
		// A short read deadline keeps the loop responsive to cancellation
		// without needing a second goroutine to close the socket.
		next := now.Add(250 * time.Millisecond)
		if next.After(deadline) {
			next = deadline
		}
		_ = conn.SetReadDeadline(next)

		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			continue
		}
		src := hostOf(from)
		payload := buf[:n]

		// mDNS replies are binary and SSDP replies are text, so the first byte
		// of an SSDP reply is a letter and an mDNS header never starts a line
		// with one.
		if looksLikeSSDP(payload) {
			c.add(devicesFromSSDP(payload, src, in.Name))
			continue
		}
		c.add(devicesFromMDNS(payload, src, in.Name))
	}
}

func looksLikeSSDP(payload []byte) bool {
	return len(payload) > 4 &&
		(payload[0] == 'H' || payload[0] == 'N' || payload[0] == 'M')
}

func hostOf(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

// mdnsCount and ssdpCount are tallied per protocol so the summary can say which
// of the two produced anything, which is the first thing to check when one is
// blocked and the other is not.
type collector struct {
	out      Sink
	adapters int

	mu        sync.Mutex
	seen      map[string]bool
	mdns      int
	ssdp      int
	failures  int
	truncated bool
}

func (c *collector) add(devices []Device) {
	for _, d := range devices {
		c.mu.Lock()
		if c.seen == nil {
			c.seen = map[string]bool{}
		}
		if len(c.seen) >= MaxDevices {
			c.truncated = true
			c.mu.Unlock()
			return
		}
		first := !c.seen[d.Key]
		c.seen[d.Key] = true
		if first {
			if d.Protocol == ProtocolMDNS {
				c.mdns++
			} else {
				c.ssdp++
			}
		}
		total := len(c.seen)
		c.mu.Unlock()

		// Emitted every time, even for a key already seen: a later reply may
		// carry more (a port, a TXT line) than the first one did, and the UI
		// upserts by key.
		c.out.Emit(d)
		c.out.Progress(total, fmt.Sprintf("Listening on %d %s, %d %s so far",
			c.adapters, plural(c.adapters, "adapter", "adapters"),
			total, plural(total, "device", "devices")))
	}
}

func (c *collector) sendFailed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures++
}

func (c *collector) full() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen) >= MaxDevices
}

func (c *collector) summary() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]any{
		"devices":      len(c.seen),
		"mdns":         c.mdns,
		"ssdp":         c.ssdp,
		"adapters":     c.adapters,
		"sendFailures": c.failures,
		"truncated":    c.truncated,
		"note":         note(len(c.seen), c.adapters, c.failures, c.truncated),
	}
}

// note always carries the silence sentence, whatever else it says.
func note(devices, adapters, failures int, truncated bool) string {
	var parts []string
	switch {
	case failures >= adapters && adapters > 0:
		parts = append(parts,
			"None of this computer's adapters would send the questions, so nothing could answer. Multicast is often blocked by a VPN client or a virtual adapter. Silence here is not proof there are no devices.")
	case devices == 0:
		parts = append(parts,
			"Nothing answered. That does not mean the network is empty: a device that does not advertise itself stays invisible, and "+
				strings.ToLower(silenceNote[:1])+silenceNote[1:])
	default:
		parts = append(parts, "Only devices that advertise themselves appear here. "+silenceNote)
	}
	if failures > 0 && failures < adapters {
		parts = append(parts, fmt.Sprintf("%d of %d adapters would not send the questions.", failures, adapters))
	}
	if truncated {
		parts = append(parts, fmt.Sprintf("Stopped after %d devices.", MaxDevices))
	}
	return strings.Join(parts, " ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
