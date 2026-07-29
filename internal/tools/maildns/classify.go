package maildns

import (
	"fmt"
	"strings"
)

// Classify turns the parsed records into the findings list, the report level and
// the headline. It takes no network, so every sentence the user can see is
// testable.
func Classify(r *Report) {
	r.Findings = []Finding{}
	mxProblem := false

	// MX
	switch {
	case r.NullMX:
		r.add(LevelOK, AreaMX, "Accepts no email, deliberately", fmt.Sprintf(
			"%s publishes an empty MX record, which is the standard way for a domain to say it accepts no email at all.",
			r.Domain))
	case len(r.MX) == 0:
		mxProblem = true
		r.add(LevelError, AreaMX, "No mail servers", fmt.Sprintf(
			"%s publishes no MX record, so it cannot receive email at all. If that is deliberate, publish a null MX record to say so.",
			r.Domain))
	default:
		r.add(LevelOK, AreaMX, fmt.Sprintf("%d mail %s", len(r.MX), plural(len(r.MX), "server", "servers")),
			fmt.Sprintf("Mail for %s is delivered to %s.", r.Domain, lowestMX(r.MX)))
	}

	// SPF
	switch {
	case !r.SPF.Found:
		r.add(LevelError, AreaSPF, "No SPF record", fmt.Sprintf(
			"Nothing tells a receiving server which machines may send email as %s, so anyone can claim to be this domain and nothing in DNS contradicts them.",
			r.Domain))
	case r.SPF.Count > 1:
		r.add(LevelError, AreaSPF, fmt.Sprintf("%d SPF records", r.SPF.Count),
			"A domain may publish only one SPF record. With more than one, receiving servers treat SPF as broken and ignore all of them. Merge them into a single record.")
	default:
		r.addSPFEnding()
	}
	if r.SPF.Found {
		r.addSPFLookups()
	}

	// DMARC
	switch {
	case !r.DMARC.Found:
		r.add(LevelError, AreaDMARC, "No DMARC record",
			"Nothing tells a receiving server what to do when a message fails SPF or DKIM, so failures are quietly ignored. This is the record that actually stops spoofing.")
	case r.DMARC.Count > 1:
		r.add(LevelError, AreaDMARC, fmt.Sprintf("%d DMARC records", r.DMARC.Count),
			"A domain may publish only one DMARC record. With more than one, receiving servers ignore DMARC entirely.")
	default:
		r.addDMARCPolicy()
	}

	// DKIM
	r.addDKIM()

	r.Level = worstLevel(r.Findings)
	r.Headline = headline(r.Domain, r.Level, mxProblem)
	r.SPF.Verdict = verdictFor(r.Findings, AreaSPF)
	r.DMARC.Verdict = verdictFor(r.Findings, AreaDMARC)
}

func (r *Report) add(level, area, title, detail string) {
	r.Findings = append(r.Findings, Finding{Level: level, Area: area, Title: title, Detail: detail})
}

func (r *Report) addSPFEnding() {
	switch r.SPF.All {
	case "-":
		r.add(LevelOK, AreaSPF, "SPF hard fail",
			"Anything sent from a machine not listed here should be rejected outright. That is the strongest setting.")
	case "~":
		r.add(LevelWarn, AreaSPF, "SPF soft fail",
			"Anything sent from a machine not listed here is marked as suspect but most servers will still deliver it. Tighten this to -all once you are sure the list is complete.")
	case "?":
		r.add(LevelWarn, AreaSPF, "SPF neutral",
			"The record lists senders but then says to do nothing about anyone else, which is barely better than having no SPF at all.")
	case "+":
		r.add(LevelError, AreaSPF, "SPF allows everyone", fmt.Sprintf(
			"This record says every machine on the internet may send email as %s. That is almost always a mistake.", r.Domain))
	default:
		if r.SPF.Redirect != "" {
			r.add(LevelOK, AreaSPF, "SPF redirects", fmt.Sprintf(
				"This record hands the decision to %s. Check that domain's SPF record to see what is actually enforced.",
				r.SPF.Redirect))
			return
		}
		r.add(LevelWarn, AreaSPF, "SPF has no ending",
			"The record never says what to do about a sender it has not listed, so receiving servers fall back to neutral, which is the same as doing nothing.")
	}
}

func (r *Report) addSPFLookups() {
	switch {
	case r.SPF.Lookups > SPFLookupLimit:
		r.add(LevelError, AreaSPF,
			fmt.Sprintf("%d SPF lookups, limit is %d", r.SPF.Lookups, SPFLookupLimit),
			fmt.Sprintf("Receiving servers give up after %d lookups and treat SPF as broken, which means SPF is doing nothing for this domain today. Flatten some of the include: entries.",
				SPFLookupLimit))
	case r.SPF.Lookups >= spfLookupWarnAt:
		r.add(LevelWarn, AreaSPF,
			fmt.Sprintf("%d of %d SPF lookups used", r.SPF.Lookups, SPFLookupLimit),
			fmt.Sprintf("SPF is allowed %d DNS lookups. This record uses %d, so adding one more mail provider will break it.",
				SPFLookupLimit, r.SPF.Lookups))
	}
}

func (r *Report) addDMARCPolicy() {
	switch r.DMARC.Policy {
	case "none":
		r.add(LevelWarn, AreaDMARC, "DMARC is monitoring only",
			"p=none means nothing is blocked or junked, whatever fails. It is the right first step, but it stops nobody. Move to quarantine once the reports look clean.")
	case "quarantine":
		r.add(LevelWarn, AreaDMARC, "DMARC sends failures to junk",
			"A message that fails should land in the junk folder rather than the inbox. Reject is the stronger setting.")
	case "reject":
		r.add(LevelOK, AreaDMARC, "DMARC rejects failures",
			"A message that fails SPF and DKIM should be refused outright. That is the strongest setting.")
	default:
		r.add(LevelError, AreaDMARC, "DMARC record has no policy",
			"The record is there but it has no p= tag, which makes it invalid. Receiving servers will ignore it.")
		return
	}
	if r.DMARC.Pct < 100 {
		r.add(LevelWarn, AreaDMARC, fmt.Sprintf("DMARC applies to %d%% of mail", r.DMARC.Pct),
			fmt.Sprintf("The policy is only applied to %d out of every 100 messages. The rest are treated as if the policy were none.",
				r.DMARC.Pct))
	}
}

// maxSelectorsListed is how many selectors are named one by one before the rest
// are counted instead. A domain that answers on every selector CHIT tries
// (example.com does, with a wildcard) would otherwise bury the MX, SPF and
// DMARC findings under fourteen identical DKIM lines.
const maxSelectorsListed = 3

func (r *Report) addDKIM() {
	if len(r.DKIM) == 0 {
		r.add(LevelWarn, AreaDKIM, "No DKIM key at the selectors checked", fmt.Sprintf(
			"CHIT tried %d common selector names and none answered. A domain can use any name, so this is not proof there is no DKIM. Look at the s= value in a DKIM-Signature header from this domain and put it in the extra selector box.",
			len(r.SelectorsTried)))
		return
	}

	var published, revoked []DKIMKey
	for _, key := range r.DKIM {
		if key.HasKey {
			published = append(published, key)
			continue
		}
		revoked = append(revoked, key)
	}

	// A domain that answers on every single selector, none of them with a key,
	// is publishing a wildcard record that says "no DKIM here". That is one
	// fact, not one revoked key per name CHIT happened to try.
	if len(revoked) == len(r.SelectorsTried) && len(published) == 0 {
		r.add(LevelWarn, AreaDKIM, "Every selector answered with an empty key", fmt.Sprintf(
			"All %d selectors CHIT tried returned a record with no key in it, which means %s publishes a wildcard saying it has no DKIM key at all rather than %d separate revoked keys.",
			len(revoked), r.Domain, len(revoked)))
		return
	}

	r.addKeyList(published, LevelOK, "DKIM key found", "DKIM keys found",
		"A signing key is published at %s, so this domain can sign its outgoing mail.",
		"%d selectors have a signing key published, including %s, so this domain can sign its outgoing mail.")
	r.addKeyList(revoked, LevelWarn, "DKIM key revoked", "selectors answered with an empty key",
		"The record at %s exists but its key is empty, which is how a key is revoked. Messages signed with it will fail.",
		"%d selectors answered with a record whose key is empty, including %s, which is how a key is revoked. Messages signed with them will fail.")
}

// addKeyList names a few selectors individually and counts the rest, so a
// domain with many answers still produces a report a tech will read.
func (r *Report) addKeyList(keys []DKIMKey, level, oneTitle, manyTitle, oneDetail, manyDetail string) {
	switch {
	case len(keys) == 0:
		return
	case len(keys) <= maxSelectorsListed:
		for _, key := range keys {
			r.add(level, AreaDKIM, oneTitle, fmt.Sprintf(oneDetail, key.Selector+"._domainkey."+r.Domain))
		}
	default:
		names := make([]string, 0, maxSelectorsListed)
		for _, key := range keys[:maxSelectorsListed] {
			names = append(names, key.Selector)
		}
		r.add(level, AreaDKIM, fmt.Sprintf("%d %s", len(keys), manyTitle),
			fmt.Sprintf(manyDetail, len(keys), strings.Join(names, ", ")))
	}
}

func worstLevel(findings []Finding) string {
	level := LevelOK
	for _, f := range findings {
		if f.Level == LevelError {
			return LevelError
		}
		if f.Level == LevelWarn {
			level = LevelWarn
		}
	}
	return level
}

func headline(domain, level string, mxProblem bool) string {
	switch {
	case level == LevelOK:
		return fmt.Sprintf("%s's mail records are in good shape.", domain)
	case level == LevelWarn:
		return fmt.Sprintf("%s can receive email, but its records leave gaps a spoofer can use.", domain)
	case mxProblem:
		return fmt.Sprintf("%s cannot receive email: it has no mail servers.", domain)
	default:
		return fmt.Sprintf("%s can receive email, but nothing effectively stops someone sending as it.", domain)
	}
}

// verdictFor is the one sentence the SPF and DMARC panels show above their
// detail, taken from the first finding in that area so the panel and the
// findings table can never say different things.
func verdictFor(findings []Finding, area string) string {
	for _, f := range findings {
		if f.Area == area {
			return f.Detail
		}
	}
	return ""
}

// lowestMX names the mail server that gets the mail, which is the one with the
// lowest preference.
func lowestMX(hosts []MXHost) string {
	best := hosts[0]
	for _, h := range hosts[1:] {
		if h.Preference < best.Preference {
			best = h
		}
	}
	return best.Host
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
