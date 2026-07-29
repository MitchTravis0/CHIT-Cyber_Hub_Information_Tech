// Package urlcheck follows a suspicious link to its real destination without
// loading, running or rendering anything on the way: every address in the chain
// is only asked where it points next. Nothing in a response body is parsed, so a
// JavaScript or meta-refresh redirect is invisible here and the UI says so.
package urlcheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"chit/internal/core"
)

const (
	// maxHops caps the chain. Ten is more than any legitimate link needs and
	// short enough that a redirect loop is obvious rather than slow.
	maxHops = 10
	// maxBody is what a GET is allowed to read before the body is thrown away.
	// The contents are never looked at; this exists so the connection closes
	// cleanly rather than being abandoned mid-transfer.
	maxBody = 64 << 10
	// userAgent says what this is. A bare "CHIT" is refused by several CDNs,
	// and impersonating a real browser version exactly would be a lie.
	userAgent = "Mozilla/5.0 (compatible; CHIT Link Inspector)"
	// perHop is the ceiling on any single request.
	perHop = 5 * time.Second
	// rdapTimeout is the ceiling on the domain age lookup, which is best effort.
	rdapTimeout = 5 * time.Second
	// rdapBase is a public RDAP front end that redirects to the right registry.
	rdapBase = "https://rdap.org/domain/"
	// newDomainDays is the age below which a domain counts as "very new".
	// Phishing domains are registered days to weeks before a campaign and taken
	// down within weeks, while a real business domain is almost always older.
	newDomainDays = 90

	defaultTimeoutMS = 15000
	maxTimeoutMS     = 60000
)

// The sentences a hop failure turns into. Raw Go error text is never shown.
const (
	msgBlockedHop  = "That address is on this computer or on the local network, so CHIT did not connect to it. A link from outside should never point there."
	msgNoName      = "That name could not be looked up, so there is nothing at the other end of this link. A domain that has already been taken down often looks like this."
	msgRefused     = "Nothing is listening at that address. The site may already have been taken down."
	msgTimedOut    = "That address did not answer in time, so the chain stops here."
	msgTLSFailed   = "The secure connection to that address could not be set up, so the chain stops here."
	msgUnreachable = "That address could not be reached, so the chain stops here."
	msgNoLocation  = "That address said it was redirecting but did not say where to, so the chain stops here."
	msgBadLocation = "That address gave a redirect CHIT could not make sense of, so the chain stops here."
)

// The sentences that explain why the walk ended early.
const (
	stoppedPrivate  = "The chain stopped there, because the next address is on this computer or on the local network."
	stoppedNotWeb   = "The chain was not followed past that point, because the next address is not a web address."
	stoppedLoop     = "The link redirects back to an address it has already visited, so it goes round in a circle."
	stoppedHopCap   = "The link kept redirecting and CHIT stopped after %d hops."
	stoppedNoAnswer = "The chain stopped there, because that address did not answer."
)

// errBlockedAddress is what blockPrivate returns, so the walk can tell a refusal
// apart from an ordinary connection failure with errors.Is.
var errBlockedAddress = errors.New("address is on this machine or the local network")

// cgnat is carrier-grade NAT space, which netip.Addr.IsPrivate does not cover.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// Params is what the UI sends.
type Params struct {
	// URL as pasted. A missing scheme becomes https://.
	URL string `json:"url"`
	// TimeoutMS is the budget for the whole inspection. 0 means 15000.
	TimeoutMS int `json:"timeoutMs"`
	// SkipAge turns off the RDAP domain-age lookup, which is the only part that
	// talks to anyone other than the link itself.
	SkipAge bool `json:"skipAge"`
}

// Hop is one step of the chain. It is recorded before anything is followed, so a
// hop that was refused still appears with the reason it was refused.
type Hop struct {
	// N is the position in the chain, starting at 1.
	N int `json:"n"`
	// URL is the address that was requested at this step.
	URL string `json:"url"`
	// Host is the host part of URL, so the table has a narrow column.
	Host string `json:"host"`
	// Method is "HEAD" or "GET": whichever answer was used.
	Method string `json:"method"`
	// HeadRejected is true when HEAD was tried, refused, and GET was used
	// instead. Some servers answer HEAD with an error and GET with the page.
	HeadRejected bool `json:"headRejected"`
	// Status is what this step answered with. 0 when nothing answered.
	Status int `json:"status"`
	// Location is the Location header exactly as sent, empty when there was none.
	Location string `json:"location"`
	// Next is Location resolved against URL, so a relative Location is shown as
	// the absolute address it means. Empty on the last hop.
	Next string `json:"next"`
	// TookMs is how long this one request took.
	TookMs int64 `json:"tookMs"`
	// Error is a plain sentence, empty when this step answered.
	Error string `json:"error"`
}

// Finding is one thing worth telling the tech about this link.
type Finding struct {
	// ID is a stable key such as "punycode". Not shown to the user; it is what
	// the tests assert on and what the UI keys the list by.
	ID string `json:"id"`
	// Severity is "danger", "warn" or "info".
	Severity string `json:"severity"`
	// Text is one plain sentence written for a junior tech.
	Text string `json:"text"`
}

// Unwrap records a wrapper that was stripped off before any request was made.
type Unwrap struct {
	// Wrapper names the service, e.g. "Microsoft Defender Safe Links".
	Wrapper string `json:"wrapper"`
	// From is the wrapped address.
	From string `json:"from"`
	// To is the real address that was hidden inside it.
	To string `json:"to"`
}

// HostName describes one host, raw and decoded, so a punycode lookalike is
// visible side by side with what it is pretending to be.
type HostName struct {
	Raw string `json:"raw"`
	// Decoded is Raw with every xn-- label turned back into the characters it
	// stands for. Equal to Raw when there is no punycode.
	Decoded string `json:"decoded"`
	// Punycode is true when Raw and Decoded differ.
	Punycode bool `json:"punycode"`
	// Registrable is the approximate eTLD+1, e.g. "example.co.uk". See the
	// accuracy note at the top of domain.go.
	Registrable string `json:"registrable"`
	// IsIP is true when the host is an IP address rather than a name.
	IsIP bool `json:"isIp"`
}

// Age is the best-effort RDAP answer about the destination domain. Unknown is
// the normal case for many country domains and is not a finding.
type Age struct {
	Known bool `json:"known"`
	// Registered is the registration date as YYYY-MM-DD, empty when unknown.
	Registered string `json:"registered"`
	// Days is how old the domain is in days. 0 when unknown.
	Days int `json:"days"`
	// Human is "12 days", "4 years" and so on. Empty when unknown.
	Human string `json:"human"`
	// Note is the sentence the UI shows when Known is false. Empty otherwise.
	Note string `json:"note"`
}

// Report is the whole answer. Every slice is initialised to an empty slice
// before it is returned, so a nil never reaches the UI as JSON null.
type Report struct {
	// Input is the URL exactly as pasted, trimmed.
	Input string `json:"input"`
	// Start is the URL actually requested first, after unwrapping and
	// normalising. Equal to the normalised Input when nothing was unwrapped.
	Start string `json:"start"`
	// Final is where the chain ended.
	Final     string   `json:"final"`
	Unwrapped []Unwrap `json:"unwrapped"`
	Hops      []Hop    `json:"hops"`
	StartHost HostName `json:"startHost"`
	FinalHost HostName `json:"finalHost"`
	// Findings, worst first. Empty when nothing was flagged.
	Findings []Finding `json:"findings"`
	Age      Age       `json:"age"`
	// Level is "ok", "warn", "danger" or "unknown". "unknown" means no step of
	// the chain answered, so there was nothing to judge.
	Level string `json:"level"`
	// Headline is one sentence a junior can read out loud.
	Headline string `json:"headline"`
	// Stopped explains why the walk ended early. Empty when it ran to the end.
	Stopped string `json:"stopped"`
	// CheckedAt is RFC3339 local time, so a screenshot is self-dating.
	CheckedAt string `json:"checkedAt"`
}

// Client walks the chain. The fields exist so the tests can point it at local
// httptest servers; nothing in the UI sets them.
type Client struct {
	HTTP *http.Client
	// RDAPBase is the domain-age endpoint with its trailing slash.
	RDAPBase string
	// MaxHops is maxHops in DefaultClient. The tests lower it.
	MaxHops int
}

// DefaultClient is the client the bound method uses.
func DefaultClient() *Client {
	return &Client{
		HTTP: &http.Client{
			// Jar stays nil: a phishing chain must not be able to set a cookie that
			// identifies this machine on a later hop, and a cookie kept from an
			// earlier inspection must never be sent to a new one.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				// Never follow anything automatically. Every hop is walked by hand
				// so that each one is recorded, and so that a redirect to something
				// that is not a web address is refused rather than fetched.
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				// With a proxy in use the Control hook below sees the proxy's
				// address rather than the destination's, so the private-address
				// block does not apply on a proxied connection.
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
					Control: blockPrivate,
				}).DialContext,
				TLSHandshakeTimeout: 5 * time.Second,
				ForceAttemptHTTP2:   true,
				// Every hop is usually a different host anyway, and not holding a
				// connection open to a phishing site is one less thing to explain.
				DisableKeepAlives: true,
			},
		},
		RDAPBase: rdapBase,
		MaxHops:  maxHops,
	}
}

// blockPrivate refuses to connect to an address on this computer or the local
// network. A link inspector that will fetch any address a pasted link resolves
// to turns CHIT into a way to probe the user's own LAN from an emailed link.
func blockPrivate(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return nil
	}
	if isBlocked(ap.Addr()) {
		return errBlockedAddress
	}
	return nil
}

// isBlocked is the address test on its own, so the findings can apply it to a
// host that was written as a literal address without dialling anything.
func isBlocked(a netip.Addr) bool {
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() ||
		a.IsLinkLocalMulticast() || a.IsMulticast() || a.IsUnspecified() || cgnat.Contains(a)
}

// Inspect follows params.URL to its real destination without loading, running or
// rendering anything, and reports what is worth knowing about where it lands.
func (c *Client) Inspect(ctx context.Context, p Params) (Report, error) {
	target, err := validate(&p)
	if err != nil {
		return Report{}, err
	}
	if ctx.Err() != nil {
		return Report{}, core.Errorf(core.CodeInternal,
			"CHIT was closing down, so the link was not inspected. Try it again.")
	}

	report := Report{
		Input:     strings.TrimSpace(p.URL),
		Unwrapped: []Unwrap{},
		Hops:      []Hop{},
		Findings:  []Finding{},
	}
	// Unwrap's notes are re-derived by findings from the finished report, so
	// they are not collected here: adding them as well would list them twice.
	start, steps, _ := unwrapAll(target)
	report.Unwrapped = steps
	report.Start = start
	report.Final = start

	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutMS)*time.Millisecond)
	defer cancel()

	c.walk(ctx, &report)

	report.StartHost = describeHost(report.Start)
	report.FinalHost = describeHost(report.Final)

	now := time.Now()
	report.Age = c.age(ctx, p, report.FinalHost, now)
	report.Findings = findings(&report)
	report.Level = levelFor(&report)
	report.Headline = headline(&report)
	report.CheckedAt = now.Format(time.RFC3339)
	return report, nil
}

// validate catches everything a user can get wrong before any request is made,
// and returns the address the walk starts from.
func validate(p *Params) (string, error) {
	target, err := NormalizeInput(p.URL)
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

// NormalizeInput turns what was pasted into an address this tool can request. A
// missing scheme becomes https, because that is what a browser tries first.
func NormalizeInput(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", core.Errorf(core.CodeInvalidInput,
			"Paste a link to inspect, for example https://example.com/login.")
	}
	// Prepending before parsing, so a bare host is not read as a path. A
	// javascript: address has no "://" either, so it becomes an impossible host
	// and is refused below, which is the outcome we want.
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	u, err := url.Parse(text)
	if err != nil {
		return "", notAWebAddress(raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", notAWebAddress(raw)
	}
	if u.Host == "" {
		return "", notAWebAddress(raw)
	}
	u.Scheme = scheme
	// The path, the query and the userinfo are case-sensitive, so only the host
	// is lowered.
	u.Host = strings.ToLower(u.Host)
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func notAWebAddress(raw string) error {
	return core.Errorf(core.CodeInvalidInput,
		"\"%s\" is not a web address CHIT can follow. Paste a link that starts with http:// or https://.",
		strings.TrimSpace(raw))
}

// walk follows the chain one hop at a time, recording every step including the
// ones it then refuses to follow.
func (c *Client) walk(ctx context.Context, r *Report) {
	seen := map[string]bool{}
	current := r.Start

	for {
		seen[current] = true
		hop := c.request(ctx, len(r.Hops)+1, current)
		r.Hops = append(r.Hops, hop)
		r.Final = current

		if hop.Error != "" {
			if hop.Error == msgBlockedHop {
				r.Stopped = stoppedPrivate
			} else {
				r.Stopped = stoppedNoAnswer
			}
			return
		}
		if hop.Next == "" {
			return
		}
		if _, bad := nonWebScheme(hop.Next); bad {
			r.Stopped = stoppedNotWeb
			return
		}
		if len(r.Hops) >= c.MaxHops {
			r.Stopped = fmt.Sprintf(stoppedHopCap, c.MaxHops)
			return
		}
		if seen[hop.Next] {
			r.Stopped = stoppedLoop
			return
		}
		current = hop.Next
	}
}

// request asks one address where it points next. HEAD is used first so the
// page's body, its scripts and its tracking pixels are never transferred at all.
func (c *Client) request(ctx context.Context, n int, target string) Hop {
	hop := Hop{N: n, URL: target, Method: http.MethodHead}
	if u, err := url.Parse(target); err == nil {
		hop.Host = u.Hostname()
	}

	started := time.Now()
	got, err := c.fetch(ctx, http.MethodHead, target)
	if err != nil || headRejected(got.status) {
		// Some servers answer HEAD with an error and the page with a GET, so the
		// same address is asked once more before the chain is given up on.
		hop.HeadRejected = true
		hop.Method = http.MethodGet
		got, err = c.fetch(ctx, http.MethodGet, target)
	}
	hop.TookMs = time.Since(started).Milliseconds()

	if err != nil {
		hop.Error = hopError(err)
		return hop
	}
	hop.Status = got.status
	hop.Location = got.location
	if got.status < 300 || got.status > 399 {
		return hop
	}
	if hop.Location == "" {
		hop.Error = msgNoLocation
		return hop
	}
	loc, err := url.Parse(hop.Location)
	if err != nil {
		hop.Error = msgBadLocation
		return hop
	}
	if _, bad := nonWebScheme(hop.Location); bad {
		// Nothing is resolved or requested past a target that is not a web
		// address, so it is recorded exactly as it was sent.
		hop.Next = hop.Location
		return hop
	}
	base, err := url.Parse(target)
	if err != nil {
		hop.Error = msgBadLocation
		return hop
	}
	hop.Next = base.ResolveReference(loc).String()
	return hop
}

// answer is what one request produced. The body is drained and closed inside
// fetch, so the per-hop deadline can be cancelled the moment fetch returns.
type answer struct {
	status   int
	location string
}

func (c *Client) fetch(ctx context.Context, method, target string) (answer, error) {
	ctx, cancel := context.WithTimeout(ctx, perHop)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return answer{}, err
	}
	// A bare product token is blocked by several CDNs, so the chain would end
	// early and read as "safe". No Referer is ever set: it would tell the
	// destination which link was followed, which is what the sender wants to
	// learn. No Cookie is ever set either, and the client has no jar.
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return answer{}, err
	}
	// The body is read only so the connection closes cleanly. Nothing in it is
	// examined, so a malicious page has nothing to act on.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	resp.Body.Close()
	return answer{status: resp.StatusCode, location: resp.Header.Get("Location")}, nil
}

// headRejected lists the answers that mean "ask again with GET". 405 and 501 are
// the honest refusals; 400, 403, 406 and 429 are what CDNs and bot filters send
// a HEAD they do not like; 500 is a server with no HEAD route. Anything else is
// a real answer and is kept.
func headRejected(status int) bool {
	switch status {
	case 400, 403, 405, 406, 429, 500, 501:
		return true
	}
	return false
}

// hopError turns a failed request into one sentence a tech can act on. The Go
// error text is only ever read to choose between them, never shown.
func hopError(err error) string {
	if errors.Is(err, errBlockedAddress) {
		return msgBlockedHop
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return msgNoName
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return msgRefused
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return msgTimedOut
	}
	text := err.Error()
	if strings.Contains(text, "tls:") || strings.Contains(text, "x509:") {
		return msgTLSFailed
	}
	return msgUnreachable
}

// describeHost pulls apart the host of one address: what it says, what it really
// spells, and who registered it.
func describeHost(raw string) HostName {
	u, err := url.Parse(raw)
	if err != nil {
		return HostName{}
	}
	host := u.Hostname()
	decoded := DecodeHost(host)
	out := HostName{Raw: host, Decoded: decoded, Punycode: decoded != host, Registrable: Registrable(host)}
	if _, err := netip.ParseAddr(host); err == nil {
		out.IsIP = true
	}
	return out
}
