// Package pubip asks an outside service what address this site appears from,
// and who owns it. It is the service layer the Public IP page talks to, and one
// of the few parts of CHIT that talks to the internet.
package pubip

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"chit/internal/core"
)

const (
	providerTimeout = 8 * time.Second
	v6Timeout       = 5 * time.Second
	rdnsTimeout     = 3 * time.Second
	userAgent       = "CHIT"
)

// maxBody caps what is read from a provider, so a captive portal that answers
// with a whole web page cannot be read into memory.
const maxBody = 64 << 10

// Info is what the page shows. Every string field is empty rather than a guess
// when the provider did not supply it.
type Info struct {
	IPv4        string  `json:"ipv4"`
	IPv6        string  `json:"ipv6"`
	ReverseDNS  string  `json:"reverseDns"`
	ISP         string  `json:"isp"`
	ASN         string  `json:"asn"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	Country     string  `json:"country"`
	CountryName string  `json:"countryName"`
	Timezone    string  `json:"timezone"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	// Source is the host that answered, e.g. "ipinfo.io". Shown in the UI so a
	// tech knows who was asked.
	Source string `json:"source"`
	// Partial is true when only the address is known because the detailed
	// providers were unreachable.
	Partial bool `json:"partial"`
	// Note is one plain sentence, empty when there is nothing to explain.
	Note string `json:"note"`
	// CheckedAt is RFC3339 local time, so a screenshot is self-dating.
	CheckedAt string `json:"checkedAt"`
}

// Provider is one service that can answer "what is my address". Providers are
// tried in order, so the first entry is the one with the most detail.
type Provider struct {
	Name string
	URL  string
	// Parse turns the response body into an Info. It must not touch the network.
	Parse func(body []byte) (Info, error)
}

type Client struct {
	HTTP      *http.Client
	Providers []Provider
	// V6URL is fetched over IPv6 only, purely to learn the IPv6 address.
	V6URL string
	// Resolver does the reverse lookup. DefaultClient sets the system resolver;
	// a nil Resolver skips the lookup, which is what the tests use so none of
	// them needs DNS.
	Resolver *net.Resolver

	// v6HTTP dials over IPv6 only. Nil outside DefaultClient, which is what
	// makes the IPv6 probe skippable in tests.
	v6HTTP *http.Client
}

// DefaultClient is the client the bound method uses: ipinfo.io first for the
// full detail, ipwho.is when it does not answer, and Cloudflare's trace
// endpoint as an address-only last resort.
func DefaultClient() *Client {
	return &Client{
		HTTP: familyClient("tcp4"),
		Providers: []Provider{
			{Name: "ipinfo.io", URL: "https://ipinfo.io/json", Parse: parseIPInfo},
			{Name: "ipwho.is", URL: "https://ipwho.is/", Parse: parseIPWho},
			{Name: "cloudflare.com", URL: "https://www.cloudflare.com/cdn-cgi/trace", Parse: parseCloudflareTrace},
		},
		V6URL:    "https://ipinfo.io/json",
		Resolver: net.DefaultResolver,
		v6HTTP:   familyClient("tcp6"),
	}
}

// familyClient pins every connection to one address family, because the whole
// point of the IPv6 probe is to learn what a v6-only path looks like rather
// than to let the stack pick whichever family answers first.
func familyClient(network string) *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return d.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout: 5 * time.Second,
		ForceAttemptHTTP2:   true,
	}}
}

// Lookup asks the providers in order and returns the first answer, filling in
// the IPv6 address and the reverse DNS name when they are available.
func (c *Client) Lookup(ctx context.Context) (Info, error) {
	info, answered := c.detail(ctx)
	c.probeV6(ctx, &info)

	if !answered && info.IPv6 == "" {
		if ctx.Err() != nil {
			return Info{}, core.Errorf(core.CodeTimeout,
				"The public IP lookup took too long and was stopped. The connection may be very slow or blocked.")
		}
		return Info{}, core.Errorf(core.CodeNetwork,
			"Could not reach any of the public IP services. Check that this machine has internet access, and that a proxy or firewall is not blocking it.")
	}

	if info.IPv4 != "" {
		info.ReverseDNS = c.reverseDNS(ctx, info.IPv4)
	}
	info.CheckedAt = time.Now().Format(time.RFC3339)

	if info.IPv4 == "" && info.IPv6 != "" {
		info.Note = "This connection reached the service over IPv6 only, so there is no public IPv4 address to show."
	}
	if info.ISP == "" && !info.Partial && info.Note == "" {
		info.Note = "The service knew the address but not the ISP behind it. That is normal on a business line whose range is registered to a reseller."
	}
	return info, nil
}

// detail tries every provider in order and keeps the first usable answer.
func (c *Client) detail(ctx context.Context) (Info, bool) {
	for _, p := range c.Providers {
		info, err := c.fetch(ctx, p)
		if err != nil {
			continue
		}
		info.Source = p.Name
		return info, true
	}
	return Info{}, false
}

// fetch is a function rather than the body of the loop so its timeout is
// cancelled at the end of each attempt instead of at the end of the lookup.
func (c *Client) fetch(ctx context.Context, p Provider) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()

	body, err := get(ctx, c.HTTP, p.URL)
	if err != nil {
		return Info{}, err
	}
	return p.Parse(body)
}

// probeV6 fills in the IPv6 address. Failing is normal: most sites have no
// IPv6 at all, so nothing here can fail the lookup.
func (c *Client) probeV6(ctx context.Context, info *Info) {
	if c.v6HTTP == nil || c.V6URL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, v6Timeout)
	defer cancel()

	body, err := get(ctx, c.v6HTTP, c.V6URL)
	if err != nil {
		return
	}
	var found struct {
		IP string `json:"ip"`
	}
	if json.Unmarshal(body, &found) != nil {
		return
	}
	if _, err := netip.ParseAddr(found.IP); err != nil {
		return
	}
	// Some stacks answer the v6 URL over v4, which would show the same address
	// twice and suggest IPv6 works when it does not.
	if found.IP == info.IPv4 {
		return
	}
	info.IPv6 = found.IP
}

func (c *Client) reverseDNS(ctx context.Context, ip string) string {
	if c.Resolver == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, rdnsTimeout)
	defer cancel()

	names, err := c.Resolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

func get(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, errProvider
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}
