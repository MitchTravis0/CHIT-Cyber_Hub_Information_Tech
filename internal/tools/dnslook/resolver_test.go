package dnslook

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

func TestServerAddress(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"system resolver", "", "", false},
		{"bare ipv4", "8.8.8.8", "8.8.8.8:53", false},
		{"ipv4 with a port", "8.8.8.8:5353", "8.8.8.8:5353", false},
		{"bare ipv6", "2001:4860:4860::8888", "[2001:4860:4860::8888]:53", false},
		{"ipv6 with a port", "[2001:db8::1]:53", "[2001:db8::1]:53", false},
		{"a name", "dns.google", "", true},
		{"empty port", "8.8.8.8:", "", true},
		{"port out of range", "8.8.8.8:70000", "", true},
		{"not an address", "not an ip", "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := serverAddress(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("serverAddress(%q) = %q, want an error", c.in, got)
				}
				if code := core.CodeOf(err); code != core.CodeInvalidInput {
					t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
				}
				if !strings.Contains(err.Error(), "Enter the DNS server as an IP address") {
					t.Errorf("message %q does not say what to type instead", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("serverAddress(%q) returned %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("serverAddress(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestResolverForSystem(t *testing.T) {
	if got := resolverFor("", time.Second); got != net.DefaultResolver {
		t.Errorf("resolverFor(\"\") = %p, want the system resolver", got)
	}
}

// TestResolverForCustomDialsTheGivenAddress pins the behaviour the whole tool
// rests on: the address Go would have chosen is ignored and the question goes
// to the server the tech ticked. Loopback only, no network.
func TestResolverForCustomDialsTheGivenAddress(t *testing.T) {
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen on loopback: %v", err)
	}
	defer listener.Close()

	got := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 512)
		if _, _, err := listener.ReadFrom(buf); err == nil {
			got <- struct{}{}
		}
	}()

	r := resolverFor(listener.LocalAddr().String(), 300*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Rooted, so Go's resolver cannot reject or expand the name before dialling.
	// Nothing answers the query, so the lookup itself is left to time out.
	go func() { _, _ = r.LookupIP(ctx, "ip4", "example.com.") }()

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("the resolver never sent a query to the address it was given")
	}
}
