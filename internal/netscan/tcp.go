package netscan

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// minPortTimeout keeps the per port slice usable when a short host timeout is
// split across a long port list.
const minPortTimeout = 150 * time.Millisecond

// TCPProbe knocks on each port in turn until something proves the host exists.
// An accepted connection reports the port; a refused connection means the host
// answered with a reset, which proves it is there even though nothing is
// listening, and reports port 0.
//
// Exported alongside PingOnce so Internet Triage gets the same ICMP-then-TCP
// fallback the sweep has, rather than becoming a third copy of it.
func TCPProbe(ctx context.Context, addr netip.Addr, ports []int, timeout time.Duration) (int, time.Duration, bool) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	per := timeout / time.Duration(len(ports))
	if per < minPortTimeout {
		per = minPortTimeout
	}

	var (
		refusedAfter time.Duration
		refused      bool
	)
	for _, port := range ports {
		if ctx.Err() != nil {
			break
		}
		portCtx, portCancel := context.WithTimeout(ctx, per)
		started := time.Now()
		var d net.Dialer
		conn, err := d.DialContext(portCtx, "tcp", netip.AddrPortFrom(addr, uint16(port)).String())
		portCancel()

		if err == nil {
			conn.Close()
			return port, time.Since(started), true
		}
		if !refused && IsRefused(err) {
			refused, refusedAfter = true, time.Since(started)
		}
	}
	if refused {
		return 0, refusedAfter, true
	}
	return 0, 0, false
}

// IsRefused reports a TCP reset. Windows uses WSAECONNREFUSED rather than the
// POSIX errno, and older Go releases do not map the two, so the text is checked
// as well. Exported because the Port Scanner, Ping Monitor and Wake-on-LAN all
// need the same answer, and one copy is easier to keep right than four.
func IsRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "refused")
}
