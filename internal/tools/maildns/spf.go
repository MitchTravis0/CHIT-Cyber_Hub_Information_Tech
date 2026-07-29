package maildns

import (
	"strings"
)

// SPFLookupLimit is the number of DNS lookups RFC 7208 allows an SPF record to
// cost. Past it, receiving servers give up and treat SPF as broken, which means
// a record over the limit is doing nothing at all.
const SPFLookupLimit = 10

// SPFTerm is one mechanism or modifier from the SPF record, in record order.
type SPFTerm struct {
	// Qualifier is "+", "-", "~" or "?" for a mechanism, "" for a modifier.
	Qualifier string `json:"qualifier"`
	// Mechanism is lower case: "all", "include", "a", "mx", "ptr", "ip4",
	// "ip6", "exists", "redirect", "exp", or "unknown".
	Mechanism string `json:"mechanism"`
	// Value is what followed the colon or equals sign, "" when there was none.
	Value string `json:"value"`
	// CostsLookup is true for the terms that count towards the limit of 10.
	CostsLookup bool `json:"costsLookup"`
	// Raw is the term exactly as written.
	Raw string `json:"raw"`
}

type SPF struct {
	Found bool `json:"found"`
	// Record is the SPF TXT string, "" when Found is false.
	Record string `json:"record"`
	// Count is how many v=spf1 records the domain publishes.
	Count int       `json:"count"`
	Terms []SPFTerm `json:"terms"`
	// All is the qualifier on the final "all" mechanism: "-", "~", "?", "+" or
	// "" when there is none.
	All string `json:"all"`
	// Redirect is the redirect= target, "" when there is none.
	Redirect string `json:"redirect"`
	// Lookups is the number of terms that cost a DNS lookup.
	Lookups int `json:"lookups"`
	// Verdict is one sentence about what this record allows.
	Verdict string `json:"verdict"`
}

// costingMechanisms are the terms RFC 7208 counts against the limit of 10.
// ip4, ip6 and all cost nothing because they need no further query.
var costingMechanisms = map[string]bool{
	"include":  true,
	"a":        true,
	"mx":       true,
	"ptr":      true,
	"exists":   true,
	"redirect": true,
}

// knownMechanisms is every mechanism name RFC 7208 defines. Anything else is
// reported as "unknown" rather than guessed at.
var knownMechanisms = map[string]bool{
	"all":     true,
	"include": true,
	"a":       true,
	"mx":      true,
	"ptr":     true,
	"ip4":     true,
	"ip6":     true,
	"exists":  true,
}

// knownModifiers are the two RFC 7208 defines. Both use "=" rather than ":".
var knownModifiers = map[string]bool{"redirect": true, "exp": true}

// findSPF picks the SPF record out of a domain's TXT records. A domain with
// more than one is broken, and the count is reported rather than the first one
// silently winning.
func findSPF(txts []string) SPF {
	out := SPF{Terms: []SPFTerm{}}
	for _, txt := range txts {
		trimmed := strings.TrimSpace(txt)
		// The version tag has to be the whole first term, so a TXT record that
		// merely mentions v=spf1 somewhere is not an SPF record.
		if !hasSPFPrefix(trimmed) {
			continue
		}
		out.Count++
		if !out.Found {
			out.Found = true
			out.Record = trimmed
		}
	}
	if !out.Found {
		return out
	}
	parseSPF(&out)
	return out
}

func hasSPFPrefix(txt string) bool {
	const tag = "v=spf1"
	if len(txt) < len(tag) || !strings.EqualFold(txt[:len(tag)], tag) {
		return false
	}
	return len(txt) == len(tag) || txt[len(tag)] == ' ' || txt[len(tag)] == '\t'
}

func parseSPF(s *SPF) {
	for i, field := range strings.Fields(s.Record) {
		if i == 0 {
			continue // the v=spf1 version tag
		}
		term := parseTerm(field)
		s.Terms = append(s.Terms, term)
		if term.CostsLookup {
			s.Lookups++
		}
		switch term.Mechanism {
		case "all":
			// A record with two "all" terms is malformed; the last one is what
			// this reports, matching how the record reads left to right.
			s.All = term.Qualifier
		case "redirect":
			s.Redirect = term.Value
		}
	}
}

func parseTerm(raw string) SPFTerm {
	term := SPFTerm{Raw: raw, Mechanism: "unknown"}

	// A modifier is name=value and never carries a qualifier.
	if i := strings.Index(raw, "="); i > 0 && !strings.Contains(raw[:i], ":") {
		name := strings.ToLower(raw[:i])
		if knownModifiers[name] {
			term.Mechanism = name
			term.Value = raw[i+1:]
			term.CostsLookup = costingMechanisms[name]
			return term
		}
		return term
	}

	body := raw
	term.Qualifier = "+"
	switch body[0] {
	case '+', '-', '~', '?':
		term.Qualifier = string(body[0])
		body = body[1:]
	}

	name, value := body, ""
	if i := strings.Index(body, ":"); i >= 0 {
		name, value = body[:i], body[i+1:]
	} else if i := strings.Index(body, "/"); i >= 0 {
		// a/24 and mx/24 attach a prefix length directly to the mechanism.
		name, value = body[:i], body[i:]
	}
	name = strings.ToLower(name)
	if !knownMechanisms[name] {
		// An unknown mechanism keeps its qualifier so the raw term still reads
		// correctly, but it is not counted as a lookup: CHIT does not know
		// whether it would cost one.
		return term
	}
	term.Mechanism = name
	term.Value = value
	term.CostsLookup = costingMechanisms[name]
	return term
}
