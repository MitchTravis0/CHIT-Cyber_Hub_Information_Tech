package dnscmp

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeParams(t *testing.T) {
	long := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." +
		strings.Repeat("c", 63) + "." + strings.Repeat("d", 61) // 253 characters
	tooLong := long + "e"

	tests := []struct {
		name     string
		in       Params
		wantName string
		wantType string
		wantErr  string
	}{
		{"plain name", Params{Name: "example.com"}, "example.com", "A", ""},
		{"upper case lowered", Params{Name: "Example.COM"}, "example.com", "A", ""},
		{"trailing dot dropped", Params{Name: "example.com."}, "example.com", "A", ""},
		{"whitespace trimmed", Params{Name: "  example.com  "}, "example.com", "A", ""},
		{"service label", Params{Name: "_ldap._tcp.example.com"}, "_ldap._tcp.example.com", "A", ""},
		{"253 characters accepted", Params{Name: long}, long, "A", ""},
		{"254 characters rejected", Params{Name: tooLong}, "", "", "is not a host name"},
		{"label of 64 rejected", Params{Name: strings.Repeat("a", 64) + ".com"}, "", "", "is not a host name"},
		{"leading hyphen rejected", Params{Name: "-bad.example.com"}, "", "", "is not a host name"},
		{"trailing hyphen rejected", Params{Name: "bad-.example.com"}, "", "", "is not a host name"},
		{"empty label rejected", Params{Name: "bad..example.com"}, "", "", "is not a host name"},
		{"space rejected", Params{Name: "exa mple"}, "", "", "is not a host name"},
		{"empty rejected", Params{Name: ""}, "", "", "Type a name to look up"},
		{"whitespace only rejected", Params{Name: "   "}, "", "", "Type a name to look up"},
		{"ipv4 rejected", Params{Name: "192.168.1.1"}, "", "", "is an address, not a name"},
		{"ipv6 rejected", Params{Name: "2606:4700::1111"}, "", "", "is an address, not a name"},
		{"type defaults to A", Params{Name: "example.com", Type: ""}, "example.com", "A", ""},
		{"type upper cased", Params{Name: "example.com", Type: "mx"}, "example.com", "MX", ""},
		{"PTR rejected", Params{Name: "example.com", Type: "PTR"}, "", "", "cannot compare PTR records"},
		{"SRV rejected", Params{Name: "example.com", Type: "SRV"}, "", "", "cannot compare SRV records"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.normalize()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("want an error containing %q, got none", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize errored: %v", err)
			}
			if got.name != tt.wantName {
				t.Errorf("name = %q, want %q", got.name, tt.wantName)
			}
			if got.typ != tt.wantType {
				t.Errorf("type = %q, want %q", got.typ, tt.wantType)
			}
		})
	}
}

func TestNormalizeServers(t *testing.T) {
	tests := []struct {
		name      string
		in        []string
		wantCount int
		wantErr   string
	}{
		{"nothing ticked falls back to the system resolver", nil, 1, ""},
		{"empty string is the system resolver", []string{""}, 1, ""},
		{"eight accepted", []string{"", "1.1.1.1", "8.8.8.8", "9.9.9.9", "1.0.0.1", "8.8.4.4", "149.112.112.112", "208.67.222.222"}, 8, ""},
		{"nine rejected", []string{"", "1.1.1.1", "8.8.8.8", "9.9.9.9", "1.0.0.1", "8.8.4.4", "149.112.112.112", "208.67.222.222", "208.67.220.220"}, 0, "at most 8 resolvers"},
		{"duplicates collapse", []string{"1.1.1.1", "1.1.1.1", "8.8.8.8"}, 2, ""},
		{"a name is rejected", []string{"dc01"}, 0, "is not one"},
		{"a bad port is rejected", []string{"1.1.1.1:0"}, 0, "is not one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeServers(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeServers errored: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Errorf("got %d servers, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestNormalizeParamsTimeout(t *testing.T) {
	tests := []struct {
		in      int
		want    int
		wantErr bool
	}{
		{0, 3000, false},
		{199, 0, true},
		{200, 200, false},
		{15000, 15000, false},
		{15001, 0, true},
		{-1, 0, true},
	}
	for _, tt := range tests {
		got, err := Params{Name: "example.com", TimeoutMS: tt.in}.normalize()
		if tt.wantErr {
			if err == nil {
				t.Errorf("timeout %d was accepted", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("timeout %d errored: %v", tt.in, err)
			continue
		}
		if got.timeoutMS != tt.want {
			t.Errorf("timeout %d became %d, want %d", tt.in, got.timeoutMS, tt.want)
		}
	}
}

func TestNormalizeValues(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"sorted", []string{"b.example", "a.example"}, []string{"a.example", "b.example"}},
		{"same set in a different order compares equal", []string{"a.example", "b.example"}, []string{"a.example", "b.example"}},
		{"trailing dots removed", []string{"a.example."}, []string{"a.example"}},
		{"lower cased", []string{"A.Example"}, []string{"a.example"}},
		{"duplicates collapsed", []string{"a.example", "a.example"}, []string{"a.example"}},
		{"empties dropped", []string{"", "a.example"}, []string{"a.example"}},
		{"nothing", nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeValues(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalizeValues(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func answer(label string, status string, ms float64, values ...string) Answer {
	return Answer{Server: label, Label: label, Values: values, Status: status, QueryMS: ms}
}

func TestMajority(t *testing.T) {
	tests := []struct {
		name          string
		in            []Answer
		wantMajority  []string
		wantCount     int
		wantAnswered  int
		wantAgreement bool
	}{
		{
			name: "three against two",
			in: []Answer{
				answer("a", StatusOK, 1, "1.1.1.1"),
				answer("b", StatusOK, 1, "1.1.1.1"),
				answer("c", StatusOK, 1, "1.1.1.1"),
				answer("d", StatusOK, 1, "2.2.2.2"),
				answer("e", StatusOK, 1, "2.2.2.2"),
			},
			wantMajority: []string{"1.1.1.1"}, wantCount: 3, wantAnswered: 5, wantAgreement: false,
		},
		{
			name: "a tie goes to the earlier request position",
			in: []Answer{
				answer("a", StatusOK, 1, "1.1.1.1"),
				answer("b", StatusOK, 1, "2.2.2.2"),
				answer("c", StatusOK, 1, "1.1.1.1"),
				answer("d", StatusOK, 1, "2.2.2.2"),
			},
			wantMajority: []string{"1.1.1.1"}, wantCount: 2, wantAnswered: 4, wantAgreement: false,
		},
		{
			name: "everyone agrees",
			in: []Answer{
				answer("a", StatusOK, 1, "1.1.1.1"),
				answer("b", StatusOK, 1, "1.1.1.1"),
			},
			wantMajority: []string{"1.1.1.1"}, wantCount: 2, wantAnswered: 2, wantAgreement: true,
		},
		{
			name:         "one resolver only",
			in:           []Answer{answer("a", StatusOK, 1, "1.1.1.1")},
			wantMajority: []string{"1.1.1.1"}, wantCount: 1, wantAnswered: 1, wantAgreement: true,
		},
		{
			name: "an empty answer can win the majority",
			in: []Answer{
				answer("a", StatusEmpty, 1),
				answer("b", StatusEmpty, 1),
				answer("c", StatusOK, 1, "1.1.1.1"),
			},
			wantMajority: []string{}, wantCount: 2, wantAnswered: 3, wantAgreement: false,
		},
		{
			name: "everyone errored",
			in: []Answer{
				answer("a", StatusError, 1),
				answer("b", StatusError, 1),
			},
			wantMajority: []string{}, wantCount: 0, wantAnswered: 0, wantAgreement: true,
		},
		{
			name: "an errored resolver is not counted and is never out of step",
			in: []Answer{
				answer("a", StatusOK, 1, "1.1.1.1"),
				answer("b", StatusError, 1),
			},
			wantMajority: []string{"1.1.1.1"}, wantCount: 1, wantAnswered: 1, wantAgreement: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Comparison{Name: "example.com", Type: "A", Answers: tt.in}
			Summarize(c)
			if !reflect.DeepEqual(c.Majority, tt.wantMajority) {
				t.Errorf("majority = %v, want %v", c.Majority, tt.wantMajority)
			}
			if c.MajorityCount != tt.wantCount {
				t.Errorf("majorityCount = %d, want %d", c.MajorityCount, tt.wantCount)
			}
			if c.Answered != tt.wantAnswered {
				t.Errorf("answered = %d, want %d", c.Answered, tt.wantAnswered)
			}
			if c.Agree != tt.wantAgreement {
				t.Errorf("agree = %v, want %v", c.Agree, tt.wantAgreement)
			}
		})
	}
}

func TestInStep(t *testing.T) {
	c := &Comparison{Name: "example.com", Type: "A", Answers: []Answer{
		answer("a", StatusOK, 1, "1.1.1.1"),
		answer("b", StatusOK, 1, "1.1.1.1"),
		answer("c", StatusOK, 1, "2.2.2.2"),
		answer("d", StatusEmpty, 1),
		answer("e", StatusError, 1),
	}}
	Summarize(c)

	want := []bool{true, true, false, false, true}
	for i, a := range c.Answers {
		if a.InStep != want[i] {
			t.Errorf("row %d (%s, %s) inStep = %v, want %v", i, a.Label, a.Status, a.InStep, want[i])
		}
	}
}

// TestFastestIgnoresTheSystemResolver pins the caveat: the system row may be
// served from a local cache or a stub, so its time is not comparable with a
// real network query and must not win the ranking.
func TestFastestIgnoresTheSystemResolver(t *testing.T) {
	c := &Comparison{Name: "example.com", Type: "A", Answers: []Answer{
		{Server: "", Label: SystemResolverLabel, Values: []string{"1.1.1.1"}, Status: StatusOK, QueryMS: 1},
		{Server: "1.1.1.1", Label: "1.1.1.1", Values: []string{"1.1.1.1"}, Status: StatusOK, QueryMS: 12},
		{Server: "8.8.8.8", Label: "8.8.8.8", Values: []string{"1.1.1.1"}, Status: StatusOK, QueryMS: 30},
	}}
	Summarize(c)

	if c.FastestLabel != "1.1.1.1" {
		t.Errorf("fastest = %q, want 1.1.1.1: the system resolver must not win", c.FastestLabel)
	}
	if c.FastestMS != 12 {
		t.Errorf("fastestMs = %v, want 12", c.FastestMS)
	}
}

func TestFastestIgnoresErroredRows(t *testing.T) {
	c := &Comparison{Name: "example.com", Type: "A", Answers: []Answer{
		{Server: "1.1.1.1", Label: "1.1.1.1", Status: StatusError, QueryMS: 0.1},
		{Server: "8.8.8.8", Label: "8.8.8.8", Values: []string{"1.1.1.1"}, Status: StatusOK, QueryMS: 30},
	}}
	Summarize(c)
	if c.FastestLabel != "8.8.8.8" {
		t.Errorf("fastest = %q, want 8.8.8.8: a resolver that failed instantly did not answer fastest", c.FastestLabel)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name         string
		typ          string
		answers      []Answer
		wantLevel    string
		wantHeadline string
		wantAdvice   string
	}{
		{
			"one resolver only", "A",
			[]Answer{answer("a", StatusOK, 1, "1.1.1.1")},
			LevelOK, "Only one resolver was asked", "",
		},
		{
			"all agree on an address", "A",
			[]Answer{answer("a", StatusOK, 1, "93.184.216.34"), answer("b", StatusOK, 1, "93.184.216.34")},
			LevelOK, "All 2 resolvers agree: example.com is 93.184.216.34.", "",
		},
		{
			"all agree on an IPv6 address", "AAAA",
			[]Answer{answer("a", StatusOK, 1, "2606:2800::1"), answer("b", StatusOK, 1, "2606:2800::1")},
			LevelOK, "All 2 resolvers agree: example.com is 2606:2800::1.", "",
		},
		{
			// "example.com is ns1.example.com" is nonsense. Only an address
			// answers the question "what is this name"; everything else has to
			// name the record type.
			"all agree on a name server list", "NS",
			[]Answer{answer("a", StatusOK, 1, "ns1.example.com"), answer("b", StatusOK, 1, "ns1.example.com")},
			LevelOK, "All 2 resolvers agree on the NS records for example.com: ns1.example.com.", "",
		},
		{
			"all agree on a TXT record", "TXT",
			[]Answer{answer("a", StatusOK, 1, "v=spf1 -all"), answer("b", StatusOK, 1, "v=spf1 -all")},
			LevelOK, "All 2 resolvers agree on the TXT records for example.com: v=spf1 -all.", "",
		},
		{
			"all agree there is no record", "A",
			[]Answer{answer("a", StatusEmpty, 1), answer("b", StatusEmpty, 1)},
			LevelOK, "All 2 resolvers agree: there is no A record for example.com.", "",
		},
		{
			"some disagree", "A",
			[]Answer{
				answer("a", StatusOK, 1, "1.1.1.1"),
				answer("b", StatusOK, 1, "1.1.1.1"),
				answer("c", StatusOK, 1, "1.1.1.1"),
				answer("d", StatusOK, 1, "2.2.2.2"),
				answer("e", StatusOK, 1, "2.2.2.2"),
			},
			LevelWarn, "3 of 5 resolvers say 1.1.1.1, the other 2 say something else.", "stale cache",
		},
		{
			"nothing answered", "A",
			[]Answer{answer("a", StatusError, 1), answer("b", StatusError, 1)},
			LevelError, "None of the resolvers answered", "port 53",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Comparison{Name: "example.com", Type: tt.typ, Answers: tt.answers}
			Summarize(c)
			if c.Level != tt.wantLevel {
				t.Errorf("level = %q, want %q (headline %q)", c.Level, tt.wantLevel, c.Headline)
			}
			if !strings.Contains(c.Headline, tt.wantHeadline) {
				t.Errorf("headline\n got %q\nwant it to contain %q", c.Headline, tt.wantHeadline)
			}
			if tt.wantAdvice == "" {
				if c.Advice != "" {
					t.Errorf("advice = %q, want empty", c.Advice)
				}
				return
			}
			if !strings.Contains(c.Advice, tt.wantAdvice) {
				t.Errorf("advice %q does not contain %q", c.Advice, tt.wantAdvice)
			}
		})
	}
}

// TestHeadlineTruncatesValues pins the literal 3: a domain with twenty
// addresses must not produce a headline nobody can read.
func TestHeadlineTruncatesValues(t *testing.T) {
	if headlineValues != 3 {
		t.Fatalf("headlineValues = %d, want 3", headlineValues)
	}
	tests := []struct {
		in   []string
		want string
	}{
		{[]string{}, "nothing"},
		{[]string{"a"}, "a"},
		{[]string{"a", "b", "c"}, "a, b, c"},
		{[]string{"a", "b", "c", "d"}, "a, b, c and 1 more"},
		{[]string{"a", "b", "c", "d", "e"}, "a, b, c and 2 more"},
	}
	for _, tt := range tests {
		if got := listValues(tt.in); got != tt.want {
			t.Errorf("listValues(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestNoNilSlices uses the inputs that actually reach the guards: an Answer
// whose Values was never initialised, and a majority answer set that is empty
// because "there is no such record" won the vote. Both would marshal to JSON
// null and render as a blank table.
func TestNoNilSlices(t *testing.T) {
	c := &Comparison{Name: "example.com", Type: "A", Answers: []Answer{
		{Server: "a", Label: "a", Status: StatusError},
		{Server: "b", Label: "b", Status: StatusEmpty},
		{Server: "c", Label: "c", Status: StatusEmpty},
	}}
	Summarize(c)
	if c.Majority == nil {
		t.Error("Majority is nil, which marshals to JSON null")
	}
	for _, a := range c.Answers {
		if a.Values == nil {
			t.Errorf("row %s has a nil Values, which marshals to JSON null", a.Label)
		}
	}
}

func TestServersListsTheSystemResolverFirstAndDoesNotRepeat(t *testing.T) {
	got := Servers()
	if len(got) < 4 {
		t.Fatalf("got %d options, want at least the system resolver plus three public ones", len(got))
	}
	if got[0].ID != "" || got[0].Label != SystemResolverLabel {
		t.Errorf("first option is %+v, want the system resolver", got[0])
	}
	seen := make(map[string]bool)
	for _, o := range got {
		if seen[o.ID] {
			t.Errorf("option %q appears twice", o.ID)
		}
		seen[o.ID] = true
		if strings.TrimSpace(o.Label) == "" || strings.TrimSpace(o.Detail) == "" {
			t.Errorf("option %+v has a blank label or detail", o)
		}
	}
	for _, want := range []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"} {
		if !seen[want] {
			t.Errorf("the public resolver %s is missing", want)
		}
	}
}

func TestCompareAgainstLocalResolvers(t *testing.T) {
	old := startFakeDNS(t, map[uint16][]record{typeA: {aRecord("203.0.113.10")}})
	new1 := startFakeDNS(t, map[uint16][]record{typeA: {aRecord("198.51.100.20")}})
	new2 := startFakeDNS(t, map[uint16][]record{typeA: {aRecord("198.51.100.20")}})
	silent := startSilentDNS(t)

	got, err := Compare(context.Background(), Params{
		Name:      "example.com",
		Type:      "A",
		Servers:   []string{new1.addr(), new2.addr(), old.addr(), silent.addr()},
		TimeoutMS: 700,
	})
	if err != nil {
		t.Fatalf("Compare errored: %v", err)
	}
	if len(got.Answers) != 4 {
		t.Fatalf("got %d rows, want 4", len(got.Answers))
	}
	if got.Agree {
		t.Error("agree = true, but two resolvers gave different addresses")
	}
	if got.Level != LevelWarn {
		t.Errorf("level = %q, want %q", got.Level, LevelWarn)
	}
	if want := "2 of 3 resolvers say 198.51.100.20, the other 1 say something else."; got.Headline != want {
		t.Errorf("headline = %q, want %q", got.Headline, want)
	}
	if !reflect.DeepEqual(got.Majority, []string{"198.51.100.20"}) {
		t.Errorf("majority = %v, want [198.51.100.20]", got.Majority)
	}

	// Rows come back in the order they were asked for, whatever order they
	// answered in.
	if got.Answers[0].Label != new1.addr() || got.Answers[3].Label != silent.addr() {
		t.Errorf("rows are out of request order: %q ... %q", got.Answers[0].Label, got.Answers[3].Label)
	}
	if got.Answers[2].InStep {
		t.Error("the stale resolver is marked in step")
	}
	if got.Answers[3].Status != StatusError {
		t.Errorf("the silent resolver has status %q, want %q", got.Answers[3].Status, StatusError)
	}
	if !strings.Contains(got.Answers[3].Message, "did not answer within 700 ms") {
		t.Errorf("the silent resolver's message is %q", got.Answers[3].Message)
	}
	if got.CheckedAt == "" {
		t.Error("checkedAt is blank")
	}
}

func TestCompareEveryRecordType(t *testing.T) {
	f := startAliasDNS(t, "target.example.com.", map[uint16][]record{
		typeA:     {aRecord("203.0.113.10"), aRecord("203.0.113.11")},
		typeAAAA:  {aaaaRecord("2001:db8::1")},
		typeCNAME: {nameRecord(typeCNAME, "target.example.com.")},
		typeMX:    {mxRecord(10, "mail.example.com."), mxRecord(20, "backup.example.com.")},
		typeTXT:   {txtRecord("v=spf1 -all")},
		typeNS:    {nameRecord(typeNS, "ns1.example.com.")},
	})

	tests := []struct {
		typ  string
		want []string
	}{
		{"A", []string{"203.0.113.10", "203.0.113.11"}},
		{"AAAA", []string{"2001:db8::1"}},
		{"CNAME", []string{"target.example.com"}},
		{"MX", []string{"10 mail.example.com", "20 backup.example.com"}},
		{"TXT", []string{"v=spf1 -all"}},
		{"NS", []string{"ns1.example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			got, err := Compare(context.Background(), Params{
				Name: "example.com", Type: tt.typ, Servers: []string{f.addr()}, TimeoutMS: 1500,
			})
			if err != nil {
				t.Fatalf("Compare errored: %v", err)
			}
			row := got.Answers[0]
			if row.Status != StatusOK {
				t.Fatalf("status = %q (%s), want ok", row.Status, row.Message)
			}
			want := normalizeValues(tt.want)
			if !reflect.DeepEqual(row.Values, want) {
				t.Errorf("values = %v, want %v", row.Values, want)
			}
		})
	}
}

// TestNullMXBecomesEmpty is one of the two sentinel cases CONVENTIONS section 7
// names: LookupMX returns a host of "." for the RFC 7505 null MX, which once
// rendered as an ok row with nothing in it.
func TestNullMXBecomesEmpty(t *testing.T) {
	f := startFakeDNS(t, map[uint16][]record{typeMX: {nullMXRecord()}})

	got, err := Compare(context.Background(), Params{
		Name: "example.com", Type: "MX", Servers: []string{f.addr()}, TimeoutMS: 1500,
	})
	if err != nil {
		t.Fatalf("Compare errored: %v", err)
	}
	row := got.Answers[0]
	if row.Status != StatusEmpty {
		t.Fatalf("status = %q, want %q: a null MX is an answer, not a record", row.Status, StatusEmpty)
	}
	if len(row.Values) != 0 {
		t.Errorf("values = %v, want none", row.Values)
	}
	if !strings.Contains(row.Message, "accepts no email at all") {
		t.Errorf("message = %q", row.Message)
	}
}

// TestCNAMEEchoBecomesEmpty is the other sentinel: LookupCNAME hands back the
// name you asked for when there is no alias.
func TestCNAMEEchoBecomesEmpty(t *testing.T) {
	f := startFakeDNS(t, map[uint16][]record{typeCNAME: {nameRecord(typeCNAME, "example.com.")}})

	got, err := Compare(context.Background(), Params{
		Name: "example.com", Type: "CNAME", Servers: []string{f.addr()}, TimeoutMS: 1500,
	})
	if err != nil {
		t.Fatalf("Compare errored: %v", err)
	}
	row := got.Answers[0]
	if row.Status != StatusEmpty {
		t.Fatalf("status = %q, want %q: a CNAME pointing at the name itself is no alias", row.Status, StatusEmpty)
	}
	if !strings.Contains(row.Message, "no CNAME record") {
		t.Errorf("message = %q", row.Message)
	}
}

func TestNoRecordIsAnAnswerNotAnError(t *testing.T) {
	withRecord := startFakeDNS(t, map[uint16][]record{typeA: {aRecord("203.0.113.10")}})
	without := startFakeDNS(t, map[uint16][]record{})

	got, err := Compare(context.Background(), Params{
		Name: "example.com", Type: "A",
		Servers: []string{withRecord.addr(), without.addr()}, TimeoutMS: 1500,
	})
	if err != nil {
		t.Fatalf("Compare errored: %v", err)
	}
	if got.Answers[1].Status != StatusEmpty {
		t.Fatalf("status = %q, want %q", got.Answers[1].Status, StatusEmpty)
	}
	if got.Answers[1].InStep {
		t.Error("a resolver saying there is no record while another gives an address is out of step")
	}
	if got.Agree {
		t.Error("agree = true, but one resolver has no record and another does")
	}
}

func TestCompareRejectsBadInputBeforeAsking(t *testing.T) {
	f := startFakeDNS(t, map[uint16][]record{typeA: {aRecord("203.0.113.10")}})

	if _, err := Compare(context.Background(), Params{
		Name: "192.168.1.1", Servers: []string{f.addr()},
	}); err == nil {
		t.Fatal("an IP address was accepted as a name")
	}
	if f.count() != 0 {
		t.Errorf("the fake was asked %d times: validation must happen before any query", f.count())
	}
}

func TestEveryNonOKRowCarriesASentence(t *testing.T) {
	empty := startFakeDNS(t, map[uint16][]record{})
	silent := startSilentDNS(t)

	got, err := Compare(context.Background(), Params{
		Name: "example.com", Type: "A",
		Servers: []string{empty.addr(), silent.addr()}, TimeoutMS: 500,
	})
	if err != nil {
		t.Fatalf("Compare errored: %v", err)
	}
	for _, row := range got.Answers {
		if row.Status != StatusOK && strings.TrimSpace(row.Message) == "" {
			t.Errorf("row %s has status %q and no message", row.Label, row.Status)
		}
	}
}
