package pingmon

import (
	"context"
	"math"
	"net"
	"runtime"
	"strconv"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"chit/internal/netscan"
)

// pingOnce sends a single ICMP echo. A non-nil error means the packet could not
// be sent at all (blocked socket, no permission); ok=false with no error means
// nothing answered in time.
//
// This duplicates internal/netscan/icmp.go rather than calling into it because
// that one takes a netip.Addr and this tool holds the target as a string it
// resolved once. Unifying the two means changing a shared Phase 1 engine, which
// is a decision for the repo owner, not a side effect of adding a tool.
func pingOnce(ctx context.Context, ip string, timeout time.Duration) (time.Duration, bool, error) {
	p := probing.New(ip)
	p.SetIPAddr(&net.IPAddr{IP: net.ParseIP(ip)})
	// Linux and macOS hand out unprivileged datagram ICMP sockets, so no root is
	// needed. Windows has no such socket type but does let a normal user open a
	// raw ICMP socket, so the raw path is the unprivileged one there.
	p.SetPrivileged(runtime.GOOS == "windows")
	p.SetLogger(probing.NoopLogger{})
	p.Count = 1
	p.Timeout = timeout
	p.Interval = timeout
	p.RecordRtts = false
	p.RecordTTLs = false

	if err := p.RunWithContext(ctx); err != nil {
		return 0, false, err
	}
	stats := p.Statistics()
	if stats.PacketsRecv == 0 {
		return 0, false, nil
	}
	return stats.AvgRtt, true, nil
}

// tcpPing times a TCP handshake. A refused connection counts as a reply: the
// host answered, which is what the measurement is for.
func tcpPing(ctx context.Context, ip string, port int, timeout time.Duration) (time.Duration, bool) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err == nil {
		conn.Close()
		return time.Since(started), true
	}
	if netscan.IsRefused(err) {
		return time.Since(started), true
	}
	return 0, false
}

func milliseconds(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}
