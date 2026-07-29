package pubip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"chit/internal/core"
)

const ipinfoBody = `{
  "ip": "203.0.113.9",
  "hostname": "host9.example.net",
  "city": "Leeds",
  "region": "England",
  "country": "GB",
  "loc": "53.7960,-1.5478",
  "org": "AS12345 Example Communications Ltd",
  "postal": "LS1",
  "timezone": "Europe/London"
}`

const ipwhoBody = `{
  "ip": "203.0.113.9",
  "success": true,
  "city": "Leeds",
  "region": "England",
  "country": "United Kingdom",
  "country_code": "GB",
  "latitude": 53.796,
  "longitude": -1.5478,
  "connection": { "asn": 12345, "org": "Example Communications Ltd", "isp": "Example Communications Ltd" },
  "timezone": { "id": "Europe/London" }
}`

const traceBody = "fl=123abc\nh=www.cloudflare.com\nip=203.0.113.9\nts=1753440000.123\nloc=GB\n"

const partialNote = "Only the address could be looked up: the services that name the ISP and location did not answer. The address itself is correct."

func TestParseIPInfo(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    Info
		wantErr bool
	}{
		{
			name: "full body",
			body: ipinfoBody,
			want: Info{
				IPv4:        "203.0.113.9",
				ISP:         "Example Communications Ltd",
				ASN:         "AS12345",
				City:        "Leeds",
				Region:      "England",
				Country:     "GB",
				CountryName: "United Kingdom",
				Timezone:    "Europe/London",
				Latitude:    53.796,
				Longitude:   -1.5478,
			},
		},
		{
			name: "missing org",
			body: `{"ip":"203.0.113.9","country":"US"}`,
			want: Info{IPv4: "203.0.113.9", Country: "US", CountryName: "United States"},
		},
		{
			name: "org without an AS prefix",
			body: `{"ip":"203.0.113.9","org":"Example Communications Ltd"}`,
			want: Info{IPv4: "203.0.113.9", ISP: "Example Communications Ltd"},
		},
		{
			name: "org that is only an AS number",
			body: `{"ip":"203.0.113.9","org":"AS12345"}`,
			want: Info{IPv4: "203.0.113.9", ISP: "AS12345"},
		},
		{
			name: "loc with one field",
			body: `{"ip":"203.0.113.9","loc":"53.7960"}`,
			want: Info{IPv4: "203.0.113.9"},
		},
		{
			name: "loc with junk",
			body: `{"ip":"203.0.113.9","loc":"north,west"}`,
			want: Info{IPv4: "203.0.113.9"},
		},
		{
			name: "unknown country code falls back to the code",
			body: `{"ip":"203.0.113.9","country":"ZZ"}`,
			want: Info{IPv4: "203.0.113.9", Country: "ZZ", CountryName: "ZZ"},
		},
		{name: "bogon", body: `{"ip":"192.168.1.4","bogon":true}`, wantErr: true},
		{name: "empty body", body: "", wantErr: true},
		{name: "captive portal html", body: "<html><body>Sign in</body></html>", wantErr: true},
		{name: "address is not an ip", body: `{"ip":"not an address"}`, wantErr: true},
		{name: "no address at all", body: `{"city":"Leeds"}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIPInfo([]byte(tt.body))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseIPInfo(%q) = %+v, want an error", tt.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIPInfo(%q) returned %v", tt.body, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseIPInfo(%q)\n got %+v\nwant %+v", tt.body, got, tt.want)
			}
		})
	}
}

func TestParseIPWho(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    Info
		wantErr bool
	}{
		{
			name: "full body",
			body: ipwhoBody,
			want: Info{
				IPv4:        "203.0.113.9",
				ISP:         "Example Communications Ltd",
				ASN:         "AS12345",
				City:        "Leeds",
				Region:      "England",
				Country:     "GB",
				CountryName: "United Kingdom",
				Timezone:    "Europe/London",
				Latitude:    53.796,
				Longitude:   -1.5478,
			},
		},
		{
			name: "missing connection",
			body: `{"ip":"203.0.113.9","success":true,"country_code":"GB","country":"United Kingdom"}`,
			want: Info{IPv4: "203.0.113.9", Country: "GB", CountryName: "United Kingdom"},
		},
		{
			name: "zero asn",
			body: `{"ip":"203.0.113.9","success":true,"connection":{"asn":0,"isp":"Example Communications Ltd"}}`,
			want: Info{IPv4: "203.0.113.9", ISP: "Example Communications Ltd"},
		},
		{
			name: "isp falls back to org",
			body: `{"ip":"203.0.113.9","success":true,"connection":{"asn":12345,"org":"Example Communications Ltd"}}`,
			want: Info{IPv4: "203.0.113.9", ISP: "Example Communications Ltd", ASN: "AS12345"},
		},
		{name: "success false", body: `{"ip":"203.0.113.9","success":false}`, wantErr: true},
		{name: "empty body", body: "", wantErr: true},
		{name: "address is not an ip", body: `{"ip":"","success":true}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIPWho([]byte(tt.body))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseIPWho(%q) = %+v, want an error", tt.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIPWho(%q) returned %v", tt.body, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseIPWho(%q)\n got %+v\nwant %+v", tt.body, got, tt.want)
			}
		})
	}
}

func TestParseCloudflareTrace(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    Info
		wantErr bool
	}{
		{
			name: "full body",
			body: traceBody,
			want: Info{
				IPv4:        "203.0.113.9",
				Country:     "GB",
				CountryName: "United Kingdom",
				Partial:     true,
				Note:        partialNote,
			},
		},
		{
			name: "line with no equals sign is ignored",
			body: "warning\nip=203.0.113.9\nloc=ZZ\n",
			want: Info{
				IPv4:        "203.0.113.9",
				Country:     "ZZ",
				CountryName: "ZZ",
				Partial:     true,
				Note:        partialNote,
			},
		},
		{name: "no ip line", body: "fl=123abc\nloc=GB\n", wantErr: true},
		{name: "empty body", body: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCloudflareTrace([]byte(tt.body))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseCloudflareTrace(%q) = %+v, want an error", tt.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCloudflareTrace(%q) returned %v", tt.body, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCloudflareTrace(%q)\n got %+v\nwant %+v", tt.body, got, tt.want)
			}
		})
	}
}

func TestCountryName(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{"known code", "GB", "United Kingdom"},
		{"another known code", "JP", "Japan"},
		{"unknown code returns the code", "ZZ", "ZZ"},
		{"empty code", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countryName(tt.code); got != tt.want {
				t.Errorf("countryName(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

// testServer answers each path with the body registered for it, and 500 for a
// path with no body.
func testServer(t *testing.T, bodies map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLookupUsesFirstWorkingProvider(t *testing.T) {
	srv := testServer(t, map[string]string{
		"/second": "not json at all",
		"/third":  ipinfoBody,
	})
	c := &Client{
		HTTP: srv.Client(),
		Providers: []Provider{
			{Name: "first", URL: srv.URL + "/first", Parse: parseIPInfo},
			{Name: "second", URL: srv.URL + "/second", Parse: parseIPInfo},
			{Name: "third", URL: srv.URL + "/third", Parse: parseIPInfo},
		},
	}

	info, err := c.Lookup(context.Background())
	if err != nil {
		t.Fatalf("Lookup returned %v", err)
	}
	if info.Source != "third" {
		t.Errorf("Source = %q, want %q", info.Source, "third")
	}
	if info.IPv4 != "203.0.113.9" {
		t.Errorf("IPv4 = %q, want %q", info.IPv4, "203.0.113.9")
	}
	if info.ISP != "Example Communications Ltd" || info.ASN != "AS12345" {
		t.Errorf("ISP/ASN = %q / %q, want %q / %q",
			info.ISP, info.ASN, "Example Communications Ltd", "AS12345")
	}
	if info.City != "Leeds" || info.CountryName != "United Kingdom" {
		t.Errorf("City/CountryName = %q / %q, want %q / %q",
			info.City, info.CountryName, "Leeds", "United Kingdom")
	}
	if info.Partial {
		t.Error("Partial = true, want false for a full answer")
	}
	if info.Note != "" {
		t.Errorf("Note = %q, want it empty when the ISP is known", info.Note)
	}
}

func TestLookupAllProvidersFail(t *testing.T) {
	srv := testServer(t, nil)
	c := &Client{
		HTTP: srv.Client(),
		Providers: []Provider{
			{Name: "first", URL: srv.URL + "/first", Parse: parseIPInfo},
			{Name: "second", URL: srv.URL + "/second", Parse: parseIPWho},
			{Name: "third", URL: srv.URL + "/third", Parse: parseCloudflareTrace},
		},
	}

	_, err := c.Lookup(context.Background())
	if err == nil {
		t.Fatal("Lookup returned no error when every provider failed")
	}
	want := "Could not reach any of the public IP services. Check that this machine has internet access, and that a proxy or firewall is not blocking it."
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
	if code := core.CodeOf(err); code != core.CodeNetwork {
		t.Errorf("code = %q, want %q", code, core.CodeNetwork)
	}
}

func TestLookupPartial(t *testing.T) {
	srv := testServer(t, map[string]string{"/trace": traceBody})
	c := &Client{
		HTTP: srv.Client(),
		Providers: []Provider{
			{Name: "first", URL: srv.URL + "/first", Parse: parseIPInfo},
			{Name: "second", URL: srv.URL + "/second", Parse: parseIPWho},
			{Name: "cloudflare.com", URL: srv.URL + "/trace", Parse: parseCloudflareTrace},
		},
	}

	info, err := c.Lookup(context.Background())
	if err != nil {
		t.Fatalf("Lookup returned %v", err)
	}
	if !info.Partial {
		t.Error("Partial = false, want true when only the trace endpoint answered")
	}
	if info.Note != partialNote {
		t.Errorf("Note = %q, want %q", info.Note, partialNote)
	}
	if info.Source != "cloudflare.com" {
		t.Errorf("Source = %q, want %q", info.Source, "cloudflare.com")
	}
	if info.IPv4 != "203.0.113.9" {
		t.Errorf("IPv4 = %q, want %q", info.IPv4, "203.0.113.9")
	}
}

func TestLookupNoISP(t *testing.T) {
	srv := testServer(t, map[string]string{"/only": `{"ip":"203.0.113.9","city":"Leeds","country":"GB"}`})
	c := &Client{
		HTTP:      srv.Client(),
		Providers: []Provider{{Name: "only", URL: srv.URL + "/only", Parse: parseIPInfo}},
	}

	info, err := c.Lookup(context.Background())
	if err != nil {
		t.Fatalf("Lookup returned %v", err)
	}
	want := "The service knew the address but not the ISP behind it. That is normal on a business line whose range is registered to a reseller."
	if info.Note != want {
		t.Errorf("Note = %q, want %q", info.Note, want)
	}
}

func TestLookupHonoursContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	c := &Client{
		HTTP:      srv.Client(),
		Providers: []Provider{{Name: "slow", URL: srv.URL + "/slow", Parse: parseIPInfo}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	_, err := c.Lookup(ctx)
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("Lookup returned no error for a cancelled context")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Lookup took %s, want it to give up promptly", elapsed)
	}
	if code := core.CodeOf(err); code != core.CodeTimeout {
		t.Errorf("code = %q, want %q", code, core.CodeTimeout)
	}
}

func TestLookupSetsCheckedAt(t *testing.T) {
	srv := testServer(t, map[string]string{"/only": ipinfoBody})
	c := &Client{
		HTTP:      srv.Client(),
		Providers: []Provider{{Name: "only", URL: srv.URL + "/only", Parse: parseIPInfo}},
	}

	info, err := c.Lookup(context.Background())
	if err != nil {
		t.Fatalf("Lookup returned %v", err)
	}
	if _, err := time.Parse(time.RFC3339, info.CheckedAt); err != nil {
		t.Errorf("CheckedAt = %q, which does not parse as RFC3339: %v", info.CheckedAt, err)
	}
}
