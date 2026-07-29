package maildns

import (
	"reflect"
	"testing"
)

func TestFindDMARC(t *testing.T) {
	tests := []struct {
		name          string
		txts          []string
		wantFound     bool
		wantCount     int
		wantPolicy    string
		wantSubdomain string
		wantPct       int
		wantRUA       []string
		wantRUF       []string
	}{
		{
			name: "monitoring only", txts: []string{"v=DMARC1; p=none"},
			wantFound: true, wantCount: 1, wantPolicy: "none", wantPct: 100,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "reject with reports", txts: []string{"v=DMARC1;p=reject;pct=50;rua=mailto:a@b.com,mailto:c@d.com"},
			wantFound: true, wantCount: 1, wantPolicy: "reject", wantPct: 50,
			wantRUA: []string{"mailto:a@b.com", "mailto:c@d.com"}, wantRUF: []string{},
		},
		{
			name: "subdomain policy", txts: []string{"v=DMARC1; p=none; sp=quarantine"},
			wantFound: true, wantCount: 1, wantPolicy: "none", wantSubdomain: "quarantine", wantPct: 100,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "failure reports", txts: []string{"v=DMARC1; p=reject; ruf=mailto:f@b.com"},
			wantFound: true, wantCount: 1, wantPolicy: "reject", wantPct: 100,
			wantRUA: []string{}, wantRUF: []string{"mailto:f@b.com"},
		},
		{
			name: "pct zero", txts: []string{"v=DMARC1; p=reject; pct=0"},
			wantFound: true, wantCount: 1, wantPolicy: "reject", wantPct: 0,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "pct above 100 is clamped", txts: []string{"v=DMARC1; p=reject; pct=101"},
			wantFound: true, wantCount: 1, wantPolicy: "reject", wantPct: 100,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "negative pct is clamped", txts: []string{"v=DMARC1; p=reject; pct=-5"},
			wantFound: true, wantCount: 1, wantPolicy: "reject", wantPct: 0,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "tags in any order", txts: []string{"v=DMARC1; rua=mailto:a@b.com; pct=25; p=quarantine"},
			wantFound: true, wantCount: 1, wantPolicy: "quarantine", wantPct: 25,
			wantRUA: []string{"mailto:a@b.com"}, wantRUF: []string{},
		},
		{
			name: "trailing semicolon", txts: []string{"v=DMARC1; p=reject;"},
			wantFound: true, wantCount: 1, wantPolicy: "reject", wantPct: 100,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "whitespace around equals", txts: []string{"v=DMARC1 ; p = reject ; pct = 40"},
			wantFound: true, wantCount: 1, wantPolicy: "reject", wantPct: 40,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "mixed case normalises", txts: []string{"V=DMARC1; P=REJECT"},
			wantFound: true, wantCount: 1, wantPolicy: "reject", wantPct: 100,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "unknown tags ignored", txts: []string{"v=DMARC1; p=none; fo=1; rf=afrf; ri=86400; adkim=s"},
			wantFound: true, wantCount: 1, wantPolicy: "none", wantPct: 100,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "no policy tag", txts: []string{"v=DMARC1; rua=mailto:a@b.com"},
			wantFound: true, wantCount: 1, wantPolicy: "", wantPct: 100,
			wantRUA: []string{"mailto:a@b.com"}, wantRUF: []string{},
		},
		{
			name: "two records", txts: []string{"v=DMARC1; p=none", "v=DMARC1; p=reject"},
			wantFound: true, wantCount: 2, wantPolicy: "none", wantPct: 100,
			wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "not a DMARC record", txts: []string{"some other text"},
			wantFound: false, wantPct: 100, wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "the tag mentioned mid string is not a record", txts: []string{"note v=DMARC1; p=reject"},
			wantFound: false, wantPct: 100, wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "tag glued to more text", txts: []string{"v=DMARC12; p=reject"},
			wantFound: false, wantPct: 100, wantRUA: []string{}, wantRUF: []string{},
		},
		{
			name: "nothing at all", txts: nil,
			wantFound: false, wantPct: 100, wantRUA: []string{}, wantRUF: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findDMARC(tt.txts)
			if got.Found != tt.wantFound {
				t.Fatalf("found = %v, want %v", got.Found, tt.wantFound)
			}
			if got.Count != tt.wantCount {
				t.Errorf("count = %d, want %d", got.Count, tt.wantCount)
			}
			if got.Policy != tt.wantPolicy {
				t.Errorf("policy = %q, want %q", got.Policy, tt.wantPolicy)
			}
			if got.Subdomain != tt.wantSubdomain {
				t.Errorf("subdomain = %q, want %q", got.Subdomain, tt.wantSubdomain)
			}
			if got.Pct != tt.wantPct {
				t.Errorf("pct = %d, want %d", got.Pct, tt.wantPct)
			}
			if !reflect.DeepEqual(got.RUA, tt.wantRUA) {
				t.Errorf("rua = %v, want %v", got.RUA, tt.wantRUA)
			}
			if !reflect.DeepEqual(got.RUF, tt.wantRUF) {
				t.Errorf("ruf = %v, want %v", got.RUF, tt.wantRUF)
			}
		})
	}
}

// TestDMARCSlicesAreNeverNil uses the input that reaches the guard: a domain
// with no DMARC record at all, where nothing ever assigns RUA or RUF.
func TestDMARCSlicesAreNeverNil(t *testing.T) {
	got := findDMARC(nil)
	if got.RUA == nil || got.RUF == nil {
		t.Fatal("RUA or RUF is nil, which marshals to JSON null")
	}
}

func TestSplitTag(t *testing.T) {
	tests := []struct {
		in        string
		wantName  string
		wantValue string
		wantOK    bool
	}{
		{"p=reject", "p", "reject", true},
		{" p = reject ", "p", "reject", true},
		{"P=REJECT", "p", "REJECT", true},
		{"p=", "p", "", true},
		{"noequals", "", "", false},
		{"=value", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		name, value, ok := splitTag(tt.in)
		if ok != tt.wantOK || name != tt.wantName || value != tt.wantValue {
			t.Errorf("splitTag(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.in, name, value, ok, tt.wantName, tt.wantValue, tt.wantOK)
		}
	}
}
