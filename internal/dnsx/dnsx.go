// Package dnsx points Go's DNS client at a chosen server. It is a shared
// engine beside internal/netscan and internal/walk, not a tool package: the
// DNS Resolver Comparer, the Email DNS Checker and Internet Triage all need to
// ask one named server a question, and without this they would each carry their
// own copy of the same twenty lines and the same rejection wording.
//
// internal/tools/dnslook has an older private copy of both functions. It was
// deliberately left alone when this package was added, so that nothing already
// verified was touched in an additive phase. Collapsing the two is a job for
// whoever next has reason to edit that tool.
package dnsx

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"time"

	"chit/internal/core"
)

// DefaultPort is where a DNS server listens when the user did not say.
const DefaultPort = "53"

// ServerAddress turns a typed DNS server into a dial target. The empty string
// means the system resolver and has no address of its own.
func ServerAddress(server string) (string, error) {
	if server == "" {
		return "", nil
	}
	if host, port, err := net.SplitHostPort(server); err == nil {
		if _, err := netip.ParseAddr(host); err == nil && validPort(port) {
			return net.JoinHostPort(host, port), nil
		}
	} else if _, err := netip.ParseAddr(server); err == nil {
		return net.JoinHostPort(server, DefaultPort), nil
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

// ResolverFor returns the system resolver for an empty address, otherwise Go's
// own DNS client pointed at one address.
//
// The address argument handed to Dial is deliberately ignored: that is what
// makes a custom server behave identically on Windows, macOS and Linux, where
// the list Go would otherwise have chosen from comes from three different
// places.
func ResolverFor(addr string, timeout time.Duration) *net.Resolver {
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
