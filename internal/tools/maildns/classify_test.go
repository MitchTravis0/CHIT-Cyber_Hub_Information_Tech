package maildns

import (
	"strings"
	"testing"
)

// healthy is a domain with nothing wrong: mail servers, a hard-fail SPF, a
// reject DMARC and a published DKIM key.
func healthy() Report {
	return Report{
		Domain:         "example.com",
		MX:             []MXHost{{Host: "mail.example.com", Preference: 10}},
		SPF:            findSPF([]string{"v=spf1 include:_spf.example.com -all"}),
		DMARC:          findDMARC([]string{"v=DMARC1; p=reject"}),
		DKIM:           []DKIMKey{{Selector: "selector1", HasKey: true, KeyType: "rsa"}},
		SelectorsTried: CommonSelectors,
	}
}

func findingsFor(t *testing.T, r Report, area string) []Finding {
	t.Helper()
	Classify(&r)
	var out []Finding
	for _, f := range r.Findings {
		if f.Area == area {
			out = append(out, f)
		}
	}
	return out
}

func hasFinding(r Report, level, area, titleFragment, detailFragment string) bool {
	for _, f := range r.Findings {
		if f.Level == level && f.Area == area &&
			strings.Contains(f.Title, titleFragment) &&
			strings.Contains(f.Detail, detailFragment) {
			return true
		}
	}
	return false
}

func TestClassifyMX(t *testing.T) {
	tests := []struct {
		name          string
		mx            []MXHost
		nullMX        bool
		wantLevel     string
		wantTitle     string
		wantDetail    string
		wantMXProblem bool
	}{
		{
			"mail servers present",
			[]MXHost{{Host: "mail.example.com", Preference: 10}}, false,
			LevelOK, "1 mail server", "delivered to mail.example.com", false,
		},
		{
			"several mail servers name the lowest preference",
			[]MXHost{{Host: "backup.example.com", Preference: 20}, {Host: "mail.example.com", Preference: 10}}, false,
			LevelOK, "2 mail servers", "delivered to mail.example.com", false,
		},
		{
			"no mail servers",
			nil, false,
			LevelError, "No mail servers", "cannot receive email at all", true,
		},
		{
			"null MX",
			nil, true,
			LevelOK, "Accepts no email, deliberately", "standard way for a domain to say", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := healthy()
			r.MX = tt.mx
			r.NullMX = tt.nullMX
			Classify(&r)
			if !hasFinding(r, tt.wantLevel, AreaMX, tt.wantTitle, tt.wantDetail) {
				t.Fatalf("no %s MX finding titled %q with %q. Got: %+v",
					tt.wantLevel, tt.wantTitle, tt.wantDetail, findingsFor(t, r, AreaMX))
			}
			if tt.wantMXProblem && !strings.Contains(r.Headline, "cannot receive email") {
				t.Errorf("headline = %q, want it to say the domain cannot receive email", r.Headline)
			}
		})
	}
}

func TestClassifySPF(t *testing.T) {
	tests := []struct {
		name       string
		txts       []string
		wantLevel  string
		wantTitle  string
		wantDetail string
	}{
		{"no record", nil, LevelError, "No SPF record", "anyone can claim to be this domain"},
		{"hard fail", []string{"v=spf1 -all"}, LevelOK, "SPF hard fail", "rejected outright"},
		{"soft fail", []string{"v=spf1 ~all"}, LevelWarn, "SPF soft fail", "most servers will still deliver it"},
		{"neutral", []string{"v=spf1 ?all"}, LevelWarn, "SPF neutral", "barely better than having no SPF"},
		{"pass everyone", []string{"v=spf1 +all"}, LevelError, "SPF allows everyone", "almost always a mistake"},
		{"no ending", []string{"v=spf1 include:a.example"}, LevelWarn, "SPF has no ending", "fall back to neutral"},
		{"redirect", []string{"v=spf1 redirect=example.net"}, LevelOK, "SPF redirects", "hands the decision to example.net"},
		{"two records", []string{"v=spf1 -all", "v=spf1 ~all"}, LevelError, "2 SPF records", "only one SPF record"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := healthy()
			r.SPF = findSPF(tt.txts)
			Classify(&r)
			if !hasFinding(r, tt.wantLevel, AreaSPF, tt.wantTitle, tt.wantDetail) {
				t.Fatalf("no %s SPF finding titled %q with %q. Got: %+v",
					tt.wantLevel, tt.wantTitle, tt.wantDetail, findingsFor(t, r, AreaSPF))
			}
		})
	}
}

func TestClassifySPFLookups(t *testing.T) {
	// The literals 8, 10 and 11 are written in, and the sentences carry them.
	seven := "v=spf1 include:a include:b include:c include:d include:e include:f include:g -all"
	eight := seven[:len(seven)-5] + " include:h -all"
	eleven := "v=spf1 include:a include:b include:c include:d include:e include:f include:g include:h include:i a mx -all"

	tests := []struct {
		name      string
		record    string
		wantLevel string
		wantTitle string
		wantAny   bool
	}{
		{"seven is quiet", seven, "", "", false},
		{"eight warns", eight, LevelWarn, "8 of 10 SPF lookups used", true},
		{"eleven is an error", eleven, LevelError, "11 SPF lookups, limit is 10", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := healthy()
			r.SPF = findSPF([]string{tt.record})
			Classify(&r)
			found := false
			for _, f := range r.Findings {
				if f.Area == AreaSPF && strings.Contains(f.Title, "lookup") {
					found = true
					if f.Level != tt.wantLevel || !strings.Contains(f.Title, tt.wantTitle) {
						t.Errorf("lookup finding = %s %q, want %s %q", f.Level, f.Title, tt.wantLevel, tt.wantTitle)
					}
				}
			}
			if found != tt.wantAny {
				t.Errorf("lookup finding present = %v, want %v (lookups = %d)", found, tt.wantAny, r.SPF.Lookups)
			}
		})
	}
}

func TestClassifyDMARC(t *testing.T) {
	tests := []struct {
		name       string
		txts       []string
		wantLevel  string
		wantTitle  string
		wantDetail string
	}{
		{"no record", nil, LevelError, "No DMARC record", "quietly ignored"},
		{"monitoring only", []string{"v=DMARC1; p=none"}, LevelWarn, "DMARC is monitoring only", "stops nobody"},
		{"quarantine", []string{"v=DMARC1; p=quarantine"}, LevelWarn, "DMARC sends failures to junk", "junk folder"},
		{"reject", []string{"v=DMARC1; p=reject"}, LevelOK, "DMARC rejects failures", "refused outright"},
		{"no policy tag", []string{"v=DMARC1; rua=mailto:a@b.com"}, LevelError, "DMARC record has no policy", "no p= tag"},
		{"two records", []string{"v=DMARC1; p=none", "v=DMARC1; p=reject"}, LevelError, "2 DMARC records", "ignore DMARC entirely"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := healthy()
			r.DMARC = findDMARC(tt.txts)
			Classify(&r)
			if !hasFinding(r, tt.wantLevel, AreaDMARC, tt.wantTitle, tt.wantDetail) {
				t.Fatalf("no %s DMARC finding titled %q with %q. Got: %+v",
					tt.wantLevel, tt.wantTitle, tt.wantDetail, findingsFor(t, r, AreaDMARC))
			}
		})
	}
}

func TestClassifyDMARCPct(t *testing.T) {
	r := healthy()
	r.DMARC = findDMARC([]string{"v=DMARC1; p=reject; pct=40"})
	Classify(&r)
	if !hasFinding(r, LevelWarn, AreaDMARC, "DMARC applies to 40% of mail", "40 out of every 100 messages") {
		t.Fatalf("no pct finding. Got: %+v", findingsFor(t, r, AreaDMARC))
	}

	full := healthy()
	Classify(&full)
	for _, f := range full.Findings {
		if strings.Contains(f.Title, "applies to") {
			t.Errorf("pct 100 produced a finding it should not: %q", f.Title)
		}
	}
}

func TestClassifyDKIM(t *testing.T) {
	t.Run("key found", func(t *testing.T) {
		r := healthy()
		Classify(&r)
		if !hasFinding(r, LevelOK, AreaDKIM, "DKIM key found", "selector1._domainkey.example.com") {
			t.Fatalf("no DKIM finding. Got: %+v", findingsFor(t, r, AreaDKIM))
		}
	})

	t.Run("key revoked", func(t *testing.T) {
		r := healthy()
		r.DKIM = []DKIMKey{{Selector: "selector1", HasKey: false, KeyType: "rsa"}}
		Classify(&r)
		if !hasFinding(r, LevelWarn, AreaDKIM, "DKIM key revoked", "its key is empty") {
			t.Fatalf("no revoked finding. Got: %+v", findingsFor(t, r, AreaDKIM))
		}
	})

	// example.com answers every selector, including made-up ones, with
	// "v=DKIM1; p=". That is one wildcard record saying "no DKIM here", and
	// reporting it as fourteen separate revoked keys is the Phase 5
	// "flag fires on 381 of 393 entries" failure in a new form.
	t.Run("a wildcard is one finding, not one per selector", func(t *testing.T) {
		r := healthy()
		r.DKIM = nil
		for _, s := range CommonSelectors {
			r.DKIM = append(r.DKIM, DKIMKey{Selector: s, HasKey: false, KeyType: "rsa"})
		}
		Classify(&r)

		dkim := findingsFor(t, r, AreaDKIM)
		if len(dkim) != 1 {
			t.Fatalf("got %d DKIM findings, want 1. Got: %+v", len(dkim), dkim)
		}
		if !hasFinding(r, LevelWarn, AreaDKIM, "Every selector answered with an empty key", "wildcard") {
			t.Errorf("finding = %+v", dkim[0])
		}
		if !strings.Contains(dkim[0].Detail, "no DKIM key at all") {
			t.Errorf("the sentence does not say what a wildcard means: %q", dkim[0].Detail)
		}
	})

	t.Run("several revoked keys collapse into one finding", func(t *testing.T) {
		r := healthy()
		r.DKIM = []DKIMKey{
			{Selector: "selector1", HasKey: true},
			{Selector: "s1", HasKey: false},
			{Selector: "s2", HasKey: false},
			{Selector: "k1", HasKey: false},
			{Selector: "k2", HasKey: false},
		}
		Classify(&r)

		dkim := findingsFor(t, r, AreaDKIM)
		// One "key found" plus one collapsed "revoked" line, not five lines.
		if len(dkim) != 2 {
			t.Fatalf("got %d DKIM findings, want 2. Got: %+v", len(dkim), dkim)
		}
		if !hasFinding(r, LevelWarn, AreaDKIM, "4 selectors answered with an empty key", "revoked") {
			t.Errorf("no collapsed revoked finding. Got: %+v", dkim)
		}
	})

	t.Run("a couple of revoked keys are still named individually", func(t *testing.T) {
		r := healthy()
		r.DKIM = []DKIMKey{{Selector: "s1", HasKey: false}, {Selector: "s2", HasKey: false}}
		Classify(&r)

		dkim := findingsFor(t, r, AreaDKIM)
		if len(dkim) != 2 {
			t.Fatalf("got %d DKIM findings, want 2 named selectors. Got: %+v", len(dkim), dkim)
		}
		if !hasFinding(r, LevelWarn, AreaDKIM, "DKIM key revoked", "s1._domainkey.example.com") {
			t.Errorf("s1 is not named. Got: %+v", dkim)
		}
	})

	t.Run("nothing found names how many were tried", func(t *testing.T) {
		r := healthy()
		r.DKIM = []DKIMKey{}
		Classify(&r)
		if !hasFinding(r, LevelWarn, AreaDKIM, "No DKIM key at the selectors checked", "14 common selector names") {
			t.Fatalf("no not-found finding. Got: %+v", findingsFor(t, r, AreaDKIM))
		}
		// The wording must never claim there is no DKIM.
		for _, f := range r.Findings {
			if f.Area == AreaDKIM && !strings.Contains(f.Detail, "not proof there is no DKIM") {
				t.Errorf("the DKIM sentence does not say it is not proof: %q", f.Detail)
			}
		}
	})
}

// TestCommonSelectorsCount pins the length as a literal, because the number
// appears in the sentence a user reads. Shortening the list without changing the
// sentence would make the tool lie about what it checked.
func TestCommonSelectorsCount(t *testing.T) {
	if len(CommonSelectors) != 14 {
		t.Fatalf("CommonSelectors has %d entries, want 14 (the sentence says how many were tried)",
			len(CommonSelectors))
	}
	seen := make(map[string]bool, len(CommonSelectors))
	for _, s := range CommonSelectors {
		if seen[s] {
			t.Errorf("selector %q appears twice", s)
		}
		seen[s] = true
		if len(s) > MaxSelectorLength {
			t.Errorf("selector %q is longer than a DNS label", s)
		}
	}
}

// TestClassifyRate is the guard against a heuristic that fires on everything: a
// correctly configured domain must produce zero findings above ok.
func TestClassifyRate(t *testing.T) {
	r := healthy()
	Classify(&r)
	for _, f := range r.Findings {
		if f.Level != LevelOK {
			t.Errorf("a well configured domain raised a %s finding: %s / %s", f.Level, f.Title, f.Detail)
		}
	}
	if r.Level != LevelOK {
		t.Errorf("level = %q, want %q", r.Level, LevelOK)
	}
	if r.Headline != "example.com's mail records are in good shape." {
		t.Errorf("headline = %q", r.Headline)
	}
}

func TestHeadlines(t *testing.T) {
	tests := []struct {
		name string
		tune func(*Report)
		want string
	}{
		{"all good", func(*Report) {}, "example.com's mail records are in good shape."},
		{
			"a gap", func(r *Report) { r.SPF = findSPF([]string{"v=spf1 ~all"}) },
			"example.com can receive email, but its records leave gaps a spoofer can use.",
		},
		{
			"no mail servers", func(r *Report) { r.MX = nil },
			"example.com cannot receive email: it has no mail servers.",
		},
		{
			"no SPF at all", func(r *Report) { r.SPF = findSPF(nil) },
			"example.com can receive email, but nothing effectively stops someone sending as it.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := healthy()
			tt.tune(&r)
			Classify(&r)
			if r.Headline != tt.want {
				t.Errorf("headline = %q, want %q", r.Headline, tt.want)
			}
		})
	}
}

func TestEveryFindingIsComplete(t *testing.T) {
	variants := []func(*Report){
		func(*Report) {},
		func(r *Report) { r.MX = nil },
		func(r *Report) { r.NullMX = true; r.MX = nil },
		func(r *Report) { r.SPF = findSPF(nil) },
		func(r *Report) { r.SPF = findSPF([]string{"v=spf1 ~all"}) },
		func(r *Report) { r.SPF = findSPF([]string{"v=spf1 +all"}) },
		func(r *Report) { r.SPF = findSPF([]string{"v=spf1"}) },
		func(r *Report) { r.DMARC = findDMARC(nil) },
		func(r *Report) { r.DMARC = findDMARC([]string{"v=DMARC1; p=none; pct=10"}) },
		func(r *Report) { r.DKIM = []DKIMKey{} },
		func(r *Report) { r.DKIM = []DKIMKey{{Selector: "s1"}} },
	}
	for i, tune := range variants {
		r := healthy()
		tune(&r)
		Classify(&r)
		if len(r.Findings) == 0 {
			t.Fatalf("variant %d produced no findings at all", i)
		}
		for _, f := range r.Findings {
			if strings.TrimSpace(f.Title) == "" || strings.TrimSpace(f.Detail) == "" ||
				f.Area == "" || f.Level == "" {
				t.Errorf("variant %d has an incomplete finding: %+v", i, f)
			}
		}
		if strings.TrimSpace(r.Headline) == "" {
			t.Errorf("variant %d has a blank headline", i)
		}
	}
}

// TestFindingsStayReadable is the rate check: no shape of input may bury the
// answer under a list nobody will read. Fourteen DKIM lines for one domain is
// not a report, it is noise.
func TestFindingsStayReadable(t *testing.T) {
	allRevoked := healthy()
	allRevoked.DKIM = nil
	for _, s := range CommonSelectors {
		allRevoked.DKIM = append(allRevoked.DKIM, DKIMKey{Selector: s, KeyType: "rsa"})
	}
	allFound := healthy()
	allFound.DKIM = nil
	for _, s := range CommonSelectors {
		allFound.DKIM = append(allFound.DKIM, DKIMKey{Selector: s, HasKey: true, KeyType: "rsa"})
	}

	for name, r := range map[string]Report{
		"every selector revoked": allRevoked,
		"every selector signing": allFound,
		"nothing configured":     {Domain: "example.com", SelectorsTried: CommonSelectors},
	} {
		r := r
		Classify(&r)
		// The literal 8 is the readable-report bound. A domain that trips more
		// than this is being described badly, not diagnosed.
		if len(r.Findings) > 8 {
			t.Errorf("%s produced %d findings, want at most 8:\n%+v", name, len(r.Findings), r.Findings)
		}
	}
}

func TestWorstLevelWins(t *testing.T) {
	tests := []struct {
		name string
		in   []Finding
		want string
	}{
		{"all ok", []Finding{{Level: LevelOK}, {Level: LevelOK}}, LevelOK},
		{"one warn", []Finding{{Level: LevelOK}, {Level: LevelWarn}}, LevelWarn},
		{"one error among warns", []Finding{{Level: LevelWarn}, {Level: LevelError}}, LevelError},
		{"error first", []Finding{{Level: LevelError}, {Level: LevelOK}}, LevelError},
		{"nothing", nil, LevelOK},
	}
	for _, tt := range tests {
		if got := worstLevel(tt.in); got != tt.want {
			t.Errorf("%s: worstLevel = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLowestMX(t *testing.T) {
	got := lowestMX([]MXHost{
		{Host: "backup.example.com", Preference: 20},
		{Host: "mail.example.com", Preference: 10},
		{Host: "last.example.com", Preference: 30},
	})
	if got != "mail.example.com" {
		t.Errorf("lowestMX = %q, want mail.example.com", got)
	}
}
