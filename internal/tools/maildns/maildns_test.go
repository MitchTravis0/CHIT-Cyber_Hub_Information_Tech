package maildns

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestParseDKIM(t *testing.T) {
	tests := []struct {
		name        string
		txts        []string
		wantOK      bool
		wantHasKey  bool
		wantKeyType string
	}{
		{"a published rsa key", []string{"v=DKIM1; k=rsa; p=MIIBIjANBgkq"}, true, true, "rsa"},
		{"no k tag defaults to rsa", []string{"v=DKIM1; p=MIIBIjANBgkq"}, true, true, "rsa"},
		{"ed25519", []string{"v=DKIM1; k=ed25519; p=abc"}, true, true, "ed25519"},
		{"revoked, empty p", []string{"v=DKIM1; p="}, true, false, "rsa"},
		{"revoked with other tags", []string{"v=DKIM1; k=rsa; t=y; p="}, true, false, "rsa"},
		{"mixed case tag", []string{"V=DKIM1; K=RSA; P=abc"}, true, true, "rsa"},
		{"no p tag at all", []string{"v=DKIM1; k=rsa"}, true, false, "rsa"},
		{"not a DKIM record", []string{"some other text"}, false, false, ""},
		{"the tag mid string", []string{"note v=DKIM1; p=abc"}, false, false, ""},
		{"tag glued to more text", []string{"v=DKIM11; p=abc"}, false, false, ""},
		{"nothing", nil, false, false, ""},
		{"a DKIM record among others", []string{"unrelated", "v=DKIM1; p=abc"}, true, true, "rsa"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseDKIM("selector1", tt.txts)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Selector != "selector1" {
				t.Errorf("selector = %q, want selector1", got.Selector)
			}
			if got.HasKey != tt.wantHasKey {
				t.Errorf("hasKey = %v, want %v", got.HasKey, tt.wantHasKey)
			}
			if got.KeyType != tt.wantKeyType {
				t.Errorf("keyType = %q, want %q", got.KeyType, tt.wantKeyType)
			}
			if got.Record == "" {
				t.Error("record is blank")
			}
		})
	}
}

func TestNormalizeDomain(t *testing.T) {
	long := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." +
		strings.Repeat("c", 63) + "." + strings.Repeat("d", 61) // 253 characters

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{"plain", "example.com", "example.com", ""},
		{"upper case", " Example.COM ", "example.com", ""},
		{"an email address", "someone@example.com", "example.com", ""},
		{"an email address with a plus", "some.one+tag@mail.example.co.uk", "mail.example.co.uk", ""},
		{"a url", "https://example.com/path?q=1", "example.com", ""},
		{"a url with no path", "http://example.com", "example.com", ""},
		{"a trailing dot", "example.com.", "example.com", ""},
		{"a subdomain", "mail.example.com", "mail.example.com", ""},
		{"253 characters", long, long, ""},
		{"254 characters", long + "e", "", "is not a domain name"},
		{"a label of 64", strings.Repeat("a", 64) + ".com", "", "is not a domain name"},
		{"a leading hyphen", "-bad.example.com", "", "is not a domain name"},
		{"a trailing hyphen", "bad-.example.com", "", "is not a domain name"},
		{"an underscore", "_dmarc.example.com", "", "is not a domain name"},
		{"a space", "exa mple.com", "", "is not a domain name"},
		{"empty", "", "", "Type a domain to check"},
		{"whitespace only", "   ", "", "Type a domain to check"},
		{"a single label", "localhost", "", "not a full domain name"},
		{"an IPv4 address", "192.168.1.1", "", "is an address, not a domain"},
		{"an IPv6 address", "2606:4700::1111", "", "is an address, not a domain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeDomain(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("NormalizeDomain(%q) = %q, want an error", tt.in, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeDomain(%q) errored: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeDomain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeParams(t *testing.T) {
	tests := []struct {
		name          string
		in            Params
		wantSelectors int
		wantTimeout   int
		wantErr       string
	}{
		{"defaults", Params{Domain: "example.com"}, 14, 5000, ""},
		{"an extra selector", Params{Domain: "example.com", Selector: "mysel"}, 15, 5000, ""},
		{"an extra selector already in the list", Params{Domain: "example.com", Selector: "google"}, 14, 5000, ""},
		{"whitespace selector is ignored", Params{Domain: "example.com", Selector: "  "}, 14, 5000, ""},
		{"a selector of 63 is accepted", Params{Domain: "example.com", Selector: strings.Repeat("s", 63)}, 15, 5000, ""},
		{"a selector of 64 is rejected", Params{Domain: "example.com", Selector: strings.Repeat("s", 64)}, 0, 0, "at most 63 characters"},
		{"a named server is rejected", Params{Domain: "example.com", Server: "dc01"}, 0, 0, "is not one"},
		{"an IP server is accepted", Params{Domain: "example.com", Server: "8.8.8.8"}, 14, 5000, ""},
		{"timeout 0 defaults", Params{Domain: "example.com", TimeoutMS: 0}, 14, 5000, ""},
		{"timeout 499 rejected", Params{Domain: "example.com", TimeoutMS: 499}, 0, 0, "between 0.5 and 20 seconds"},
		{"timeout 500 accepted", Params{Domain: "example.com", TimeoutMS: 500}, 14, 500, ""},
		{"timeout 20000 accepted", Params{Domain: "example.com", TimeoutMS: 20000}, 14, 20000, ""},
		{"timeout 20001 rejected", Params{Domain: "example.com", TimeoutMS: 20001}, 0, 0, "between 0.5 and 20 seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.normalize()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize errored: %v", err)
			}
			if len(got.selectors) != tt.wantSelectors {
				t.Errorf("selectors = %d, want %d", len(got.selectors), tt.wantSelectors)
			}
			if got.timeoutMS != tt.wantTimeout {
				t.Errorf("timeout = %d, want %d", got.timeoutMS, tt.wantTimeout)
			}
		})
	}
}

func TestSortKeys(t *testing.T) {
	order := []string{"default", "google", "selector1", "selector2"}
	keys := []DKIMKey{{Selector: "selector2"}, {Selector: "default"}, {Selector: "google"}}
	sortKeys(keys, order)
	want := []string{"default", "google", "selector2"}
	for i, k := range keys {
		if k.Selector != want[i] {
			t.Errorf("key %d is %q, want %q", i, k.Selector, want[i])
		}
	}
}

func TestServerLabel(t *testing.T) {
	if got := serverLabel(""); got != SystemResolverLabel {
		t.Errorf("serverLabel(\"\") = %q, want %q", got, SystemResolverLabel)
	}
	if got := serverLabel("8.8.8.8"); got != "8.8.8.8" {
		t.Errorf("serverLabel(\"8.8.8.8\") = %q", got)
	}
}

func TestCheckAgainstLocalResolver(t *testing.T) {
	f := startFakeDNS(t, zone{
		"example.com": {
			typeMX:  {mxRecord(10, "mail.example.com."), mxRecord(20, "backup.example.com.")},
			typeTXT: {txtRecord("v=spf1 include:_spf.example.com -all"), txtRecord("google-site-verification=abc")},
		},
		"_dmarc.example.com": {
			typeTXT: {txtRecord("v=DMARC1; p=reject; rua=mailto:dmarc@example.com")},
		},
		"selector1._domainkey.example.com": {
			typeTXT: {txtRecord("v=DKIM1; k=rsa; p=MIIBIjANBgkq")},
		},
	})

	got, err := Check(context.Background(), Params{
		Domain: "example.com", Server: f.addr(), TimeoutMS: 3000,
	})
	if err != nil {
		t.Fatalf("Check errored: %v", err)
	}

	wantMX := []MXHost{{Host: "mail.example.com", Preference: 10}, {Host: "backup.example.com", Preference: 20}}
	if !reflect.DeepEqual(got.MX, wantMX) {
		t.Errorf("mx = %+v, want %+v", got.MX, wantMX)
	}
	if got.NullMX {
		t.Error("nullMx = true for a domain with real mail servers")
	}
	if !got.SPF.Found || got.SPF.All != "-" || got.SPF.Lookups != 1 {
		t.Errorf("spf = %+v", got.SPF)
	}
	if !got.DMARC.Found || got.DMARC.Policy != "reject" {
		t.Errorf("dmarc = %+v", got.DMARC)
	}
	if len(got.DKIM) != 1 || got.DKIM[0].Selector != "selector1" || !got.DKIM[0].HasKey {
		t.Errorf("dkim = %+v", got.DKIM)
	}
	if got.Level != LevelOK {
		t.Errorf("level = %q, want %q. Findings: %+v", got.Level, LevelOK, got.Findings)
	}
	if got.Server != f.addr() {
		t.Errorf("server = %q, want %q", got.Server, f.addr())
	}
	if got.CheckedAt == "" {
		t.Error("checkedAt is blank")
	}
	if len(got.SelectorsTried) != 14 {
		t.Errorf("selectorsTried = %d, want 14", len(got.SelectorsTried))
	}
}

func TestCheckReportsANullMX(t *testing.T) {
	f := startFakeDNS(t, zone{
		"example.com": {typeMX: {nullMXRecord()}},
	})

	got, err := Check(context.Background(), Params{Domain: "example.com", Server: f.addr(), TimeoutMS: 3000})
	if err != nil {
		t.Fatalf("Check errored: %v", err)
	}
	if !got.NullMX {
		t.Fatal("nullMx = false for a domain publishing the RFC 7505 empty MX")
	}
	if len(got.MX) != 0 {
		t.Errorf("mx = %+v, want none: the root is not a mail server", got.MX)
	}
	if !hasFinding(got, LevelOK, AreaMX, "Accepts no email, deliberately", "standard way") {
		t.Errorf("no null MX finding. Findings: %+v", got.Findings)
	}
}

func TestCheckFindsACustomSelector(t *testing.T) {
	f := startFakeDNS(t, zone{
		"example.com": {typeTXT: {txtRecord("v=spf1 -all")}},
		"weird-name._domainkey.example.com": {
			typeTXT: {txtRecord("v=DKIM1; p=abc")},
		},
	})

	got, err := Check(context.Background(), Params{
		Domain: "example.com", Selector: "weird-name", Server: f.addr(), TimeoutMS: 3000,
	})
	if err != nil {
		t.Fatalf("Check errored: %v", err)
	}
	if len(got.DKIM) != 1 || got.DKIM[0].Selector != "weird-name" {
		t.Fatalf("dkim = %+v, want the custom selector", got.DKIM)
	}
	if len(got.SelectorsTried) != 15 {
		t.Errorf("selectorsTried = %d, want 15", len(got.SelectorsTried))
	}
}

func TestCheckSlicesAreNeverNil(t *testing.T) {
	// A domain with nothing at all is the input that leaves every slice empty.
	f := startFakeDNS(t, zone{})

	got, err := Check(context.Background(), Params{Domain: "example.com", Server: f.addr(), TimeoutMS: 3000})
	if err != nil {
		t.Fatalf("Check errored: %v", err)
	}
	if got.MX == nil || got.DKIM == nil || got.Findings == nil ||
		got.SelectorsTried == nil || got.SPF.Terms == nil ||
		got.DMARC.RUA == nil || got.DMARC.RUF == nil {
		t.Fatalf("a slice is nil and would marshal to JSON null: %+v", got)
	}
}

func TestCheckRejectsBadInputBeforeAsking(t *testing.T) {
	f := startFakeDNS(t, zone{})

	if _, err := Check(context.Background(), Params{Domain: "localhost", Server: f.addr()}); err == nil {
		t.Fatal("a single label was accepted as a domain")
	}
	if f.count() != 0 {
		t.Errorf("the fake was asked %d times: validation must happen before any query", f.count())
	}
}

func TestCheckTimesOutWithAReadableMessage(t *testing.T) {
	f := startSilentDNS(t)

	_, err := Check(context.Background(), Params{Domain: "example.com", Server: f.addr(), TimeoutMS: 600})
	if err == nil {
		t.Fatal("a silent DNS server produced no error")
	}
	if !strings.Contains(err.Error(), "did not answer within 600 ms") ||
		!strings.Contains(err.Error(), "port 53") {
		t.Errorf("error = %q", err)
	}
}
