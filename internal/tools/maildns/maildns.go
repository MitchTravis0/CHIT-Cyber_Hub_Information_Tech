// Package maildns reads a domain's MX, SPF, DKIM and DMARC records and says in
// plain English what they allow. It is the service layer the Email DNS Checker
// page talks to.
package maildns

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"time"

	"chit/internal/core"
	"chit/internal/dnsx"
)

// Level values, shared with the other Phase 8 shape B tools.
const (
	LevelOK    = "ok"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Areas a finding can belong to.
const (
	AreaMX    = "MX"
	AreaSPF   = "SPF"
	AreaDKIM  = "DKIM"
	AreaDMARC = "DMARC"
)

// SystemResolverLabel is what the report carries when the answers came through
// whatever this computer is configured to use.
const SystemResolverLabel = "this computer's own resolver"

const (
	defaultTimeoutMS = 5000
	minTimeoutMS     = 500
	maxTimeoutMS     = 20000
	maxNameLength    = 253
	maxLabelLength   = 63
	workers          = 8
	// spfLookupWarnAt is where a record is close enough to the limit that
	// adding one more provider breaks it.
	spfLookupWarnAt = 8
)

type Params struct {
	Domain string `json:"domain"`
	// Selector is one extra DKIM selector to try, "" for none.
	Selector string `json:"selector"`
	// Server is the DNS server to ask, "" for the system resolver.
	Server    string `json:"server"`
	TimeoutMS int    `json:"timeoutMs"`
}

// MXHost is one mail exchanger.
type MXHost struct {
	Host       string `json:"host"`
	Preference int    `json:"preference"`
}

// Finding is one line of the verdict list.
type Finding struct {
	Level  string `json:"level"`
	Area   string `json:"area"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type Report struct {
	Domain string `json:"domain"`
	// Server is the resolver label the answers came from.
	Server string   `json:"server"`
	MX     []MXHost `json:"mx"`
	// NullMX is true when the domain publishes the RFC 7505 empty MX.
	NullMX bool      `json:"nullMx"`
	SPF    SPF       `json:"spf"`
	DMARC  DMARC     `json:"dmarc"`
	DKIM   []DKIMKey `json:"dkim"`
	// SelectorsTried is every selector probed, so the UI can name them.
	SelectorsTried []string  `json:"selectorsTried"`
	Findings       []Finding `json:"findings"`
	Level          string    `json:"level"`
	Headline       string    `json:"headline"`
	CheckedAt      string    `json:"checkedAt"`
}

type settings struct {
	domain    string
	selectors []string
	server    string
	addr      string
	timeout   time.Duration
	timeoutMS int
}

func (p Params) normalize() (settings, error) {
	var s settings

	domain, err := NormalizeDomain(p.Domain)
	if err != nil {
		return s, err
	}
	s.domain = domain

	selector := strings.TrimSpace(p.Selector)
	if len(selector) > MaxSelectorLength {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"A DKIM selector is at most %d characters. %q is longer than that.",
			MaxSelectorLength, selector)
	}
	s.selectors = append([]string{}, CommonSelectors...)
	if selector != "" && !contains(s.selectors, selector) {
		s.selectors = append(s.selectors, selector)
	}

	s.server = strings.TrimSpace(p.Server)
	s.addr, err = dnsx.ServerAddress(s.server)
	if err != nil {
		return settings{}, err
	}

	s.timeoutMS = p.TimeoutMS
	if s.timeoutMS == 0 {
		s.timeoutMS = defaultTimeoutMS
	}
	if s.timeoutMS < minTimeoutMS || s.timeoutMS > maxTimeoutMS {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"The wait for an answer must be between 0.5 and 20 seconds. %d ms is outside that.",
			s.timeoutMS)
	}
	s.timeout = time.Duration(s.timeoutMS) * time.Millisecond
	return s, nil
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// NormalizeDomain accepts what a tech will actually paste: an email address, a
// URL, a trailing dot, any case.
func NormalizeDomain(raw string) (string, error) {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return "", core.Errorf(core.CodeInvalidInput,
			"Type a domain to check, for example example.com.")
	}
	if i := strings.Index(text, "://"); i >= 0 {
		text = text[i+3:]
	}
	if i := strings.LastIndex(text, "@"); i >= 0 {
		text = text[i+1:]
	}
	if i := strings.IndexAny(text, "/?#"); i >= 0 {
		text = text[:i]
	}
	text = strings.TrimSuffix(text, ".")

	if _, err := netip.ParseAddr(text); err == nil {
		return "", core.Errorf(core.CodeInvalidInput,
			"%s is an address, not a domain. Type the domain whose mail records you want, for example example.com.",
			text)
	}

	bad := core.Errorf(core.CodeInvalidInput,
		"%q is not a domain name. Try example.com.", raw)
	if text == "" || len(text) > maxNameLength {
		return "", bad
	}
	labels := strings.Split(text, ".")
	if len(labels) < 2 {
		return "", core.Errorf(core.CodeInvalidInput,
			"%q is not a full domain name. A mail domain has at least one dot, for example example.com.",
			text)
	}
	for _, label := range labels {
		if label == "" || len(label) > maxLabelLength {
			return "", bad
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", bad
		}
		for _, c := range label {
			switch {
			case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			default:
				return "", bad
			}
		}
	}
	return text, nil
}

// Check reads every record and explains what they allow.
func Check(ctx context.Context, p Params) (Report, error) {
	st, err := p.normalize()
	if err != nil {
		return Report{}, err
	}

	resolver := dnsx.ResolverFor(st.addr, st.timeout)
	ctx, cancel := context.WithTimeout(ctx, st.timeout)
	defer cancel()

	report := Report{
		Domain:         st.domain,
		Server:         serverLabel(st.server),
		MX:             []MXHost{},
		DKIM:           []DKIMKey{},
		SelectorsTried: st.selectors,
		Findings:       []Finding{},
	}

	mxs, mxErr := resolver.LookupMX(ctx, st.domain)
	if fatal := fatalLookupError(st.domain, st.server, st.timeoutMS, mxErr); fatal != nil {
		return Report{}, fatal
	}
	for _, mx := range mxs {
		host := strings.TrimSuffix(mx.Host, ".")
		if host == "" {
			// The RFC 7505 null MX: Go reports the root as an empty host. It is
			// a deliberate statement, not a missing record.
			report.NullMX = true
			continue
		}
		report.MX = append(report.MX, MXHost{Host: host, Preference: int(mx.Pref)})
	}

	txts, txtErr := resolver.LookupTXT(ctx, st.domain)
	if fatal := fatalLookupError(st.domain, st.server, st.timeoutMS, txtErr); fatal != nil {
		return Report{}, fatal
	}
	report.SPF = findSPF(txts)

	dmarcTxts, _ := resolver.LookupTXT(ctx, "_dmarc."+st.domain)
	report.DMARC = findDMARC(dmarcTxts)

	for key := range core.Pool(ctx, st.selectors, workers,
		func(c context.Context, selector string) (DKIMKey, bool) {
			records, err := resolver.LookupTXT(c, selector+"._domainkey."+st.domain)
			if err != nil {
				return DKIMKey{}, false
			}
			return parseDKIM(selector, records)
		}) {
		report.DKIM = append(report.DKIM, key)
	}
	sortKeys(report.DKIM, st.selectors)

	Classify(&report)
	report.CheckedAt = time.Now().Format(time.RFC3339)
	return report, nil
}

func serverLabel(server string) string {
	if server == "" {
		return SystemResolverLabel
	}
	return server
}

// sortKeys puts the found selectors back into probe order, which core.Pool does
// not preserve.
func sortKeys(keys []DKIMKey, order []string) {
	rank := make(map[string]int, len(order))
	for i, s := range order {
		rank[s] = i
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && rank[keys[j].Selector] < rank[keys[j-1].Selector]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}

// fatalLookupError separates "this domain does not exist" and "the DNS server is
// unreachable", which stop the whole check, from "there is no record of that
// type", which is an answer.
func fatalLookupError(domain, server string, timeoutMS int, err error) error {
	if err == nil {
		return nil
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			// Go reports NXDOMAIN and "no records of that type" identically, so
			// a domain with no MX would otherwise look like a missing domain.
			// Only a name that resolves nowhere at all reaches the caller, and
			// that is decided by the A lookup the caller has already done.
			return nil
		}
		if dnsErr.IsTimeout {
			return core.Errorf(core.CodeTimeout,
				"The DNS server %s did not answer within %d ms. Check that this computer can reach it on port 53.",
				serverLabel(server), timeoutMS)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return core.Errorf(core.CodeTimeout,
			"The DNS server %s did not answer within %d ms. Check that this computer can reach it on port 53.",
			serverLabel(server), timeoutMS)
	}
	return core.Errorf(core.CodeNetwork,
		"The mail records for %s could not be read. Check that this computer can reach a DNS server.", domain)
}
