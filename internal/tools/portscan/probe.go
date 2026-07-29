package portscan

import (
	"context"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"chit/internal/netscan"
)

const (
	// bannerWait is how long a service gets to say hello. Plenty of them never
	// do, so this is spent in full on every open port when banners are on.
	bannerWait = 1500 * time.Millisecond
	bannerSize = 512
	maxBanner  = 200
)

// probe decides one port. An accepted connection is open, a refused connection
// is closed (which proves the host is there), and silence is filtered.
func probe(ctx context.Context, ip string, port int, timeout time.Duration, banners bool) Result {
	out := Result{Port: port, State: StateFiltered, Service: serviceName(port)}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		if netscan.IsRefused(err) {
			out.State = StateClosed
			out.LatencyMS = milliseconds(time.Since(started))
		}
		return out
	}
	defer conn.Close()

	out.State = StateOpen
	out.LatencyMS = milliseconds(time.Since(started))
	if banners {
		out.Banner = readBanner(conn)
	}
	return out
}

// readBanner takes whatever the service says first. A read timeout is the
// normal outcome for anything that waits to be asked, such as a web server.
func readBanner(conn net.Conn) string {
	_ = conn.SetReadDeadline(time.Now().Add(bannerWait))
	buf := make([]byte, bannerSize)
	n, _ := conn.Read(buf)
	return sanitizeBanner(string(buf[:n]))
}

// sanitizeBanner keeps the printable ASCII of a greeting and throws away the
// control characters and binary a non-text service sends, so one line of it can
// go straight into a table cell.
func sanitizeBanner(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	gap := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7E {
			gap = true
			continue
		}
		if c == ' ' {
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteByte(' ')
		}
		gap = false
		b.WriteByte(c)
	}

	out := b.String()
	if len(out) > maxBanner {
		out = out[:maxBanner] + "..."
	}
	return out
}

func milliseconds(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}
