package tlsprobe

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantHost string
		wantPort int
		wantErr  string
	}{
		{"bare name", "example.com", "example.com", 443, ""},
		{"name with port", "example.com:8443", "example.com", 8443, ""},
		{"https url", "https://example.com", "example.com", 443, ""},
		{"https url with port and path", "https://example.com:9443/path?q=1", "example.com", 9443, ""},
		{"http url", "http://example.com", "example.com", 443, ""},
		{"http url with port", "http://example.com:8080", "example.com", 8080, ""},
		{"ipv4", "192.168.1.10", "192.168.1.10", 443, ""},
		{"ipv4 with port", "192.168.1.10:993", "192.168.1.10", 993, ""},
		{"bracketed ipv6 with port", "[2606:4700::1111]:443", "2606:4700::1111", 443, ""},
		{"bare ipv6", "2606:4700::1111", "2606:4700::1111", 443, ""},
		{"surrounding whitespace", "  example.com  ", "example.com", 443, ""},
		{"highest port", "example.com:65535", "example.com", 65535, ""},
		{"lowest port", "example.com:1", "example.com", 1, ""},
		{"ftp scheme rejected", "ftp://example.com", "", 0, "only probe a plain TLS service"},
		{"ldaps scheme rejected", "ldaps://example.com", "", 0, "only probe a plain TLS service"},
		{"port zero rejected", "example.com:0", "", 0, "port between 1 and 65535"},
		{"port too high rejected", "example.com:65536", "", 0, "port between 1 and 65535"},
		{"non numeric port rejected", "example.com:https", "", 0, "port between 1 and 65535"},
		{"empty rejected", "", "", 0, "Type the server to probe"},
		{"whitespace rejected", "   ", "", 0, "Type the server to probe"},
		{"embedded space rejected", "exa mple.com", "", 0, "is not a server name"},
		{"no host before the port rejected", ":443", "", 0, "is not a server name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTarget(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseTarget(%q) = %+v, want an error", tt.in, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTarget(%q) errored: %v", tt.in, err)
			}
			if got.host != tt.wantHost || got.port != tt.wantPort {
				t.Errorf("parseTarget(%q) = %s:%d, want %s:%d", tt.in, got.host, got.port, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestNormalizeTimeout(t *testing.T) {
	tests := []struct {
		in      int
		want    int
		wantErr bool
	}{
		{0, 5000, false},
		{499, 0, true},
		{500, 500, false},
		{30000, 30000, false},
		{30001, 0, true},
		{-1, 0, true},
	}
	for _, tt := range tests {
		got, err := normalizeTimeout(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("normalizeTimeout(%d) = %d, want an error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeTimeout(%d) errored: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("normalizeTimeout(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// TestVersionList pins the rows and their order. The names are written as
// literals: they are what the user reads, and reordering them would silently
// change every headline, which names accepted[0].
func TestVersionList(t *testing.T) {
	want := []string{"SSL 3.0", "TLS 1.0", "TLS 1.1", "TLS 1.2", "TLS 1.3"}
	got := versions()
	if len(got) != 5 {
		t.Fatalf("got %d versions, want 5", len(got))
	}
	notTestable := 0
	for i, v := range got {
		if v.name != want[i] {
			t.Errorf("version %d is %q, want %q", i, v.name, want[i])
		}
		if v.id == 0 {
			notTestable++
		}
	}
	if notTestable != 1 {
		t.Errorf("%d versions are not testable, want exactly 1 (SSL 3.0)", notTestable)
	}
	if got[0].id != 0 {
		t.Error("SSL 3.0 must be the untestable one")
	}
}

// TestAllCipherSuitesIncludesInsecure is the input that reaches the branch: a
// server offering only an old suite must still read as accepting the version.
func TestAllCipherSuitesIncludesInsecure(t *testing.T) {
	all := allCipherSuites()
	if len(all) <= len(tls.CipherSuites()) {
		t.Fatalf("got %d suites, want more than the %d secure ones", len(all), len(tls.CipherSuites()))
	}
	found := false
	for _, id := range all {
		if id == tls.TLS_RSA_WITH_AES_128_CBC_SHA {
			found = true
		}
	}
	if !found {
		t.Error("TLS_RSA_WITH_AES_128_CBC_SHA is missing, so an old server would read as refusing a version it accepts")
	}
}

func attemptsByVersion(r Report) map[string]Attempt {
	out := make(map[string]Attempt, len(r.Attempts))
	for _, a := range r.Attempts {
		out[a.Version] = a
	}
	return out
}

func TestProbeAgainstLegacyServer(t *testing.T) {
	addr := tlsServer(t, func(c *tls.Config) {
		c.MinVersion = tls.VersionTLS10
		c.MaxVersion = tls.VersionTLS12
	})

	report, err := Probe(context.Background(), Params{Target: addr, TimeoutMS: 3000})
	if err != nil {
		t.Fatalf("Probe errored: %v", err)
	}
	if len(report.Attempts) != 5 {
		t.Fatalf("got %d attempts, want 5", len(report.Attempts))
	}
	by := attemptsByVersion(report)

	for _, name := range []string{"TLS 1.0", "TLS 1.1", "TLS 1.2"} {
		a := by[name]
		if !a.Accepted {
			t.Errorf("%s was refused (%s), want accepted", name, a.Message)
		}
		if a.Cipher == "" {
			t.Errorf("%s was accepted with no cipher name", name)
		}
		if a.HandshakeMS <= 0 {
			t.Errorf("%s took %v ms, want a positive number", name, a.HandshakeMS)
		}
	}
	if by["TLS 1.3"].Accepted {
		t.Error("TLS 1.3 was accepted by a server capped at 1.2")
	}
	if ssl := by["SSL 3.0"]; ssl.Testable || ssl.Accepted || ssl.HandshakeMS != 0 {
		t.Errorf("SSL 3.0 = %+v, want not testable, not accepted, no timing", ssl)
	}
	if !strings.Contains(by["SSL 3.0"].Message, "Go's TLS library does not implement it") {
		t.Errorf("SSL 3.0 message = %q", by["SSL 3.0"].Message)
	}
	if report.Level != LevelWarn {
		t.Errorf("level = %q, want %q (old and modern together)", report.Level, LevelWarn)
	}
	if report.IP == "" {
		t.Error("IP is blank")
	}
}

func TestProbeAgainstModernServer(t *testing.T) {
	addr := tlsServer(t, func(c *tls.Config) { c.MinVersion = tls.VersionTLS12 })

	report, err := Probe(context.Background(), Params{Target: addr, TimeoutMS: 3000})
	if err != nil {
		t.Fatalf("Probe errored: %v", err)
	}
	by := attemptsByVersion(report)

	for _, name := range []string{"TLS 1.0", "TLS 1.1"} {
		if by[name].Accepted {
			t.Errorf("%s was accepted by a server that requires 1.2", name)
		}
		if by[name].Message == "" {
			t.Errorf("%s carries no message", name)
		}
	}
	for _, name := range []string{"TLS 1.2", "TLS 1.3"} {
		if !by[name].Accepted {
			t.Errorf("%s was refused (%s), want accepted", name, by[name].Message)
		}
	}
	if report.Level != LevelOK {
		t.Errorf("level = %q, want %q", report.Level, LevelOK)
	}
	if !strings.Contains(report.Headline, "TLS 1.2 and TLS 1.3") {
		t.Errorf("headline = %q", report.Headline)
	}
}

func TestProbeNothingListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	report, err := Probe(context.Background(), Params{Target: addr, TimeoutMS: 1000})
	if err != nil {
		t.Fatalf("Probe errored: %v", err)
	}
	for _, a := range report.Attempts {
		if a.Accepted {
			t.Errorf("%s was accepted on a dead port", a.Version)
		}
	}
	if report.Level != LevelError {
		t.Errorf("level = %q, want %q", report.Level, LevelError)
	}
	if !strings.Contains(report.Headline, "so no TLS version could be tested") {
		t.Errorf("headline = %q", report.Headline)
	}
	if !strings.Contains(report.Advice, "This is a connection problem, not a TLS one.") {
		t.Errorf("advice = %q", report.Advice)
	}
}

// TestProbeIgnoresBadCertificate is the input that proves InsecureSkipVerify is
// doing its job: httptest's certificate is self-signed and untrusted, and the
// versions must still be reported.
func TestProbeIgnoresBadCertificate(t *testing.T) {
	addr := tlsServer(t, func(c *tls.Config) { c.MinVersion = tls.VersionTLS12 })

	report, err := Probe(context.Background(), Params{Target: addr})
	if err != nil {
		t.Fatalf("Probe errored: %v", err)
	}
	if !attemptsByVersion(report)["TLS 1.2"].Accepted {
		t.Fatal("a self-signed certificate stopped TLS 1.2 being reported, which defeats the whole tool")
	}
}

// TestInsecureCipherOnlyServer is the input that reaches the insecure-suite
// branch: a server offering nothing but an old CBC suite on TLS 1.0.
func TestInsecureCipherOnlyServer(t *testing.T) {
	// An RSA key, because TLS_RSA_WITH_AES_128_CBC_SHA needs one. That suite is
	// in Go's InsecureCipherSuites list, so a client using Go's defaults would
	// read this server as refusing TLS 1.0 when it does not.
	addr := tlsServerWithKey(t, true, func(c *tls.Config) {
		c.MinVersion = tls.VersionTLS10
		c.MaxVersion = tls.VersionTLS10
		c.CipherSuites = []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA}
	})

	report, err := Probe(context.Background(), Params{Target: addr, TimeoutMS: 3000})
	if err != nil {
		t.Fatalf("Probe errored: %v", err)
	}
	a := attemptsByVersion(report)["TLS 1.0"]
	if !a.Accepted {
		t.Fatalf("TLS 1.0 read as refused (%s), but the server accepts it with an old cipher", a.Message)
	}
	if a.Cipher != "TLS_RSA_WITH_AES_128_CBC_SHA" {
		t.Errorf("cipher = %q, want TLS_RSA_WITH_AES_128_CBC_SHA", a.Cipher)
	}
	if report.Level != LevelError {
		t.Errorf("level = %q, want %q for a 1.0-only server", report.Level, LevelError)
	}
	if !strings.Contains(report.Headline, "accepts only TLS 1.0") {
		t.Errorf("headline = %q", report.Headline)
	}
}

func TestALPNReported(t *testing.T) {
	withALPN := tlsServer(t, func(c *tls.Config) {
		c.MinVersion = tls.VersionTLS12
		c.NextProtos = []string{"h2"}
	})
	report, err := Probe(context.Background(), Params{Target: withALPN})
	if err != nil {
		t.Fatalf("Probe errored: %v", err)
	}
	if got := attemptsByVersion(report)["TLS 1.2"].ALPN; got != "h2" {
		t.Errorf("alpn = %q, want h2", got)
	}

	noALPN := tlsServer(t, func(c *tls.Config) {
		c.MinVersion = tls.VersionTLS12
		c.NextProtos = nil
	})
	report, err = Probe(context.Background(), Params{Target: noALPN})
	if err != nil {
		t.Fatalf("Probe errored: %v", err)
	}
	if got := attemptsByVersion(report)["TLS 1.2"].ALPN; got != "" {
		t.Errorf("alpn = %q, want empty when the server agrees to none", got)
	}
}

func TestProbeRejectsANameThatDoesNotResolve(t *testing.T) {
	_, err := Probe(context.Background(), Params{Target: "nosuchhost.invalid"})
	if err == nil {
		t.Fatal("want an error for a name that does not resolve")
	}
	if !strings.Contains(err.Error(), "Could not find a computer called nosuchhost.invalid") {
		t.Errorf("error = %q", err)
	}
}

func TestClassify(t *testing.T) {
	accepted := func(names ...string) []Attempt {
		out := make([]Attempt, 0, 5)
		for _, v := range versions() {
			a := Attempt{Version: v.name, Testable: v.id != 0}
			for _, n := range names {
				if n == v.name {
					a.Accepted = true
				}
			}
			out = append(out, a)
		}
		return out
	}

	tests := []struct {
		name         string
		in           []Attempt
		wantLevel    string
		wantHeadline string
	}{
		{"modern only", accepted("TLS 1.2", "TLS 1.3"), LevelOK, "accepts TLS 1.2 and TLS 1.3"},
		{"1.3 only", accepted("TLS 1.3"), LevelOK, "accepts TLS 1.3"},
		{"1.2 only", accepted("TLS 1.2"), LevelOK, "accepts TLS 1.2"},
		{"old and modern", accepted("TLS 1.0", "TLS 1.2"), LevelWarn, "still accepts TLS 1.0"},
		{"1.1 and modern", accepted("TLS 1.1", "TLS 1.3"), LevelWarn, "still accepts TLS 1.1"},
		{"1.0 only", accepted("TLS 1.0"), LevelError, "accepts only TLS 1.0"},
		{"1.1 only", accepted("TLS 1.1"), LevelError, "accepts only TLS 1.1"},
		{"1.0 and 1.1 only", accepted("TLS 1.0", "TLS 1.1"), LevelError, "accepts only TLS 1.0 and TLS 1.1"},
		{"nothing", accepted(), LevelError, "so no TLS version could be tested"},
		{"all four", accepted("TLS 1.0", "TLS 1.1", "TLS 1.2", "TLS 1.3"), LevelWarn, "still accepts TLS 1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, headline, advice := Classify("mail.example.com", 443, tt.in)
			if level != tt.wantLevel {
				t.Errorf("level = %q, want %q (headline %q)", level, tt.wantLevel, headline)
			}
			if !strings.Contains(headline, tt.wantHeadline) {
				t.Errorf("headline %q does not contain %q", headline, tt.wantHeadline)
			}
			if strings.TrimSpace(advice) == "" && level != LevelOK {
				t.Errorf("level %q carries no advice", level)
			}
		})
	}
}

func TestClassifyNamesTheHostAndPort(t *testing.T) {
	_, headline, _ := Classify("mail.example.com", 8443, []Attempt{{Version: "TLS 1.2"}})
	if !strings.Contains(headline, "mail.example.com") || !strings.Contains(headline, "8443") {
		t.Errorf("headline %q does not name the host and port that failed", headline)
	}
}

func TestJoinVersions(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"A"}, "A"},
		{[]string{"A", "B"}, "A and B"},
		{[]string{"A", "B", "C"}, "A, B and C"},
		{[]string{"A", "B", "C", "D"}, "A, B, C and D"},
	}
	for _, tt := range tests {
		if got := joinVersions(tt.in); got != tt.want {
			t.Errorf("joinVersions(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFailureMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"timeout", context.DeadlineExceeded, "got no answer within 1500 ms"},
		{"reset", errText("read tcp 1.2.3.4:1 -> 5.6.7.8:443: connection reset by peer"), "closed the connection during"},
		{"anything else", errText("tls: protocol version not supported"), "The server refused TLS 1.0."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := failureMessage("TLS 1.0", tt.err, 1500)
			if !strings.Contains(got, tt.want) {
				t.Errorf("message %q does not contain %q", got, tt.want)
			}
		})
	}
}

type errText string

func (e errText) Error() string { return string(e) }

// TestEveryAttemptHasAMessage is the "never ship a row with nothing in it" rule.
func TestEveryAttemptHasAMessage(t *testing.T) {
	addr := tlsServer(t, func(c *tls.Config) { c.MinVersion = tls.VersionTLS12 })
	report, err := Probe(context.Background(), Params{Target: addr})
	if err != nil {
		t.Fatalf("Probe errored: %v", err)
	}
	for _, a := range report.Attempts {
		if strings.TrimSpace(a.Message) == "" {
			t.Errorf("%s carries no message", a.Version)
		}
		if strings.TrimSpace(a.Version) == "" {
			t.Error("an attempt has no version name")
		}
	}
}

// TestAttemptsNeverNil is the input that reaches the guard: a report built
// before any attempt ran.
func TestAttemptsNeverNil(t *testing.T) {
	r := Report{Attempts: []Attempt{}}
	if r.Attempts == nil {
		t.Fatal("Attempts is nil, which marshals to JSON null and breaks the table")
	}
}

func TestProbeHonoursCancellation(t *testing.T) {
	addr := tlsServer(t, func(c *tls.Config) { c.MinVersion = tls.VersionTLS12 })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Probe(ctx, Params{Target: addr, TimeoutMS: 3000}); err == nil {
		t.Fatal("a cancelled probe returned no error")
	}
	_ = time.Now
}
