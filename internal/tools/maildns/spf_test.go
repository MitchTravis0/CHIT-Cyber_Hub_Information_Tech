package maildns

import (
	"reflect"
	"strings"
	"testing"
)

func TestFindSPF(t *testing.T) {
	tests := []struct {
		name      string
		txts      []string
		wantFound bool
		wantCount int
		wantAll   string
	}{
		{"hard fail", []string{"v=spf1 -all"}, true, 1, "-"},
		{"soft fail", []string{"v=spf1 include:_spf.google.com ~all"}, true, 1, "~"},
		{"neutral", []string{"v=spf1 ?all"}, true, 1, "?"},
		{"pass everyone", []string{"v=spf1 +all"}, true, 1, "+"},
		{"bare all defaults to pass", []string{"v=spf1 all"}, true, 1, "+"},
		{"no all at all", []string{"v=spf1 include:_spf.example.com"}, true, 1, ""},
		{"only the version tag", []string{"v=spf1"}, true, 1, ""},
		{"mixed case tag", []string{"V=SPF1 -ALL"}, true, 1, "-"},
		{"leading whitespace", []string{"   v=spf1 -all"}, true, 1, "-"},
		{"two records", []string{"v=spf1 -all", "v=spf1 ~all"}, true, 2, "-"},
		{"other TXT records ignored", []string{"google-site-verification=abc", "v=spf1 -all"}, true, 1, "-"},
		{"nothing", []string{"google-site-verification=abc"}, false, 0, ""},
		{"empty list", nil, false, 0, ""},
		// A record that merely mentions the tag is not an SPF record. Matching a
		// substring here would make an unrelated TXT record look like SPF.
		{"tag not at the start", []string{"note: v=spf1 -all is what we want"}, false, 0, ""},
		{"tag glued to more text", []string{"v=spf1x -all"}, false, 0, ""},
		{"two all terms, the last wins", []string{"v=spf1 -all ~all"}, true, 1, "~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findSPF(tt.txts)
			if got.Found != tt.wantFound {
				t.Fatalf("found = %v, want %v", got.Found, tt.wantFound)
			}
			if got.Count != tt.wantCount {
				t.Errorf("count = %d, want %d", got.Count, tt.wantCount)
			}
			if got.All != tt.wantAll {
				t.Errorf("all = %q, want %q", got.All, tt.wantAll)
			}
			if got.Terms == nil {
				t.Error("Terms is nil, which marshals to JSON null")
			}
		})
	}
}

func TestParseTerm(t *testing.T) {
	tests := []struct {
		raw       string
		qualifier string
		mechanism string
		value     string
		costs     bool
	}{
		{"-all", "-", "all", "", false},
		{"~all", "~", "all", "", false},
		{"?all", "?", "all", "", false},
		{"+all", "+", "all", "", false},
		{"all", "+", "all", "", false},
		{"include:_spf.google.com", "+", "include", "_spf.google.com", true},
		{"-include:bad.example", "-", "include", "bad.example", true},
		{"a", "+", "a", "", true},
		{"a:mail.example.com", "+", "a", "mail.example.com", true},
		{"a/24", "+", "a", "/24", true},
		{"mx", "+", "mx", "", true},
		{"mx:example.net", "+", "mx", "example.net", true},
		{"ptr", "+", "ptr", "", true},
		{"ptr:example.com", "+", "ptr", "example.com", true},
		{"exists:%{i}.example.com", "+", "exists", "%{i}.example.com", true},
		{"ip4:203.0.113.0/24", "+", "ip4", "203.0.113.0/24", false},
		{"ip6:2001:db8::/32", "+", "ip6", "2001:db8::/32", false},
		{"redirect=example.net", "", "redirect", "example.net", true},
		{"exp=why.example.com", "", "exp", "why.example.com", false},
		{"INCLUDE:UPPER.example", "+", "include", "UPPER.example", true},
		// An unknown mechanism must not be counted as a lookup: CHIT does not
		// know whether it would cost one.
		{"spf2.0/pra", "+", "unknown", "", false},
		{"nonsense=thing", "", "unknown", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := parseTerm(tt.raw)
			if got.Qualifier != tt.qualifier {
				t.Errorf("qualifier = %q, want %q", got.Qualifier, tt.qualifier)
			}
			if got.Mechanism != tt.mechanism {
				t.Errorf("mechanism = %q, want %q", got.Mechanism, tt.mechanism)
			}
			if got.Value != tt.value {
				t.Errorf("value = %q, want %q", got.Value, tt.value)
			}
			if got.CostsLookup != tt.costs {
				t.Errorf("costsLookup = %v, want %v", got.CostsLookup, tt.costs)
			}
			if got.Raw != tt.raw {
				t.Errorf("raw = %q, want %q", got.Raw, tt.raw)
			}
		})
	}
}

func TestSPFLookupCount(t *testing.T) {
	// The literal 10 is written in rather than read from SPFLookupLimit: this is
	// the number in the user-facing sentence, and the two must not drift apart.
	if SPFLookupLimit != 10 {
		t.Fatalf("SPFLookupLimit = %d, want 10 (RFC 7208)", SPFLookupLimit)
	}

	tests := []struct {
		name   string
		record string
		want   int
	}{
		{"nothing costs a lookup", "v=spf1 ip4:203.0.113.0/24 ip6:2001:db8::/32 -all", 0},
		{"one include", "v=spf1 include:_spf.google.com -all", 1},
		{"a and mx both cost", "v=spf1 a mx -all", 2},
		{
			"exactly ten",
			"v=spf1 include:a include:b include:c include:d include:e include:f include:g include:h a mx -all",
			10,
		},
		{
			"eleven, over the limit",
			"v=spf1 include:a include:b include:c include:d include:e include:f include:g include:h include:i a mx -all",
			11,
		},
		{"redirect costs one", "v=spf1 redirect=example.net", 1},
		{"exp costs nothing", "v=spf1 exp=why.example.com -all", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findSPF([]string{tt.record})
			if got.Lookups != tt.want {
				t.Errorf("lookups = %d, want %d", got.Lookups, tt.want)
			}
		})
	}
}

func TestParseSPFTermOrderAndRedirect(t *testing.T) {
	got := findSPF([]string{"v=spf1 a mx ip4:203.0.113.0/24 redirect=example.net"})
	want := []string{"a", "mx", "ip4", "redirect"}
	if len(got.Terms) != len(want) {
		t.Fatalf("got %d terms, want %d", len(got.Terms), len(want))
	}
	for i, term := range got.Terms {
		if term.Mechanism != want[i] {
			t.Errorf("term %d is %q, want %q", i, term.Mechanism, want[i])
		}
	}
	if got.Redirect != "example.net" {
		t.Errorf("redirect = %q, want example.net", got.Redirect)
	}
}

func TestParseSPFHandlesExtraWhitespace(t *testing.T) {
	got := findSPF([]string{"v=spf1   a    mx   -all"})
	if len(got.Terms) != 3 {
		t.Fatalf("got %d terms, want 3", len(got.Terms))
	}
	if got.All != "-" {
		t.Errorf("all = %q, want -", got.All)
	}
}

func TestKnownMechanismsCoverTheRFC(t *testing.T) {
	// Written as a literal list rather than derived from the map, so removing a
	// mechanism from the map fails here instead of silently reclassifying it as
	// unknown.
	want := []string{"all", "include", "a", "mx", "ptr", "ip4", "ip6", "exists"}
	for _, name := range want {
		if !knownMechanisms[name] {
			t.Errorf("%s is not a known mechanism", name)
		}
	}
	if len(knownMechanisms) != len(want) {
		t.Errorf("knownMechanisms has %d entries, want %d", len(knownMechanisms), len(want))
	}

	costing := []string{"include", "a", "mx", "ptr", "exists", "redirect"}
	for _, name := range costing {
		if !costingMechanisms[name] {
			t.Errorf("%s should cost a DNS lookup", name)
		}
	}
	if len(costingMechanisms) != len(costing) {
		t.Errorf("costingMechanisms has %d entries, want %d", len(costingMechanisms), len(costing))
	}
	for _, name := range []string{"ip4", "ip6", "all", "exp"} {
		if costingMechanisms[name] {
			t.Errorf("%s must not cost a DNS lookup", name)
		}
	}
}

func TestSPFTermsAreNeverNil(t *testing.T) {
	// The input that reaches the guard: a domain with no SPF record at all.
	got := findSPF(nil)
	if got.Terms == nil {
		t.Fatal("Terms is nil, which marshals to JSON null and breaks the table")
	}
	if !reflect.DeepEqual(got.Terms, []SPFTerm{}) {
		t.Errorf("Terms = %v, want an empty slice", got.Terms)
	}
}

func TestSPFRecordKeepsItsOriginalText(t *testing.T) {
	const record = "v=spf1 include:_spf.google.com ~all"
	got := findSPF([]string{"  " + record + "  "})
	if !strings.Contains(got.Record, "include:_spf.google.com") {
		t.Errorf("record = %q", got.Record)
	}
	if got.Record != record {
		t.Errorf("record = %q, want it trimmed to %q", got.Record, record)
	}
}
