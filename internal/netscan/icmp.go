package netscan

import (
	"context"
	"net"
	"net/netip"
	"runtime"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

// privilegedPing decides which socket the ping uses.
//
// Linux and macOS hand out unprivileged datagram ICMP sockets ("udp" here), so
// no root is needed. Windows has no such socket type, but it does let a normal
// user open a raw ICMP socket, so the raw path is the unprivileged one there.
func privilegedPing() bool { return runtime.GOOS == "windows" }

// PingOnce sends a single echo request. A non-nil error means the ping could
// not be sent at all (blocked socket, no permission); a false ok with no error
// simply means nothing answered in time.
//
// Exported so Internet Triage can reach the gateway with the same probe the
// sweep uses, rather than becoming a third copy of it.
func PingOnce(ctx context.Context, addr netip.Addr, timeout time.Duration) (time.Duration, bool, error) {
	p := probing.New(addr.String())
	p.SetIPAddr(&net.IPAddr{IP: net.IP(addr.AsSlice())})
	p.SetPrivileged(privilegedPing())
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
