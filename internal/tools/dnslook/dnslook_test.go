package dnslook

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantIP  bool
		wantErr bool
	}{
		{"plain name", "example.com", "example.com", false, false},
		{"upper case and trailing dot", "EXAMPLE.COM.", "example.com", false, false},
		{"surrounding spaces", "  example.com  ", "example.com", false, false},
		{"srv style underscores", "_sip._tcp.example.com", "_sip._tcp.example.com", false, false},
		{"ipv4", "192.168.1.1", "192.168.1.1", true, false},
		{"ipv6 loopback", "::1", "::1", true, false},
		{"ipv6", "2001:db8::1", "2001:db8::1", true, false},
		{"many labels", "a.b.c.d.e.f.g", "a.b.c.d.e.f.g", false, false},
		{"single label", "wsus", "wsus", false, false},
		{"label of 63", strings.Repeat("a", 63) + ".com", strings.Repeat("a", 63) + ".com", false, false},
		{"label of 64", strings.Repeat("a", 64) + ".com", "", false, true},
		{"name of 253", strings.Repeat("ab.", 83) + "aaaa", strings.Repeat("ab.", 83) + "aaaa", false, false},
		{"name of 254", strings.Repeat("ab.", 84) + "ab", "", false, true},
		{"leading hyphen", "-bad.com", "", false, true},
		{"trailing hyphen", "bad-.com", "", false, true},
		{"empty label", "a..b", "", false, true},
		{"space inside", "exa mple.com", "", false, true},
		{"empty", "", "", false, true},
		{"underscore inside a label", "ex_ample.com", "", false, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, isIP, err := normalizeName(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("normalizeName(%q) accepted a bad name", c.in)
				}
				if code := core.CodeOf(err); code != core.CodeInvalidInput {
					t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
				}
				if !strings.Contains(err.Error(), "is not a host name or an IP address") {
					t.Errorf("message %q does not explain what is wanted", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeName(%q) returned %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("name = %q, want %q", got, c.want)
			}
			if isIP != c.wantIP {
				t.Errorf("isIP = %v, want %v", isIP, c.wantIP)
			}
		})
	}
}

func TestNormalizeDefaults(t *testing.T) {
	st, err := Params{Name: "example.com", Types: []string{"A"}}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	if st.timeoutMS != 3000 || st.timeout != 3*time.Second {
		t.Errorf("timeout = %d ms (%s), want 3000 ms", st.timeoutMS, st.timeout)
	}
	if len(st.servers) != 1 || st.servers[0].id != "" {
		t.Fatalf("servers = %+v, want the system resolver only", st.servers)
	}
	if st.servers[0].label != SystemResolverLabel || st.servers[0].addr != "" {
		t.Errorf("system resolver target = %+v", st.servers[0])
	}
}

func TestNormalizeTypes(t *testing.T) {
	st, err := Params{Name: "example.com", Types: []string{"a", "MX", "a"}}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	if len(st.types) != 2 || st.types[0] != "A" || st.types[1] != "MX" {
		t.Errorf("types = %v, want [A MX] upper-cased and de-duplicated", st.types)
	}

	_, err = Params{Name: "example.com", Types: []string{"SOA"}}.normalize()
	if err == nil {
		t.Fatal("SOA was accepted")
	}
	want := "CHIT does not look up SOA records. Choose from A, AAAA, CNAME, MX, TXT, NS, SRV and PTR."
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}

	_, err = Params{Name: "example.com"}.normalize()
	if err == nil {
		t.Fatal("an empty type list was accepted")
	}
	if err.Error() != "Pick at least one record type to look up." {
		t.Errorf("message = %q", err.Error())
	}
}

func TestNormalizeServers(t *testing.T) {
	cases := []struct {
		name    string
		servers []string
		wantIDs []string
		wantErr string
	}{
		{"system resolver", []string{""}, []string{""}, ""},
		{"one public server", []string{"8.8.8.8"}, []string{"8.8.8.8"}, ""},
		{"system plus public", []string{"", "8.8.8.8"}, []string{"", "8.8.8.8"}, ""},
		{"duplicates collapsed", []string{"8.8.8.8", "8.8.8.8", ""}, []string{"8.8.8.8", ""}, ""},
		{"six is the maximum", []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6"},
			[]string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6"}, ""},
		{"seven is too many", []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7"},
			nil, "Ask at most 6 servers at a time. You picked 7."},
		{"a name is not a server", []string{"dns.google"}, nil,
			`Enter the DNS server as an IP address, for example 8.8.8.8 or 192.168.1.10. "dns.google" is not one.`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, err := Params{Name: "example.com", Types: []string{"A"}, Servers: c.servers}.normalize()
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("%v was accepted", c.servers)
				}
				if err.Error() != c.wantErr {
					t.Errorf("message = %q, want %q", err.Error(), c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize() returned %v", err)
			}
			if len(st.servers) != len(c.wantIDs) {
				t.Fatalf("servers = %+v, want ids %v", st.servers, c.wantIDs)
			}
			for i, want := range c.wantIDs {
				if st.servers[i].id != want {
					t.Errorf("server %d = %q, want %q", i, st.servers[i].id, want)
				}
			}
		})
	}
}

func TestNormalizeRejects(t *testing.T) {
	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{"empty name", Params{Types: []string{"A"}},
			"Enter a name or an IP address to look up, for example example.com or 192.168.1.1."},
		{"whitespace name", Params{Name: "   ", Types: []string{"A"}},
			"Enter a name or an IP address to look up, for example example.com or 192.168.1.1."},
		{"300 character name", Params{Name: strings.Repeat("ab.", 100), Types: []string{"A"}},
			"is not a host name or an IP address"},
		{"timeout too short", Params{Name: "example.com", Types: []string{"A"}, TimeoutMS: 199},
			"The wait for an answer must be between 0.2 and 15 seconds. 199 ms is outside that."},
		{"timeout too long", Params{Name: "example.com", Types: []string{"A"}, TimeoutMS: 15001},
			"The wait for an answer must be between 0.2 and 15 seconds. 15001 ms is outside that."},
		{"negative timeout", Params{Name: "example.com", Types: []string{"A"}, TimeoutMS: -1},
			"The wait for an answer must be between 0.2 and 15 seconds. -1 ms is outside that."},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.params.normalize()
			if err == nil {
				t.Fatal("bad parameters were accepted")
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("message %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestNormalizeAcceptsTheLimits(t *testing.T) {
	for _, ms := range []int{200, 15000} {
		st, err := Params{Name: "example.com", Types: []string{"A"}, TimeoutMS: ms}.normalize()
		if err != nil {
			t.Fatalf("%d ms was rejected: %v", ms, err)
		}
		if st.timeoutMS != ms {
			t.Errorf("timeout = %d, want %d", st.timeoutMS, ms)
		}
	}

	st, err := Params{Name: "example.com", Types: SupportedTypes}.normalize()
	if err != nil {
		t.Fatalf("all eight types were rejected: %v", err)
	}
	if len(st.types) != 8 {
		t.Errorf("types = %v, want all eight", st.types)
	}
}

func TestIPInputForcesPTR(t *testing.T) {
	st, err := Params{Name: "8.8.8.8", Types: []string{"A", "MX"}}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	if len(st.types) != 1 || st.types[0] != "PTR" {
		t.Errorf("types = %v, want [PTR]", st.types)
	}
	if !st.isIP {
		t.Error("isIP = false for an address")
	}
}

func TestCompareAnswers(t *testing.T) {
	ok := func(server, typ, value string) Record {
		return Record{Server: server, Type: typ, Value: value, Status: StatusOK}
	}
	cases := []struct {
		name      string
		records   []Record
		wantAgree bool
	}{
		{"one server only", []Record{ok("8.8.8.8", "A", "10.0.0.1")}, true},
		{"two servers agreeing", []Record{
			ok("8.8.8.8", "A", "10.0.0.1"),
			ok("1.1.1.1", "A", "10.0.0.1"),
		}, true},
		{"same addresses in a different order", []Record{
			ok("8.8.8.8", "A", "10.0.0.1"), ok("8.8.8.8", "A", "10.0.0.2"),
			ok("1.1.1.1", "A", "10.0.0.2"), ok("1.1.1.1", "A", "10.0.0.1"),
		}, true},
		{"two servers disagreeing", []Record{
			ok("8.8.8.8", "A", "10.0.0.1"),
			ok("1.1.1.1", "A", "10.0.0.9"),
		}, false},
		{"AAAA counts too", []Record{
			ok("8.8.8.8", "AAAA", "2001:db8::1"),
			ok("1.1.1.1", "AAAA", "2001:db8::2"),
		}, false},
		{"different MX records are normal", []Record{
			ok("8.8.8.8", "A", "10.0.0.1"), ok("8.8.8.8", "MX", "mail.example.com"),
			ok("1.1.1.1", "A", "10.0.0.1"), ok("1.1.1.1", "MX", "spamfilter.example.net"),
		}, true},
		{"empty and error rows are ignored", []Record{
			ok("8.8.8.8", "A", "10.0.0.1"),
			{Server: "1.1.1.1", Type: "A", Status: StatusEmpty, Message: "No A record"},
			{Server: "9.9.9.9", Type: "A", Status: StatusError, Message: "did not answer"},
		}, true},
		{"no address records at all", []Record{
			ok("8.8.8.8", "TXT", "v=spf1 -all"),
			ok("1.1.1.1", "TXT", "v=spf1 ~all"),
		}, true},
		{"nothing at all", nil, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			agree, note := compareAnswers("intranet.company.local", c.records)
			if agree != c.wantAgree {
				t.Fatalf("agree = %v, want %v", agree, c.wantAgree)
			}
			if agree && note != "" {
				t.Errorf("note = %q, want empty when the servers agree", note)
			}
			if !agree {
				want := "The servers do not agree on the address for intranet.company.local. That usually means one of them is holding a stale cache, or the internal and external copies of the zone are different."
				if note != want {
					t.Errorf("note = %q, want %q", note, want)
				}
			}
		})
	}
}

func TestSummaryNote(t *testing.T) {
	healthy := []Record{{Server: SystemResolverLabel, Type: "A", Value: "10.0.0.1", Status: StatusOK}}
	disagreeing := []Record{
		{Server: "8.8.8.8", Type: "A", Value: "10.0.0.1", Status: StatusOK},
		{Server: "1.1.1.1", Type: "A", Value: "10.0.0.9", Status: StatusOK},
	}

	t.Run("healthy", func(t *testing.T) {
		agree, note := summaryNote("example.com", false, false, healthy)
		if !agree || note != "" {
			t.Errorf("agree = %v, note = %q, want true and empty", agree, note)
		}
	})

	t.Run("an address was typed in", func(t *testing.T) {
		_, note := summaryNote("192.168.1.1", true, false, nil)
		want := "192.168.1.1 is an IP address, so CHIT did a reverse (PTR) lookup and ignored the other record types."
		if note != want {
			t.Errorf("note = %q, want %q", note, want)
		}
	})

	t.Run("the servers disagree", func(t *testing.T) {
		agree, note := summaryNote("example.com", false, false, disagreeing)
		if agree {
			t.Error("agree = true for two different addresses")
		}
		if !strings.HasPrefix(note, "The servers do not agree on the address for example.com.") {
			t.Errorf("note = %q", note)
		}
	})

	t.Run("nothing answered", func(t *testing.T) {
		records := []Record{{Server: "192.0.2.1", Type: "A", Status: StatusError, Message: "no answer"}}
		_, note := summaryNote("example.com", false, true, records)
		want := "None of the servers answered. Check that this computer can reach them on port 53."
		if note != want {
			t.Errorf("note = %q, want %q", note, want)
		}
	})

	t.Run("sentences are joined in order", func(t *testing.T) {
		_, note := summaryNote("192.168.1.1", true, false, disagreeing)
		if !strings.HasPrefix(note, "192.168.1.1 is an IP address,") {
			t.Errorf("note = %q, want the PTR sentence first", note)
		}
		if !strings.Contains(note, " The servers do not agree") {
			t.Errorf("note = %q, want both sentences joined with one space", note)
		}
	})
}

func TestServersAlwaysIncludesSystemAndPublic(t *testing.T) {
	options := Servers()
	if len(options) == 0 {
		t.Fatal("Servers() returned nothing, so the UI would offer no tick boxes")
	}
	if options[0].ID != "" || options[0].Label != SystemResolverLabel {
		t.Errorf("first option = %+v, want the system resolver", options[0])
	}

	seen := make(map[string]bool, len(options))
	for _, option := range options {
		if seen[option.ID] {
			t.Errorf("id %q appears twice", option.ID)
		}
		seen[option.ID] = true
		if option.Label == "" || option.Detail == "" {
			t.Errorf("option %+v is missing its label or detail", option)
		}
	}
	for _, want := range []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"} {
		if !seen[want] {
			t.Errorf("%s is not offered", want)
		}
	}
}

func TestStartLookupRejectsBadInputWithoutStartingAJob(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartLookup(Params{Name: "my server", Types: []string{"A"}})
	if err == nil {
		t.Fatal("a name with a space in it was accepted")
	}
	if id != "" {
		t.Errorf("job id = %q, want empty", id)
	}
	want := `"my server" is not a host name or an IP address. Try example.com or 192.168.1.1.`
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
	if n := s.jobs.Running(); n != 0 {
		t.Errorf("%d jobs running, want 0", n)
	}
}

func TestStartLookupReturnsJobID(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartLookup(Params{Name: "example.com", Types: []string{"A"}, Servers: []string{"192.0.2.1"}})
	if err != nil {
		t.Fatalf("StartLookup returned %v", err)
	}
	if id == "" {
		t.Fatal("StartLookup returned an empty job id")
	}
	_ = s.jobs.Cancel(id)
}

// TestLookupIsCancellable is the one that matters for the UI's Cancel button:
// forty-eight questions aimed at addresses that never route must stop soon
// after the job is cancelled, not run out their full wait.
func TestLookupIsCancellable(t *testing.T) {
	st, err := Params{
		Name:      "example.com",
		Types:     SupportedTypes,
		Servers:   []string{"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.5", "192.0.2.6"},
		TimeoutMS: 15000,
	}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}

	m := core.NewJobManager()
	done := make(chan error, 1)
	id := m.Start(JobKind, len(st.servers)*len(st.types), func(jc *core.JobContext) error {
		err := lookup(jc, st)
		done <- err
		return err
	})

	time.Sleep(200 * time.Millisecond)
	if err := m.Cancel(id); err != nil {
		t.Fatalf("Cancel returned %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled lookup returned %v, want context.Canceled", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the lookup kept running after it was cancelled")
	}
}
