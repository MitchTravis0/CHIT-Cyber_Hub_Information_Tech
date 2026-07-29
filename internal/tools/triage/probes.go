package triage

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"chit/internal/dnsx"
	"chit/internal/netinfo"
	"chit/internal/netscan"
)

// maxBodyBytes caps the portal check's read. The body it expects is eight
// bytes; a captive portal can serve a whole login page, and only the first
// part of it is needed to see that it is not the expected text.
const maxBodyBytes = 64 * 1024

// Probes are the things the ladder does to the outside world. Every field has a
// default; a test replaces the ones it needs so the whole ladder can be driven
// with no network at all. That is the only way to cover the failure branches,
// which are the branches that matter.
type Probes struct {
	Adapters func() (netinfo.Report, error)
	// Ping returns the round trip in milliseconds and whether anything
	// answered.
	Ping func(ctx context.Context, ip string, timeout time.Duration) (float64, bool)
	// Dial reports whether a TCP connection to addr succeeded and how long it
	// took. A refused connection counts as an answer: something is there.
	Dial func(ctx context.Context, addr string, timeout time.Duration) (float64, bool)
	// Resolve asks server (empty for the system resolver) for the addresses of
	// name.
	Resolve func(ctx context.Context, server, name string, timeout time.Duration) ([]string, error)
	// Get fetches url without following redirects and returns the status, the
	// Location header, the body (capped) and an error.
	Get func(ctx context.Context, url string, timeout time.Duration) (int, string, string, error)
}

// DefaultProbes is the real thing. TestDefaultProbesAreWired asserts no field is
// nil, so a suite driven entirely by fakes cannot pass while the shipped tool
// does nothing.
func DefaultProbes() Probes {
	return Probes{
		Adapters: netinfo.List,
		Ping:     realPing,
		Dial:     realDial,
		Resolve:  realResolve,
		Get:      realGet,
	}
}

// realPing uses the same ICMP probe the IP Range Scanner uses, exported from
// netscan in Phase 8 rather than copied a third time.
func realPing(ctx context.Context, ip string, timeout time.Duration) (float64, bool) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return 0, false
	}
	rtt, ok, err := netscan.PingOnce(ctx, addr, timeout)
	if err != nil || !ok {
		return 0, false
	}
	return milliseconds(rtt), true
}

// realDial counts a refused connection as an answer: the machine is there and
// said so, which is what the gateway and internet rungs are asking.
func realDial(ctx context.Context, addr string, timeout time.Duration) (float64, bool) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err == nil {
		conn.Close()
		return milliseconds(time.Since(started)), true
	}
	if netscan.IsRefused(err) {
		return milliseconds(time.Since(started)), true
	}
	return 0, false
}

func realResolve(ctx context.Context, server, name string, timeout time.Duration) ([]string, error) {
	addr, err := dnsx.ServerAddress(server)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return dnsx.ResolverFor(addr, timeout).LookupHost(ctx, name)
}

// realGet never follows a redirect: a redirect on the portal check is the whole
// finding, and following it would hide it.
func realGet(ctx context.Context, url string, timeout time.Duration) (int, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{}).DialContext,
		TLSHandshakeTimeout: timeout,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", "", err
	}
	req.Header.Set("User-Agent", "CHIT")
	// A captive portal keys off a cached answer as readily as a live one, so the
	// check must not be served from anything in between.
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	return resp.StatusCode, resp.Header.Get("Location"), string(body), nil
}

// bodyMatches compares the portal check's answer against the text it must be,
// ignoring trailing whitespace: the served file ends with a newline and a proxy
// may or may not keep it.
func bodyMatches(body, want string) bool {
	return strings.TrimRight(body, " \t\r\n") == strings.TrimRight(want, " \t\r\n")
}
