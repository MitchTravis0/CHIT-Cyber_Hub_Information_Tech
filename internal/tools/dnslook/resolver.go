package dnslook

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"time"

	"chit/internal/core"
)

// serverAddress turns a tick box id into a dial target. The empty string means
// the system resolver and has no address.
func serverAddress(server string) (string, error) {
	if server == "" {
		return "", nil
	}
	if host, port, err := net.SplitHostPort(server); err == nil {
		if _, err := netip.ParseAddr(host); err == nil && validPort(port) {
			return net.JoinHostPort(host, port), nil
		}
	} else if _, err := netip.ParseAddr(server); err == nil {
		return net.JoinHostPort(server, "53"), nil
	}
	return "", core.Errorf(core.CodeInvalidInput,
		"Enter the DNS server as an IP address, for example 8.8.8.8 or 192.168.1.10. %q is not one.", server)
}

// validPort exists because net.SplitHostPort accepts an empty or out of range
// port, which would fail much later at dial time with nothing useful to say.
func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

// resolverFor returns the system resolver for an empty server, otherwise Go's
// own DNS client pointed at one address.
//
// The address argument handed to Dial is deliberately ignored: that is what
// makes a custom server behave identically on Windows, macOS and Linux, where
// the list Go would otherwise have chosen from comes from three different
// places.
func resolverFor(addr string, timeout time.Duration) *net.Resolver {
	if addr == "" {
		return net.DefaultResolver
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: timeout}
			return d.DialContext(ctx, network, addr)
		},
	}
}
