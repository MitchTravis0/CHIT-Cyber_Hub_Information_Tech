package urlcheck

import (
	"fmt"
	"testing"
)

func findingByID(list []Finding, id string) (Finding, bool) {
	for _, f := range list {
		if f.ID == id {
			return f, true
		}
	}
	return Finding{}, false
}

// TestFindingsEveryRule pins the wording of all twenty findings, because the
// sentence is what a tech reads out to the person who was sent the link.
func TestFindingsEveryRule(t *testing.T) {
	cases := []struct {
		id       string
		severity string
		text     string
		report   Report
	}{
		{
			id:       "private-target",
			severity: "danger",
			text:     "The link points at an address on this computer or the local network (10.0.0.5). A link from outside should never do that.",
			report: Report{
				Hops: []Hop{{N: 1, URL: "http://10.0.0.5/", Host: "10.0.0.5", Error: msgBlockedHop}},
			},
		},
		{
			id:       "bad-scheme",
			severity: "danger",
			text:     "The link redirects to a javascript: address, which is code rather than a web page. CHIT did not run it. Nothing legitimate does this.",
			report: Report{
				Hops: []Hop{{N: 1, URL: "https://example.com/", Host: "example.com", Next: "javascript:alert(1)"}},
			},
		},
		{
			id:       "bad-scheme",
			severity: "danger",
			text:     "The link redirects to a data: address, which carries the whole page inside the link itself. That is how a fake login page is hidden from a mail filter. CHIT did not open it.",
			report: Report{
				Hops: []Hop{{N: 1, URL: "https://example.com/", Host: "example.com", Next: "data:text/html,<h1>hi"}},
			},
		},
		{
			id:       "bad-scheme",
			severity: "danger",
			text:     "The link redirects to a file: address, which points at a file on the computer rather than at a website. CHIT did not open it.",
			report: Report{
				Hops: []Hop{{N: 1, URL: "https://example.com/", Host: "example.com", Next: "file:///etc/passwd"}},
			},
		},
		{
			id:       "bad-scheme",
			severity: "danger",
			text:     "The link redirects to \"ms-msdt:\", which is not a web address. CHIT did not open it. A link that hands off to another program on the machine is worth asking about.",
			report: Report{
				Hops: []Hop{{N: 1, URL: "https://example.com/", Host: "example.com", Next: "ms-msdt:x"}},
			},
		},
		{
			id:       "punycode",
			severity: "danger",
			text:     "The address \"xn--80ak6aa92e.com\" is really \"аррӏе.com\". Those are not the ordinary Latin letters, which is how a fake address is made to look like a real one.",
			report: Report{
				FinalHost: HostName{Raw: "xn--80ak6aa92e.com", Decoded: "аррӏе.com", Punycode: true},
			},
		},
		{
			// A lookalike domain that redirects to its own UTF-8 spelling ends on
			// a host that is not in xn-- form, and the xn-- name the user was
			// actually sent is the start host.
			id:       "punycode",
			severity: "danger",
			text:     "The address \"xn--e1afmkfd.xn--p1ai\" is really \"пример.рф\". Those are not the ordinary Latin letters, which is how a fake address is made to look like a real one.",
			report: Report{
				StartHost: HostName{Raw: "xn--e1afmkfd.xn--p1ai", Decoded: "пример.рф", Punycode: true},
				FinalHost: HostName{Raw: "поддерживаю.рф", Decoded: "поддерживаю.рф"},
			},
		},
		{
			id:       "mixed-script",
			severity: "danger",
			text:     "The address mixes Latin letters with Cyrillic or Greek ones in the same word (\"pаypal\"). That is almost always an attempt to make a fake address look like a real one.",
			report: Report{
				FinalHost: HostName{Raw: "pаypal.com", Decoded: "pаypal.com"},
			},
		},
		{
			id:       "credentials",
			severity: "danger",
			text:     "The link has \"microsoft.com@\" in front of the address. A browser ignores everything before the @, so the page you land on is evil.example, not what the link appears to say.",
			report: Report{
				Start: "http://microsoft.com@evil.example/",
			},
		},
		{
			id:       "executable",
			severity: "danger",
			text:     "The link ends in a file called \"invoice.exe\". That downloads a program rather than opening a page. Do not run it.",
			report: Report{
				Final: "https://example.com/files/invoice.exe",
			},
		},
		{
			id:       "ip-literal",
			severity: "warn",
			text:     "The link goes to a bare IP address (93.184.216.34) instead of a name. Real services almost never send people to a number.",
			report: Report{
				Final:     "https://93.184.216.34/",
				FinalHost: HostName{Raw: "93.184.216.34", Decoded: "93.184.216.34", IsIP: true},
			},
		},
		{
			id:       "insecure",
			severity: "warn",
			text:     "The page it ends on is plain http, not https, so anything typed into it travels unencrypted. A real login page is never plain http.",
			report: Report{
				Final: "http://example.com/login",
			},
		},
		{
			id:       "downgrade",
			severity: "warn",
			text:     "The chain drops from https down to plain http part way through, at a.example.",
			report: Report{
				Hops: []Hop{{N: 1, URL: "https://a.example/", Host: "a.example", Next: "http://b.example/"}},
			},
		},
		{
			id:       "cross-domain",
			severity: "warn",
			text:     "The link starts at bit.ly and ends up at micros0ft-login.xyz, which is a different owner. That is normal for a mailing list and a red flag on a bank or Microsoft link.",
			report: Report{
				StartHost: HostName{Raw: "bit.ly"},
				FinalHost: HostName{Raw: "micros0ft-login.xyz"},
			},
		},
		{
			id:       "shortener",
			severity: "warn",
			text:     "The link goes through bit.ly, a link shortener, which hides the real address until you follow it.",
			report: Report{
				Hops: []Hop{{N: 1, URL: "https://bit.ly/3xKq", Host: "bit.ly"}},
			},
		},
		{
			id:       "free-hosting",
			severity: "warn",
			text:     "The page is hosted on github.io, which anyone can sign up to for free. A real company login page is almost never on one of these.",
			report: Report{
				FinalHost: HostName{Raw: "evil.github.io", Decoded: "evil.github.io"},
			},
		},
		{
			id:       "unusual-port",
			severity: "warn",
			text:     "The link uses port 8443 rather than the usual 80 or 443. A real website almost never does that.",
			report: Report{
				Final: "https://example.com:8443/login",
			},
		},
		{
			id:       "new-domain",
			severity: "warn",
			text:     "The domain micros0ft-login.xyz was only registered on 2026-07-14, 12 days ago. Brand new domains are what phishing campaigns run on.",
			report: Report{
				FinalHost: HostName{Raw: "micros0ft-login.xyz", Registrable: "micros0ft-login.xyz"},
				Age:       Age{Known: true, Registered: "2026-07-14", Days: 12, Human: "12 days"},
			},
		},
		{
			id:       "redirect-loop",
			severity: "warn",
			text:     "The chain loops back on itself. A page that does this usually depends on a cookie or a login that the inspector does not have.",
			report: Report{
				Hops:    []Hop{{N: 1, URL: "https://example.com/a", Host: "example.com", Next: "https://example.com/b"}},
				Stopped: stoppedLoop,
			},
		},
		{
			id:       "hop-cap",
			severity: "warn",
			text:     "The chain was still redirecting after 3 hops, so where it really ends is not known.",
			report: Report{
				Hops: []Hop{
					{N: 1, URL: "https://example.com/1", Host: "example.com", Next: "https://example.com/2"},
					{N: 2, URL: "https://example.com/2", Host: "example.com", Next: "https://example.com/3"},
					{N: 3, URL: "https://example.com/3", Host: "example.com", Next: "https://example.com/4"},
				},
				Stopped: fmt.Sprintf(stoppedHopCap, 3),
			},
		},
		{
			id:       "no-location",
			severity: "warn",
			text:     "One step said it was redirecting but did not say where to, so the chain stops there.",
			report: Report{
				Hops: []Hop{{N: 1, URL: "https://example.com/", Host: "example.com", Status: 302, Error: msgNoLocation}},
			},
		},
		{
			id:       "long-chain",
			severity: "info",
			text:     "It took 3 redirects to get there. That is normal for advertising and tracking links, and unusual for a link a person sent you.",
			report: Report{
				Hops: []Hop{
					{N: 1, URL: "https://example.com/1", Host: "example.com"},
					{N: 2, URL: "https://example.com/2", Host: "example.com"},
					{N: 3, URL: "https://example.com/3", Host: "example.com"},
					{N: 4, URL: "https://example.com/4", Host: "example.com"},
				},
			},
		},
		{
			id:       "urldefense-v3",
			severity: "info",
			text:     "This link is wrapped in Proofpoint's newer URL Defense format, which CHIT cannot unwrap. The chain below follows it, so the destination still shows up.",
			report: Report{
				Start: "https://urldefense.proofpoint.com/v3/__https://example.com/__;!!abc$",
			},
		},
		{
			id:       "mimecast",
			severity: "info",
			text:     "This link is wrapped by Mimecast, which hides the real address behind a code only Mimecast can look up. Following it is the only way to see where it goes.",
			report: Report{
				Start: "https://protect-eu.mimecast.com/s/abc123",
			},
		},
	}

	seen := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.id+"/"+tc.text[:20], func(t *testing.T) {
			report := tc.report
			got, ok := findingByID(findings(&report), tc.id)
			if !ok {
				t.Fatalf("%s did not fire", tc.id)
			}
			if got.Severity != tc.severity {
				t.Errorf("severity = %q, want %q", got.Severity, tc.severity)
			}
			if got.Text != tc.text {
				t.Errorf("text =\n%q\nwant\n%q", got.Text, tc.text)
			}
			seen[tc.id] = true
		})
	}

	want := []string{
		"private-target", "bad-scheme", "punycode", "mixed-script", "credentials", "executable",
		"ip-literal", "insecure", "downgrade", "cross-domain", "shortener", "free-hosting",
		"unusual-port", "new-domain", "redirect-loop", "hop-cap", "no-location",
		"long-chain", "urldefense-v3", "mimecast",
	}
	for _, id := range want {
		if !seen[id] {
			t.Errorf("finding %q has no case", id)
		}
	}
}

func TestFindingsOrder(t *testing.T) {
	report := Report{
		// info: four hops. warn: a bare IP and plain http. danger: punycode.
		Final:     "http://93.184.216.34/",
		FinalHost: HostName{Raw: "xn--80ak6aa92e.com", Decoded: "аррӏе.com", Punycode: true, IsIP: true},
		Hops: []Hop{
			{N: 1, URL: "http://a.example/1", Host: "a.example"},
			{N: 2, URL: "http://a.example/2", Host: "a.example"},
			{N: 3, URL: "http://a.example/3", Host: "a.example"},
			{N: 4, URL: "http://a.example/4", Host: "a.example"},
		},
	}

	got := []string{}
	for _, f := range findings(&report) {
		got = append(got, f.ID)
	}
	want := []string{"punycode", "ip-literal", "insecure", "long-chain"}
	if len(got) != len(want) {
		t.Fatalf("findings = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("findings = %v, want %v", got, want)
		}
	}
}

func TestFindingsLevel(t *testing.T) {
	cases := []struct {
		name string
		list []Finding
		want string
	}{
		{"danger wins", []Finding{{Severity: "info"}, {Severity: "warn"}, {Severity: "danger"}}, "danger"},
		{"warn only", []Finding{{Severity: "info"}, {Severity: "warn"}}, "warn"},
		{"info only", []Finding{{Severity: "info"}}, "ok"},
		{"nothing", []Finding{}, "ok"},
	}
	for _, tc := range cases {
		if got := levelOf(tc.list); got != tc.want {
			t.Errorf("%s: levelOf = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A chain where nothing ever answered used to come back level "ok" with
// "nothing about it looks wrong", while its only hop said the name could not be
// looked up. The two sentences contradicted each other, and "ok" is the one
// verdict this tool must never give for a link it did not manage to check.
func TestLevelIsUnknownWhenNothingAnswered(t *testing.T) {
	cases := []struct {
		name string
		hops []Hop
		list []Finding
		want string
	}{
		{"no hop answered", []Hop{{N: 1, Error: "That name could not be looked up."}}, nil, "unknown"},
		{"no hops at all", []Hop{}, nil, "unknown"},
		{"every hop failed", []Hop{{N: 1, Error: "a"}, {N: 2, Error: "b"}}, nil, "unknown"},
		{"one hop answered", []Hop{{N: 1, Error: "a"}, {N: 2, Status: 200}}, nil, "ok"},
		{"answered with a redirect", []Hop{{N: 1, Status: 301}}, nil, "ok"},
		// A real finding still outranks it: unknown means "not checked", and a
		// homograph in the address the user typed is checkable without a reply.
		{"danger beats unknown", []Hop{{N: 1, Error: "a"}}, []Finding{{Severity: "danger"}}, "danger"},
		{"warn beats unknown", []Hop{{N: 1, Error: "a"}}, []Finding{{Severity: "warn"}}, "warn"},
		{"info does not", []Hop{{N: 1, Error: "a"}}, []Finding{{Severity: "info"}}, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := Report{Hops: tc.hops, Findings: tc.list}
			if got := levelFor(&report); got != tc.want {
				t.Errorf("levelFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAgeUnknownIsNotAFinding(t *testing.T) {
	report := Report{
		Final:     "https://example.com/",
		StartHost: HostName{Raw: "example.com", Decoded: "example.com", Registrable: "example.com"},
		FinalHost: HostName{Raw: "example.com", Decoded: "example.com", Registrable: "example.com"},
		Hops:      []Hop{{N: 1, URL: "https://example.com/", Host: "example.com", Status: 200}},
		Age:       Age{Known: false, Days: 3, Note: ageUnknown},
	}

	list := findings(&report)
	if _, ok := findingByID(list, "new-domain"); ok {
		t.Error("an unknown age produced a new-domain finding")
	}
	if got := levelOf(list); got != "ok" {
		t.Errorf("level = %q, want ok (findings: %+v)", got, list)
	}
}

func TestNewDomainThreshold(t *testing.T) {
	cases := []struct {
		days int
		want bool
	}{{0, true}, {89, true}, {90, false}, {91, false}}

	for _, tc := range cases {
		report := Report{
			FinalHost: HostName{Raw: "example.com", Registrable: "example.com"},
			Age:       Age{Known: true, Registered: "2026-05-01", Days: tc.days, Human: humanAge(tc.days)},
		}
		_, got := findingByID(findings(&report), "new-domain")
		if got != tc.want {
			t.Errorf("%d days: new-domain fired = %v, want %v", tc.days, got, tc.want)
		}
	}
}

func TestMixedScriptLabel(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"apple.com", ""},
		{"pаypal.com", "pаypal"},
		{"中文.com", ""},
		{"ελληνικά.gr", ""},
		{"paypaλ.com", "paypaλ"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := mixedScriptLabel(tc.host); got != tc.want {
			t.Errorf("mixedScriptLabel(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

func TestHeadline(t *testing.T) {
	cases := []struct {
		name   string
		report Report
		want   string
	}{
		{
			name: "danger",
			report: Report{
				FinalHost: HostName{Decoded: "аррӏе.com"},
				Findings:  []Finding{{Severity: "danger"}, {Severity: "warn"}},
			},
			want: "This link ends at аррӏе.com, and there is something seriously wrong with it. Read the list below before anyone clicks it.",
		},
		{
			name: "one warning",
			report: Report{
				FinalHost: HostName{Decoded: "example.com"},
				Findings:  []Finding{{Severity: "warn"}, {Severity: "info"}},
			},
			want: "This link ends at example.com. There is 1 thing worth checking below before you trust it.",
		},
		{
			name: "two warnings",
			report: Report{
				FinalHost: HostName{Decoded: "example.com"},
				Findings:  []Finding{{Severity: "warn"}, {Severity: "warn"}},
			},
			want: "This link ends at example.com. There are 2 things worth checking below before you trust it.",
		},
		{
			// Status is set on every hop below because a hop that answered is
			// what "nothing looks wrong" means. A hop left at Status 0 is one
			// that never answered, and the fixtures said the opposite of what
			// they were named.
			name: "straight there",
			report: Report{
				FinalHost: HostName{Decoded: "example.com"},
				Hops:      []Hop{{N: 1, Status: 200}},
				Findings:  []Finding{},
			},
			want: "This link goes straight to example.com and nothing about it looks wrong. That is not the same as safe: check the address is one you were expecting.",
		},
		{
			name: "three redirects",
			report: Report{
				FinalHost: HostName{Decoded: "example.com"},
				Hops:      []Hop{{N: 1, Status: 301}, {N: 2, Status: 301}, {N: 3, Status: 302}, {N: 4, Status: 200}},
				Findings:  []Finding{{Severity: "info"}},
			},
			want: "This link goes to example.com through 3 redirects and nothing about it looks wrong. That is not the same as safe: check the address is one you were expecting.",
		},
		{
			name: "nothing answered",
			report: Report{
				FinalHost: HostName{Decoded: "no-such-domain.invalid"},
				Hops:      []Hop{{N: 1, Error: "That name could not be looked up."}},
				Findings:  []Finding{},
			},
			want: "CHIT could not reach no-such-domain.invalid, so there is nothing to judge. That is not the same as safe: the address may be wrong, the site may be switched off, or your network may be blocking it.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := tc.report
			if got := headline(&report); got != tc.want {
				t.Errorf("headline =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestHumanAge(t *testing.T) {
	cases := []struct {
		days int
		want string
	}{
		{0, "less than a day"},
		{1, "1 day"},
		{2, "2 days"},
		{29, "29 days"},
		{30, "30 days"},
		{31, "1 month"},
		{60, "2 months"},
		{364, "12 months"},
		{365, "1 year"},
		{366, "1 year"},
		{730, "2 years"},
		{800, "2 years"},
	}
	for _, tc := range cases {
		if got := humanAge(tc.days); got != tc.want {
			t.Errorf("humanAge(%d) = %q, want %q", tc.days, got, tc.want)
		}
	}
}

func TestExecutableExtensions(t *testing.T) {
	for _, ext := range executableExts {
		final := "https://example.com/invoice" + ext
		if got := executableName(final); got != "invoice"+ext {
			t.Errorf("executableName(%q) = %q, want %q", final, got, "invoice"+ext)
		}
	}

	for _, ext := range []string{".zip", ".js", ".pdf"} {
		final := "https://example.com/invoice" + ext
		if got := executableName(final); got != "" {
			t.Errorf("executableName(%q) = %q, want it ignored", final, got)
		}
	}

	cases := []struct {
		final string
		want  string
	}{
		{"https://example.com/download.exe?x=1", "download.exe"},
		{"https://example.com/DOWNLOAD.EXE", "DOWNLOAD.EXE"},
		{"https://example.com/", ""},
		{"https://example.com/exe", ""},
	}
	for _, tc := range cases {
		if got := executableName(tc.final); got != tc.want {
			t.Errorf("executableName(%q) = %q, want %q", tc.final, got, tc.want)
		}
	}
}

// TestPunycodeNamesOneHost keeps the finding to a single sentence when both ends
// of the chain are written in xn-- form, and pins which of the two it names.
func TestPunycodeNamesOneHost(t *testing.T) {
	report := Report{
		StartHost: HostName{Raw: "xn--n3h.net", Decoded: "☃.net", Punycode: true},
		FinalHost: HostName{Raw: "xn--80ak6aa92e.com", Decoded: "аррӏе.com", Punycode: true},
	}

	list := findings(&report)
	count := 0
	for _, f := range list {
		if f.ID == "punycode" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d punycode findings, want 1", count)
	}

	got, _ := findingByID(list, "punycode")
	want := "The address \"xn--80ak6aa92e.com\" is really \"аррӏе.com\". Those are not the ordinary Latin letters, which is how a fake address is made to look like a real one."
	if got.Text != want {
		t.Errorf("text =\n%q\nwant\n%q", got.Text, want)
	}
}
