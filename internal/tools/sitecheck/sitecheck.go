// Package sitecheck answers two questions in one request: does this address
// answer, and is its certificate about to expire. It is the service layer the
// Website / Service Up Checker page talks to.
package sitecheck

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"

	"chit/internal/core"
)

const (
	defaultTimeoutMS = 10000
	maxTimeoutMS     = 60000
	dialTimeout      = 5 * time.Second
)

// Params is what the UI sends.
type Params struct {
	// URL as typed. A missing scheme becomes https://, see NormalizeURL.
	URL string `json:"url"`
	// TimeoutMS is the budget for the whole check. 0 means 10000.
	TimeoutMS int `json:"timeoutMs"`
	// FollowRedirects walks the chain to the end. Off shows the first hop only.
	FollowRedirects bool `json:"followRedirects"`
}

// Result is one check. Every field is filled in as far as the check got, so a
// failure still shows the DNS time and the certificate when there was one.
type Result struct {
	URL        string `json:"url"`
	FinalURL   string `json:"finalUrl"`
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`

	// Level is "ok", "warn" or "error" and drives the colour of the whole card.
	Level string `json:"level"`
	// Headline is one sentence a junior can read out loud.
	Headline string `json:"headline"`

	IP           string   `json:"ip"`
	ServerHeader string   `json:"serverHeader"`
	BodyBytes    int64    `json:"bodyBytes"`
	Redirects    []Hop    `json:"redirects"`
	Timing       Timing   `json:"timing"`
	TLS          *TLSInfo `json:"tls"`
	Warnings     []string `json:"warnings"`
	Errors       []string `json:"errors"`
	CheckedAt    string   `json:"checkedAt"`

	// failure and followed are what Classify needs and the UI does not, so they
	// stay out of the JSON contract.
	failure  failure
	followed bool
}

// Hop is one step of a redirect chain.
type Hop struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Location string `json:"location"`
}

// Timing is where the time went on the final connection, except Total which
// covers the whole chain. Any stage that did not happen is 0.
type Timing struct {
	DNSMs     int64 `json:"dnsMs"`
	ConnectMs int64 `json:"connectMs"`
	TLSMs     int64 `json:"tlsMs"`
	TTFBMs    int64 `json:"ttfbMs"`
	TotalMs   int64 `json:"totalMs"`
}

// Check fetches one URL and reports what happened. A site that is down is a
// successful check with a bad answer, so it comes back as a Result with
// Level "error"; only bad parameters produce a Go error.
func Check(ctx context.Context, p Params) (Result, error) {
	target, err := validate(&p)
	if err != nil {
		return Result{}, err
	}
	if ctx.Err() != nil {
		return Result{}, core.Errorf(core.CodeInternal,
			"CHIT was closing down, so the check did not run. Try it again.")
	}

	// NormalizeURL has already proved this parses.
	u, _ := url.Parse(target)

	result := Result{
		URL:       target,
		FinalURL:  target,
		Redirects: []Hop{},
		Warnings:  []string{},
		Errors:    []string{},
		followed:  p.FollowRedirects,
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutMS)*time.Millisecond)
	defer cancel()

	started := time.Now()

	if u.Scheme == "https" {
		// The certificate described is the one presented by the address the tech
		// typed, not by whatever a redirect chain ends at, because that is the
		// address they asked about. A failed dial leaves TLS null on purpose:
		// the request below hits the same problem and its error is what gets
		// reported.
		result.TLS, _ = InspectTLS(ctx, u.Hostname(), portOf(u))
	}

	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
		TLSHandshakeTimeout: dialTimeout,
		ForceAttemptHTTP2:   true,
		// the point of this tool is to show a bad certificate, not to refuse it:
		// the printer or firewall with a self-signed certificate must still
		// report its status, and the certificate verdict comes from InspectTLS.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	defer transport.CloseIdleConnections()

	tr := &tracker{}
	resp, hops, err := fetch(ctx, transport, target, p.FollowRedirects, tr)
	result.Redirects = hops
	if err == nil {
		result.Status = resp.StatusCode
		result.StatusText = resp.Status
		result.ServerHeader = resp.Header.Get("Server")
		result.FinalURL = resp.Request.URL.String()
		result.BodyBytes = drain(resp)
	}
	result.failure = failureOf(err)
	result.Timing = tr.timing(time.Since(started))
	result.IP = tr.ip()

	now := time.Now()
	Classify(&result)
	result.CheckedAt = now.Format(time.RFC3339)
	return result, nil
}

// validate catches everything a user can get wrong before any network call, and
// fills in the default wait.
func validate(p *Params) (string, error) {
	if strings.TrimSpace(p.URL) == "" {
		return "", core.Errorf(core.CodeInvalidInput,
			"Enter an address to check, for example https://example.com.")
	}
	target, err := NormalizeURL(p.URL)
	if err != nil {
		return "", err
	}
	if p.TimeoutMS < 0 || p.TimeoutMS > maxTimeoutMS {
		return "", core.Errorf(core.CodeInvalidInput,
			"The wait must be between 1 and 60 seconds. %d ms is outside that.", p.TimeoutMS)
	}
	if p.TimeoutMS == 0 {
		p.TimeoutMS = defaultTimeoutMS
	}
	return target, nil
}

func fetch(ctx context.Context, transport *http.Transport, target string, follow bool, tr *tracker) (*http.Response, []Hop, error) {
	hops := []Hop{}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 && req.Response != nil {
				previous := via[len(via)-1]
				hops = append(hops, Hop{
					URL:      previous.URL.String(),
					Status:   req.Response.StatusCode,
					Location: req.Response.Header.Get("Location"),
				})
			}
			if !follow {
				return http.ErrUseLastResponse
			}
			if len(via) >= maxRedirects {
				return errTooManyRedirects
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(ctx, tr.trace()), http.MethodGet, target, nil)
	if err != nil {
		return nil, hops, err
	}
	req.Header.Set("User-Agent", "CHIT")
	req.Header.Set("Accept", "*/*")
	// identity so BodyBytes is the real size rather than a compressed one.
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		return nil, hops, err
	}
	return resp, hops, nil
}

// drain reads the body far enough to measure it and stops at the cap, so a
// large download does not become the check.
func drain(resp *http.Response) int64 {
	read, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
	resp.Body.Close()
	return read
}

func portOf(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// tracker records where the time went. A redirect chain reuses one trace, so
// each hook overwrites its previous value and the numbers describe the last
// connection made, which is the one the user cares about.
type tracker struct {
	mu           sync.Mutex
	requestStart time.Time
	dnsStart     time.Time
	connectStart time.Time
	tlsStart     time.Time
	dns          time.Duration
	connect      time.Duration
	handshake    time.Duration
	ttfb         time.Duration
	remote       string
}

func (t *tracker) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(string) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.requestStart = time.Now()
		},
		DNSStart: func(httptrace.DNSStartInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.dnsStart = time.Now()
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.dnsStart.IsZero() {
				t.dns = time.Since(t.dnsStart)
			}
		},
		ConnectStart: func(string, string) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.connectStart = time.Now()
		},
		ConnectDone: func(string, string, error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.connectStart.IsZero() {
				t.connect = time.Since(t.connectStart)
			}
		},
		TLSHandshakeStart: func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.tlsStart = time.Now()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.tlsStart.IsZero() {
				t.handshake = time.Since(t.tlsStart)
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.remote = info.Conn.RemoteAddr().String()
		},
		GotFirstResponseByte: func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.requestStart.IsZero() {
				t.ttfb = time.Since(t.requestStart)
			}
		},
	}
}

func (t *tracker) timing(total time.Duration) Timing {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Timing{
		DNSMs:     t.dns.Milliseconds(),
		ConnectMs: t.connect.Milliseconds(),
		TLSMs:     t.handshake.Milliseconds(),
		TTFBMs:    t.ttfb.Milliseconds(),
		TotalMs:   total.Milliseconds(),
	}
}

func (t *tracker) ip() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	host, _, err := net.SplitHostPort(t.remote)
	if err != nil {
		return t.remote
	}
	return host
}
