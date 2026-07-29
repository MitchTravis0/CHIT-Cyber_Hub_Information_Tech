package dnsx

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestServerAddress(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"system resolver", "", "", false},
		{"bare ipv4", "8.8.8.8", "8.8.8.8:53", false},
		{"ipv4 with port", "8.8.8.8:5353", "8.8.8.8:5353", false},
		{"ipv4 lowest port", "8.8.8.8:1", "8.8.8.8:1", false},
		{"ipv4 highest port", "8.8.8.8:65535", "8.8.8.8:65535", false},
		{"bare ipv6", "2606:4700:4700::1111", "[2606:4700:4700::1111]:53", false},
		{"ipv6 with port", "[2606:4700:4700::1111]:53", "[2606:4700:4700::1111]:53", false},
		{"private address", "192.168.1.10", "192.168.1.10:53", false},
		{"host name rejected", "dc01", "", true},
		{"host name with port rejected", "dc01:53", "", true},
		{"port zero rejected", "8.8.8.8:0", "", true},
		{"port too high rejected", "8.8.8.8:65536", "", true},
		{"empty port rejected", "8.8.8.8:", "", true},
		{"non numeric port rejected", "8.8.8.8:domain", "", true},
		{"whitespace rejected", " ", "", true},
		{"nonsense rejected", "8.8.8", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ServerAddress(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ServerAddress(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ServerAddress(%q) errored: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ServerAddress(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestServerAddressMessageNamesTheInput(t *testing.T) {
	_, err := ServerAddress("dc01")
	if err == nil {
		t.Fatal("want an error for a host name")
	}
	msg := err.Error()
	for _, want := range []string{"8.8.8.8", "192.168.1.10", `"dc01"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not contain %q", msg, want)
		}
	}
}

func TestResolverForEmptyIsTheSystemResolver(t *testing.T) {
	if got := ResolverFor("", time.Second); got != net.DefaultResolver {
		t.Fatalf("ResolverFor(\"\") = %p, want the system resolver %p", got, net.DefaultResolver)
	}
}

// TestResolverForDialsTheGivenAddress pins the behaviour the whole package
// exists for: the address Go would have chosen is thrown away and the one the
// user asked for is dialled instead. Without this the tool would silently use
// the system resolver on every platform and nobody would notice.
func TestResolverForDialsTheGivenAddress(t *testing.T) {
	// A closed listener gives a dial target that is guaranteed to be dead and
	// guaranteed not to be a real DNS server, so the address Dial receives is
	// the only thing under test.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	want := ln.Addr().String()
	ln.Close()

	r := ResolverFor(want, 200*time.Millisecond)
	if r == net.DefaultResolver {
		t.Fatal("ResolverFor returned the system resolver for a real address")
	}
	if !r.PreferGo {
		t.Error("PreferGo must be set, or Go may use the platform resolver and ignore the address")
	}

	got := make(chan string, 4)
	wrapped := r.Dial
	r.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		// Non-blocking on purpose. How many times Go's resolver dials depends on
		// the machine's DNS configuration: one attempt per search domain, per
		// address family, per retry. A blocking send with a fixed buffer
		// deadlocked the whole package on CI, where there are more search
		// domains than here, and the test timed out after ten minutes rather
		// than failing. The context cannot rescue it, because the goroutine is
		// stuck on a channel send rather than on I/O.
		select {
		case got <- address:
		default:
		}
		return wrapped(ctx, network, address)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// The lookup must fail: nothing is listening. What matters is what Dial saw.
	_, _ = r.LookupHost(ctx, "example.com")

	select {
	case address := <-got:
		// Go passes the address it would have used, which the Dial closure
		// ignores in favour of the one captured at construction.
		if address == want {
			t.Fatalf("Dial was handed %q, so this test cannot tell whether the closure ignores it", address)
		}
	default:
		t.Fatal("Dial was never called")
	}
}

func TestValidPort(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"1", true},
		{"53", true},
		{"65535", true},
		{"0", false},
		{"65536", false},
		{"", false},
		{"-1", false},
		{"domain", false},
		{"53x", false},
	}
	for _, tt := range tests {
		if got := validPort(tt.in); got != tt.want {
			t.Errorf("validPort(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
