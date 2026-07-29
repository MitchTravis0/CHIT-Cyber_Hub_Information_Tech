package sitecheck

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
	"time"
)

const (
	certWarnDays   = 30
	certUrgentDays = 7
	slowTotalMs    = 2000
	maxRedirects   = 10
	longChainHops  = 3
	maxBodyBytes   = 262144
)

var errTooManyRedirects = errors.New("too many redirects")

// failure is the transport level problem the check ran into. It is unexported
// because it is Classify's input, not part of the JSON contract.
type failure int

const (
	failureNone failure = iota
	failureDNS
	failureRefused
	// failureUnreachable covers a timeout, no route, and anything else the
	// connection did that this tool cannot name more precisely.
	failureUnreachable
	failureRedirectLoop
	failureTLS
)

// failureOf sorts a request error into one of the buckets Classify has wording
// for. The Go error text itself is never shown to a user.
func failureOf(err error) failure {
	if err == nil {
		return failureNone
	}
	if errors.Is(err, errTooManyRedirects) {
		return failureRedirectLoop
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return failureDNS
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return failureRefused
	}
	if isTLSError(err) {
		return failureTLS
	}
	return failureUnreachable
}

func isTLSError(err error) bool {
	var verification *tls.CertificateVerificationError
	if errors.As(err, &verification) {
		return true
	}
	var record tls.RecordHeaderError
	if errors.As(err, &record) {
		return true
	}
	var alert tls.AlertError
	return errors.As(err, &alert)
}

// Classify turns a finished check into sentences a junior tech can act on and
// sets the level that colours the whole card.
func Classify(r *Result) {
	errs := []string{}
	warns := []string{}

	switch r.failure {
	case failureDNS:
		errs = append(errs, "The name could not be looked up. Check the spelling, and whether this machine's DNS server knows about it.")
	case failureRefused:
		errs = append(errs, "The address answered but refused the connection on that port. The service is probably not running.")
	case failureUnreachable:
		errs = append(errs, "Nothing answered at that address before the time ran out. It may be off, firewalled, or on a network this machine cannot reach.")
	case failureRedirectLoop:
		errs = append(errs, "Too many redirects. The site keeps bouncing between addresses and never settles.")
	}

	certFault := false
	if t := r.TLS; t != nil {
		if t.Expired {
			errs = append(errs, fmt.Sprintf(
				"The certificate expired on %s. Browsers will refuse to load this site until it is renewed.",
				dateOf(t.NotAfter)))
			certFault = true
		}
		if t.NotYetValid {
			errs = append(errs, fmt.Sprintf(
				"The certificate is not valid until %s. Check the clock on the server.",
				dateOf(t.NotBefore)))
			certFault = true
		}
		if !t.HostnameMatch {
			errs = append(errs, fmt.Sprintf(
				"The certificate does not cover %s. It was issued for %s.",
				hostOf(r.URL), t.CommonName))
			certFault = true
		}
		if !t.ChainValid && !certFault {
			errs = append(errs, t.ChainError)
			certFault = true
		}
	}
	if r.failure == failureTLS && !certFault {
		errs = append(errs, "The secure connection could not be set up. The server may only support an old version of TLS.")
	}

	switch {
	case r.Status >= 500:
		errs = append(errs, fmt.Sprintf(
			"The server answered with %d, which means the server itself is broken, not the network.", r.Status))
	case r.Status >= 400:
		errs = append(errs, fmt.Sprintf(
			"The server answered with %d. The address is reachable but that page is not there or is not allowed.", r.Status))
	}

	if t := r.TLS; t != nil {
		switch {
		case t.DaysRemaining >= 0 && t.DaysRemaining <= certUrgentDays:
			warns = append(warns, fmt.Sprintf(
				"The certificate expires in %d days, on %s. Renew it this week.",
				t.DaysRemaining, dateOf(t.NotAfter)))
		case t.DaysRemaining >= 0 && t.DaysRemaining <= certWarnDays:
			warns = append(warns, fmt.Sprintf(
				"The certificate expires in %d days, on %s. Get the renewal booked in.",
				t.DaysRemaining, dateOf(t.NotAfter)))
		}
		if t.Version == "TLS 1.0" || t.Version == "TLS 1.1" {
			warns = append(warns, fmt.Sprintf(
				"The server is still using %s, which modern browsers refuse. It needs updating.", t.Version))
		}
		if strings.Contains(strings.ToUpper(t.SignatureAlgorithm), "SHA1") {
			warns = append(warns, "The certificate is signed with SHA-1, which is obsolete and untrusted.")
		}
	}
	if strings.HasPrefix(r.URL, "http://") {
		warns = append(warns, "This address is plain HTTP, so nothing sent to it is encrypted.")
	}
	if downgraded(r) {
		warns = append(warns, "The redirect chain drops from HTTPS to plain HTTP part way through, which is a mistake worth reporting.")
	}
	if len(r.Redirects) > longChainHops {
		warns = append(warns, fmt.Sprintf(
			"That took %d redirects to settle. Each one costs the user time.", len(r.Redirects)))
	}
	if r.Timing.TotalMs > slowTotalMs {
		warns = append(warns, fmt.Sprintf(
			"The whole check took %s, which a user would call slow.", humanMS(r.Timing.TotalMs)))
	}
	if r.Status >= 300 && r.Status < 400 && !r.followed {
		warns = append(warns, fmt.Sprintf(
			"That is a redirect to %s. Turn on \"Follow redirects\" to see where it ends up.", lastLocation(r)))
	}

	r.Errors = errs
	r.Warnings = warns
	switch {
	case len(errs) > 0:
		r.Level = "error"
		r.Headline = errs[0]
	case len(warns) > 0:
		r.Level = "warn"
		r.Headline = fmt.Sprintf("%s answered %d in %s, with something worth looking at.",
			hostOf(r.URL), r.Status, humanMS(r.Timing.TotalMs))
	default:
		r.Level = "ok"
		r.Headline = fmt.Sprintf("%s answered %d in %s.",
			hostOf(r.URL), r.Status, humanMS(r.Timing.TotalMs))
		if r.TLS != nil {
			r.Headline += fmt.Sprintf(" The certificate is good for %d more days.", r.TLS.DaysRemaining)
		}
	}
}

// downgraded reports a chain that starts encrypted and ends up in the clear.
func downgraded(r *Result) bool {
	steps := make([]string, 0, len(r.Redirects)+1)
	for _, hop := range r.Redirects {
		steps = append(steps, hop.URL)
	}
	if r.FinalURL != "" {
		steps = append(steps, r.FinalURL)
	}
	for i := 1; i < len(steps); i++ {
		if strings.HasPrefix(steps[i-1], "https://") && strings.HasPrefix(steps[i], "http://") {
			return true
		}
	}
	return false
}

func lastLocation(r *Result) string {
	if len(r.Redirects) == 0 {
		return r.FinalURL
	}
	return r.Redirects[len(r.Redirects)-1].Location
}

// humanMS reads like the frontend's formatDuration: milliseconds under a
// second, one decimal second above it.
func humanMS(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.1f s", float64(ms)/1000)
}

// dateOf turns a stored RFC3339 timestamp back into a date a tech would write
// on a ticket.
func dateOf(stamp string) string {
	when, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return stamp
	}
	return when.Format("2 January 2006")
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return raw
	}
	return u.Hostname()
}
