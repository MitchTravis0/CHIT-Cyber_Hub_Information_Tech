package dnscmp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"chit/internal/dnsx"
)

// errNullMX marks a domain that publishes only the RFC 7505 null MX. It is an
// answer ("this domain accepts no email"), not a failure, so it travels as an
// error solely to reach outcome.
var errNullMX = errors.New("null mx")

// ask sends one question to one resolver and turns the reply into an Answer.
// A question that had no answer still produces a row: "this server says there
// is no record" is exactly the kind of disagreement this tool exists to find.
func ask(ctx context.Context, t target, st settings) Answer {
	qctx, cancel := context.WithTimeout(ctx, st.timeout)
	defer cancel()

	resolver := dnsx.ResolverFor(t.addr, st.timeout)
	started := time.Now()
	values, err := lookup(qctx, resolver, st.typ, st.name)
	elapsed := math.Round(float64(time.Since(started))/float64(time.Millisecond)*100) / 100

	out := Answer{
		Server:  t.id,
		Label:   t.label,
		Values:  []string{},
		QueryMS: elapsed,
	}
	out.Status, out.Message = outcome(t.label, st.typ, st.name, st.timeoutMS, len(values), err)
	if out.Status == StatusOK {
		out.Values = normalizeValues(values)
	}
	return out
}

// lookup dispatches one record type. MX values keep their preference so that a
// changed preference counts as a disagreement.
func lookup(ctx context.Context, r *net.Resolver, typ, name string) ([]string, error) {
	switch typ {
	case "A":
		return lookupIP(ctx, r, "ip4", name)
	case "AAAA":
		return lookupIP(ctx, r, "ip6", name)
	case "CNAME":
		cname, err := r.LookupCNAME(ctx, name)
		if err != nil {
			return nil, err
		}
		cname = strings.TrimSuffix(cname, ".")
		// Go hands back the name itself when there is no alias, which is an
		// empty answer rather than a record pointing at nothing.
		if cname == "" || strings.EqualFold(cname, name) {
			return nil, nil
		}
		return []string{cname}, nil
	case "MX":
		mxs, err := r.LookupMX(ctx, name)
		if err != nil {
			return nil, err
		}
		return mxValues(mxs)
	case "TXT":
		return r.LookupTXT(ctx, name)
	case "NS":
		nss, err := r.LookupNS(ctx, name)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(nss))
		for _, ns := range nss {
			values = append(values, strings.TrimSuffix(ns.Host, "."))
		}
		return values, nil
	}
	// normalize has already rejected anything else, so this is unreachable
	// through the bound method and exists only so the switch is total.
	return nil, fmt.Errorf("unsupported type %s", typ)
}

func lookupIP(ctx context.Context, r *net.Resolver, network, name string) ([]string, error) {
	ips, err := r.LookupIP(ctx, network, name)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(ips))
	for _, ip := range ips {
		values = append(values, ip.String())
	}
	return values, nil
}

// mxValues drops the null MX, which Go reports as a host of "." and which would
// otherwise reach the table as a row marked ok with nothing in it.
func mxValues(mxs []*net.MX) ([]string, error) {
	values := make([]string, 0, len(mxs))
	for _, mx := range mxs {
		host := strings.TrimSuffix(mx.Host, ".")
		if host == "" {
			continue
		}
		values = append(values, fmt.Sprintf("%d %s", mx.Pref, host))
	}
	if len(values) == 0 && len(mxs) > 0 {
		return nil, errNullMX
	}
	return values, nil
}

// outcome turns whatever the resolver did into a status and a sentence. No
// stdlib DNS wording ever reaches the screen through here.
func outcome(server, typ, name string, timeoutMS, count int, err error) (string, string) {
	var dnsErr *net.DNSError
	switch {
	case err == nil && count > 0:
		return StatusOK, ""
	case errors.Is(err, errNullMX):
		return StatusEmpty, fmt.Sprintf(
			"%s says %s accepts no email at all: it publishes an empty MX record, which is the standard way for a domain to say so.",
			server, name)
	case err == nil:
		return StatusEmpty, noRecord(server, typ, name)
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		// Go reports NXDOMAIN and "no records of that type" the same way, so
		// they cannot be told apart and must not be described differently.
		return StatusEmpty, noRecord(server, typ, name)
	case errors.As(err, &dnsErr) && dnsErr.IsTimeout, errors.Is(err, context.DeadlineExceeded):
		return StatusError, fmt.Sprintf("%s did not answer within %d ms.", server, timeoutMS)
	default:
		return StatusError, fmt.Sprintf("%s could not answer that question.", server)
	}
}

func noRecord(server, typ, name string) string {
	return fmt.Sprintf("%s says there is no %s record for %s.", server, typ, name)
}
