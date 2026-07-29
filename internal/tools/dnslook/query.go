package dnslook

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"chit/internal/core"
)

// ask sends one question to one server and turns the reply into records. A
// question that had no answer still produces one record, because "there is no
// MX record" is an answer a tech needs.
func ask(ctx context.Context, q question, st settings) []Record {
	qctx, cancel := context.WithTimeout(ctx, st.timeout)
	defer cancel()

	resolver := resolverFor(q.server.addr, st.timeout)
	started := time.Now()
	values, priorities, err := runQuery(qctx, resolver, q.typ, st.name)
	elapsed := math.Round(float64(time.Since(started))/float64(time.Millisecond)*100) / 100

	// A cancelled job is not an answer: the user has already abandoned the table
	// these rows would land in.
	if err != nil && (errors.Is(err, context.Canceled) || ctx.Err() != nil) {
		return nil
	}

	status, message := queryOutcome(q.server.label, q.typ, st.name, st.timeoutMS, len(values), err)
	if status != StatusOK {
		return []Record{{
			Server:  q.server.label,
			Type:    q.typ,
			Name:    st.name,
			Status:  status,
			Message: message,
			QueryMS: elapsed,
		}}
	}

	out := make([]Record, 0, len(values))
	for i, value := range values {
		record := Record{
			Server:  q.server.label,
			Type:    q.typ,
			Name:    st.name,
			Value:   value,
			Status:  StatusOK,
			QueryMS: elapsed,
		}
		if i < len(priorities) {
			record.Priority = priorities[i]
		}
		out = append(out, record)
	}
	return out
}

// runQuery dispatches one record type. The priorities slice is nil for the
// types that have none.
func runQuery(ctx context.Context, r *net.Resolver, typ, name string) ([]string, []int, error) {
	switch typ {
	case "A":
		return lookupIP(ctx, r, "ip4", name)
	case "AAAA":
		return lookupIP(ctx, r, "ip6", name)
	case "CNAME":
		cname, err := r.LookupCNAME(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		cname = strings.TrimSuffix(cname, ".")
		// Go hands back the name itself when there is no alias, which is an
		// empty answer rather than a record pointing at nothing.
		if cname == "" || strings.EqualFold(cname, name) {
			return nil, nil, nil
		}
		return []string{cname}, nil, nil
	case "MX":
		mxs, err := r.LookupMX(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		return mxRecords(mxs)
	case "TXT":
		values, err := r.LookupTXT(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		return values, nil, nil
	case "NS":
		nss, err := r.LookupNS(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		values := make([]string, 0, len(nss))
		for _, ns := range nss {
			values = append(values, strings.TrimSuffix(ns.Host, "."))
		}
		return values, nil, nil
	case "SRV":
		// Empty service and proto look the name up directly, which is what a
		// tech typing _ldap._tcp.example.com expects.
		_, srvs, err := r.LookupSRV(ctx, "", "", name)
		if err != nil {
			return nil, nil, err
		}
		values := make([]string, 0, len(srvs))
		priorities := make([]int, 0, len(srvs))
		for _, srv := range srvs {
			values = append(values, fmt.Sprintf("%s:%d weight %d",
				strings.TrimSuffix(srv.Target, "."), srv.Port, srv.Weight))
			priorities = append(priorities, int(srv.Priority))
		}
		return values, priorities, nil
	case "PTR":
		names, err := r.LookupAddr(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		values := make([]string, 0, len(names))
		for _, n := range names {
			values = append(values, strings.TrimSuffix(n, "."))
		}
		return values, nil, nil
	}
	return nil, nil, core.Errorf(core.CodeInvalidInput,
		"CHIT does not look up %s records. Choose from A, AAAA, CNAME, MX, TXT, NS, SRV and PTR.", typ)
}

func lookupIP(ctx context.Context, r *net.Resolver, network, name string) ([]string, []int, error) {
	ips, err := r.LookupIP(ctx, network, name)
	if err != nil {
		return nil, nil, err
	}
	values := make([]string, 0, len(ips))
	for _, ip := range ips {
		values = append(values, ip.String())
	}
	return values, nil, nil
}

// queryOutcome turns whatever the resolver did into a status and a sentence.
// No stdlib DNS wording ever reaches the screen through here.
// errNullMX marks a domain that publishes only the RFC 7505 null MX. It is an
// answer, not a failure, so it travels as an error solely to reach queryOutcome.
var errNullMX = errors.New("null mx")

// mxRecords drops the null MX, which Go reports as a host of "." and which
// would otherwise reach the table as a row marked ok with nothing in it.
func mxRecords(mxs []*net.MX) ([]string, []int, error) {
	values := make([]string, 0, len(mxs))
	priorities := make([]int, 0, len(mxs))
	for _, mx := range mxs {
		host := strings.TrimSuffix(mx.Host, ".")
		if host == "" {
			continue
		}
		values = append(values, host)
		priorities = append(priorities, int(mx.Pref))
	}
	if len(values) == 0 && len(mxs) > 0 {
		return nil, nil, errNullMX
	}
	return values, priorities, nil
}

func queryOutcome(server, typ, name string, timeoutMS, count int, err error) (string, string) {
	var dnsErr *net.DNSError
	switch {
	case err == nil && count > 0:
		return StatusOK, ""
	case errors.Is(err, errNullMX):
		return StatusEmpty, fmt.Sprintf(
			"%s accepts no email at all. It publishes an empty MX record, which is the standard way for a domain to say so, according to %s.",
			name, server)
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
	return fmt.Sprintf("No %s record for %s according to %s.", typ, name, server)
}
