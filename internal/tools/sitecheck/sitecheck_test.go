package sitecheck

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"chit/internal/core"
)

var (
	rsaOnce sync.Once
	rsaKey  *rsa.PrivateKey
)

// sharedRSAKey keeps the whole suite to one 2048 bit key generation.
func sharedRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	rsaOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generating a test key: %v", err)
		}
		rsaKey = key
	})
	return rsaKey
}

type certOpts struct {
	commonName string
	dnsNames   []string
	ips        []net.IP
	serial     *big.Int
	notBefore  time.Time
	notAfter   time.Time
	key        crypto.Signer
}

func makeCert(t *testing.T, o certOpts) *x509.Certificate {
	t.Helper()
	key := o.key
	if key == nil {
		key = sharedRSAKey(t)
	}
	if o.serial == nil {
		o.serial = big.NewInt(1)
	}
	template := &x509.Certificate{
		SerialNumber:          o.serial,
		Subject:               pkix.Name{CommonName: o.commonName, Organization: []string{"CHIT Test"}},
		NotBefore:             o.notBefore,
		NotAfter:              o.notAfter,
		DNSNames:              o.dnsNames,
		IPAddresses:           o.ips,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatalf("creating a test certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing a test certificate: %v", err)
	}
	return cert
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"bare name gets https", "example.com", "https://example.com/", false},
		{"plain http is kept", "http://example.com", "http://example.com/", false},
		{"scheme and host are lowercased, path is not", "HTTPS://Example.COM/Path", "https://example.com/Path", false},
		{"address with a port", "192.168.1.10:8080", "https://192.168.1.10:8080/", false},
		{"path and query are preserved", "example.com/a?b=c", "https://example.com/a?b=c", false},
		{"trailing space is trimmed", "  example.com  ", "https://example.com/", false},
		{"non web scheme", "ftp://example.com", "", true},
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
		{"scheme with no host", "https://", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeURL(%q) = %q, want an error", tt.in, got)
				}
				if core.CodeOf(err) != core.CodeInvalidInput {
					t.Fatalf("code = %q, want %q", core.CodeOf(err), core.CodeInvalidInput)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeURL(%q) failed: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHumanMS(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 ms"},
		{999, "999 ms"},
		{1000, "1.0 s"},
		{1500, "1.5 s"},
		{65000, "65.0 s"},
	}
	for _, tt := range tests {
		if got := humanMS(tt.in); got != tt.want {
			t.Errorf("humanMS(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDescribeCertFields(t *testing.T) {
	now := time.Date(2025, time.March, 1, 12, 0, 0, 0, time.UTC)
	cert := makeCert(t, certOpts{
		commonName: "portal.example.com",
		dnsNames:   []string{"portal.example.com", "www.example.com"},
		ips:        []net.IP{net.ParseIP("192.168.1.10")},
		serial:     big.NewInt(0x0102030405),
		notBefore:  now.Add(-24 * time.Hour),
		notAfter:   now.Add(90 * 24 * time.Hour),
	})

	state := tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_128_GCM_SHA256,
		PeerCertificates: []*x509.Certificate{cert},
	}
	info := DescribeCert(state, "portal.example.com", now)
	if info == nil {
		t.Fatal("DescribeCert returned nil for a certificate that was presented")
	}

	if info.Version != "TLS 1.3" {
		t.Errorf("Version = %q, want %q", info.Version, "TLS 1.3")
	}
	if info.CipherSuite != "TLS_AES_128_GCM_SHA256" {
		t.Errorf("CipherSuite = %q", info.CipherSuite)
	}
	if info.CommonName != "portal.example.com" {
		t.Errorf("CommonName = %q", info.CommonName)
	}
	if !strings.Contains(info.Subject, "CN=portal.example.com") {
		t.Errorf("Subject = %q, want it to carry the common name", info.Subject)
	}
	if info.Issuer != "portal.example.com" {
		t.Errorf("Issuer = %q, want the self-signed common name", info.Issuer)
	}
	wantSANs := []string{"portal.example.com", "www.example.com", "192.168.1.10"}
	if strings.Join(info.SANs, ",") != strings.Join(wantSANs, ",") {
		t.Errorf("SANs = %v, want %v", info.SANs, wantSANs)
	}
	if info.NotBefore != now.Add(-24*time.Hour).Format(time.RFC3339) {
		t.Errorf("NotBefore = %q", info.NotBefore)
	}
	if info.NotAfter != now.Add(90*24*time.Hour).Format(time.RFC3339) {
		t.Errorf("NotAfter = %q", info.NotAfter)
	}
	if info.DaysRemaining != 90 {
		t.Errorf("DaysRemaining = %d, want 90", info.DaysRemaining)
	}
	if info.Expired || info.NotYetValid {
		t.Errorf("Expired = %v, NotYetValid = %v, want both false", info.Expired, info.NotYetValid)
	}
	if !info.HostnameMatch {
		t.Error("HostnameMatch = false, want true for a name in the SAN list")
	}
	if !info.SelfSigned {
		t.Error("SelfSigned = false, want true")
	}
	if info.ChainValid {
		t.Error("ChainValid = true, want false for a certificate no machine trusts")
	}
	if info.ChainError == "" {
		t.Error("ChainError is empty, want a sentence explaining the failure")
	}
	if len(info.ChainSubjects) != 1 || !strings.Contains(info.ChainSubjects[0], "CN=portal.example.com") {
		t.Errorf("ChainSubjects = %v", info.ChainSubjects)
	}
	if info.SerialNumber != "01:02:03:04:05" {
		t.Errorf("SerialNumber = %q, want %q", info.SerialNumber, "01:02:03:04:05")
	}
	if info.SignatureAlgorithm != "SHA256-RSA" {
		t.Errorf("SignatureAlgorithm = %q", info.SignatureAlgorithm)
	}
	if info.KeyType != "RSA 2048" {
		t.Errorf("KeyType = %q, want %q", info.KeyType, "RSA 2048")
	}
	fingerprint := regexp.MustCompile(`^([0-9A-F]{2}:){31}[0-9A-F]{2}$`)
	if !fingerprint.MatchString(info.SHA256Fingerprint) {
		t.Errorf("SHA256Fingerprint = %q, want 32 colon separated uppercase hex pairs", info.SHA256Fingerprint)
	}

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating an ECDSA key: %v", err)
	}
	ecCert := makeCert(t, certOpts{
		commonName: "ec.example.com",
		dnsNames:   []string{"ec.example.com"},
		notBefore:  now.Add(-time.Hour),
		notAfter:   now.Add(time.Hour),
		key:        ecKey,
	})
	ecInfo := DescribeCert(tls.ConnectionState{
		Version:          tls.VersionTLS12,
		PeerCertificates: []*x509.Certificate{ecCert},
	}, "ec.example.com", now)
	if ecInfo.KeyType != "ECDSA P-256" {
		t.Errorf("KeyType = %q, want %q", ecInfo.KeyType, "ECDSA P-256")
	}
	if ecInfo.Version != "TLS 1.2" {
		t.Errorf("Version = %q, want %q", ecInfo.Version, "TLS 1.2")
	}
}

func TestDescribeCertDaysRemaining(t *testing.T) {
	now := time.Date(2025, time.March, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		notAfter    time.Time
		wantDays    int
		wantExpired bool
	}{
		{"45 days", now.Add(45 * 24 * time.Hour), 45, false},
		{"30 days", now.Add(30 * 24 * time.Hour), 30, false},
		{"7 days", now.Add(7 * 24 * time.Hour), 7, false},
		{"12 hours truncates to 0", now.Add(12 * time.Hour), 0, false},
		{"expired 2 days ago", now.Add(-48 * time.Hour), -2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := makeCert(t, certOpts{
				commonName: "example.com",
				dnsNames:   []string{"example.com"},
				notBefore:  now.Add(-365 * 24 * time.Hour),
				notAfter:   tt.notAfter,
			})
			info := DescribeCert(tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
			}, "example.com", now)
			if info.DaysRemaining != tt.wantDays {
				t.Errorf("DaysRemaining = %d, want %d", info.DaysRemaining, tt.wantDays)
			}
			if info.Expired != tt.wantExpired {
				t.Errorf("Expired = %v, want %v", info.Expired, tt.wantExpired)
			}
			if info.NotYetValid {
				t.Error("NotYetValid = true, want false")
			}
		})
	}
}

func TestDescribeCertNotYetValid(t *testing.T) {
	now := time.Date(2025, time.March, 1, 12, 0, 0, 0, time.UTC)
	cert := makeCert(t, certOpts{
		commonName: "example.com",
		dnsNames:   []string{"example.com"},
		notBefore:  now.Add(24 * time.Hour),
		notAfter:   now.Add(90 * 24 * time.Hour),
	})
	info := DescribeCert(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}, "example.com", now)
	if !info.NotYetValid {
		t.Error("NotYetValid = false, want true for a certificate that starts tomorrow")
	}
	if info.Expired {
		t.Error("Expired = true, want false")
	}
}

func TestChainErrorWording(t *testing.T) {
	cert := makeCert(t, certOpts{
		commonName: "example.com",
		dnsNames:   []string{"example.com"},
		notBefore:  time.Now().Add(-time.Hour),
		notAfter:   time.Now().Add(time.Hour),
	})
	tests := []struct {
		name       string
		err        error
		selfSigned bool
		want       string
	}{
		{"no error", nil, false, ""},
		{
			"expired",
			x509.CertificateInvalidError{Cert: cert, Reason: x509.Expired, Detail: "is expired"},
			false,
			"The certificate has expired.",
		},
		{
			"wrong name",
			x509.HostnameError{Certificate: cert, Host: "other.example.com"},
			false,
			"The certificate does not cover this address. It was issued for a different name.",
		},
		{
			"self signed",
			x509.UnknownAuthorityError{Cert: cert},
			true,
			"The certificate signed itself, so no browser will trust it. That is expected on a printer, a NAS or a lab box, and wrong on a public site.",
		},
		{
			"unknown issuer",
			x509.UnknownAuthorityError{Cert: cert},
			false,
			"The chain is incomplete or the issuer is not trusted on this machine. The server may be missing its intermediate certificate.",
		},
		{
			"anything else",
			errors.New("x509: something the tech cannot act on"),
			false,
			"The certificate could not be verified on this machine.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chainErrorText(tt.err, tt.selfSigned); got != tt.want {
				t.Errorf("chainErrorText = %q, want %q", got, tt.want)
			}
		})
	}
}

// goodCert is a healthy certificate description, ready for a test to spoil one
// field of it.
func goodCert(daysRemaining int, notAfter time.Time) *TLSInfo {
	return &TLSInfo{
		Version:            "TLS 1.3",
		CipherSuite:        "TLS_AES_128_GCM_SHA256",
		CommonName:         "example.com",
		NotAfter:           notAfter.Format(time.RFC3339),
		DaysRemaining:      daysRemaining,
		HostnameMatch:      true,
		ChainValid:         true,
		SignatureAlgorithm: "SHA256-RSA",
	}
}

func TestClassifyOK(t *testing.T) {
	now := time.Date(2025, time.March, 1, 12, 0, 0, 0, time.UTC)
	r := Result{
		URL:      "https://example.com/",
		FinalURL: "https://example.com/",
		Status:   200,
		Timing:   Timing{TotalMs: 120},
		TLS:      goodCert(90, now.Add(90*24*time.Hour)),
		followed: true,
	}
	Classify(&r)

	if r.Level != "ok" {
		t.Fatalf("Level = %q, want ok (errors %v, warnings %v)", r.Level, r.Errors, r.Warnings)
	}
	if len(r.Errors) != 0 || len(r.Warnings) != 0 {
		t.Fatalf("errors %v, warnings %v, want neither", r.Errors, r.Warnings)
	}
	want := "example.com answered 200 in 120 ms. The certificate is good for 90 more days."
	if r.Headline != want {
		t.Errorf("Headline = %q, want %q", r.Headline, want)
	}
}

func TestClassifyCertExpiringSoon(t *testing.T) {
	now := time.Date(2025, time.March, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		days int
		want string
	}{
		{30, "Get the renewal booked in."},
		{8, "Get the renewal booked in."},
		{7, "Renew it this week."},
		{1, "Renew it this week."},
	}
	for _, tt := range tests {
		notAfter := now.Add(time.Duration(tt.days) * 24 * time.Hour)
		r := Result{
			URL:      "https://example.com/",
			FinalURL: "https://example.com/",
			Status:   200,
			Timing:   Timing{TotalMs: 100},
			TLS:      goodCert(tt.days, notAfter),
			followed: true,
		}
		Classify(&r)

		if r.Level != "warn" {
			t.Errorf("%d days: Level = %q, want warn", tt.days, r.Level)
		}
		if len(r.Warnings) != 1 {
			t.Fatalf("%d days: warnings = %v, want exactly one", tt.days, r.Warnings)
		}
		if !strings.HasSuffix(r.Warnings[0], tt.want) {
			t.Errorf("%d days: warning = %q, want it to end with %q", tt.days, r.Warnings[0], tt.want)
		}
		if !strings.Contains(r.Warnings[0], "on 31 March 2025") && !strings.Contains(r.Warnings[0], notAfter.Format("2 January 2006")) {
			t.Errorf("%d days: warning = %q, want the expiry date in it", tt.days, r.Warnings[0])
		}
	}
}

func TestClassifyCertExpired(t *testing.T) {
	info := goodCert(-3, time.Date(2025, time.February, 26, 9, 0, 0, 0, time.UTC))
	info.Expired = true
	// An expired certificate also fails verification, and the expiry sentence
	// must still come first because it is the one that names the fix.
	info.ChainValid = false
	info.ChainError = "The certificate has expired."

	r := Result{
		URL:      "https://example.com/",
		FinalURL: "https://example.com/",
		Timing:   Timing{TotalMs: 90},
		TLS:      info,
		failure:  failureTLS,
		followed: true,
	}
	Classify(&r)

	if r.Level != "error" {
		t.Fatalf("Level = %q, want error", r.Level)
	}
	want := "The certificate expired on 26 February 2025. Browsers will refuse to load this site until it is renewed."
	if r.Errors[0] != want {
		t.Errorf("Errors[0] = %q, want %q", r.Errors[0], want)
	}
	if r.Headline != want {
		t.Errorf("Headline = %q, want the first error", r.Headline)
	}
	for _, sentence := range r.Errors {
		if strings.HasPrefix(sentence, "The secure connection could not be set up") {
			t.Error("the generic handshake sentence fired as well as the expiry one")
		}
	}
}

func TestClassifyStatusCodes(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		followed  bool
		redirects []Hop
		wantLevel string
		wantText  string
	}{
		{"200 is fine", 200, true, nil, "ok", ""},
		{
			"301 with follow off",
			301, false,
			[]Hop{{URL: "https://example.com/", Status: 301, Location: "https://example.com/new"}},
			"warn",
			`That is a redirect to https://example.com/new. Turn on "Follow redirects" to see where it ends up.`,
		},
		{"404", 404, true, nil, "error", "The server answered with 404. The address is reachable but that page is not there or is not allowed."},
		{"500", 500, true, nil, "error", "The server answered with 500, which means the server itself is broken, not the network."},
		{"503", 503, true, nil, "error", "The server answered with 503, which means the server itself is broken, not the network."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Result{
				URL:       "https://example.com/",
				FinalURL:  "https://example.com/",
				Status:    tt.status,
				Redirects: tt.redirects,
				Timing:    Timing{TotalMs: 100},
				followed:  tt.followed,
			}
			Classify(&r)

			if r.Level != tt.wantLevel {
				t.Fatalf("Level = %q, want %q (errors %v, warnings %v)", r.Level, tt.wantLevel, r.Errors, r.Warnings)
			}
			if tt.wantText == "" {
				return
			}
			found := false
			for _, sentence := range append(append([]string{}, r.Errors...), r.Warnings...) {
				if sentence == tt.wantText {
					found = true
				}
			}
			if !found {
				t.Errorf("errors %v and warnings %v, want one of them to be %q", r.Errors, r.Warnings, tt.wantText)
			}
		})
	}
}

func TestClassifySlow(t *testing.T) {
	tests := []struct {
		total    int64
		wantWarn bool
	}{
		{1999, false},
		{2001, true},
	}
	for _, tt := range tests {
		r := Result{
			URL:      "https://example.com/",
			FinalURL: "https://example.com/",
			Status:   200,
			Timing:   Timing{TotalMs: tt.total},
			followed: true,
		}
		Classify(&r)

		slow := false
		for _, sentence := range r.Warnings {
			if strings.HasPrefix(sentence, "The whole check took") {
				slow = true
			}
		}
		if slow != tt.wantWarn {
			t.Errorf("%d ms: slow warning = %v, want %v (warnings %v)", tt.total, slow, tt.wantWarn, r.Warnings)
		}
	}
}

func TestClassifyHTTPDowngrade(t *testing.T) {
	r := Result{
		URL:      "https://example.com/",
		FinalURL: "http://example.com/landing",
		Status:   200,
		Redirects: []Hop{
			{URL: "https://example.com/", Status: 302, Location: "http://example.com/landing"},
		},
		Timing:   Timing{TotalMs: 100},
		followed: true,
	}
	Classify(&r)

	want := "The redirect chain drops from HTTPS to plain HTTP part way through, which is a mistake worth reporting."
	found := false
	for _, sentence := range r.Warnings {
		if sentence == want {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want the downgrade sentence", r.Warnings)
	}
	if r.Level != "warn" {
		t.Errorf("Level = %q, want warn", r.Level)
	}
}

func TestClassifyLongChain(t *testing.T) {
	hop := Hop{URL: "https://example.com/", Status: 302, Location: "https://example.com/"}
	tests := []struct {
		hops     int
		wantWarn bool
	}{
		{3, false},
		{4, true},
	}
	for _, tt := range tests {
		chain := make([]Hop, 0, tt.hops)
		for i := 0; i < tt.hops; i++ {
			chain = append(chain, hop)
		}
		r := Result{
			URL:       "https://example.com/",
			FinalURL:  "https://example.com/",
			Status:    200,
			Redirects: chain,
			Timing:    Timing{TotalMs: 100},
			followed:  true,
		}
		Classify(&r)

		long := false
		for _, sentence := range r.Warnings {
			if strings.HasPrefix(sentence, "That took") {
				long = true
			}
		}
		if long != tt.wantWarn {
			t.Errorf("%d hops: long chain warning = %v, want %v (warnings %v)", tt.hops, long, tt.wantWarn, r.Warnings)
		}
	}
}

func TestCheckAgainstTestServer(t *testing.T) {
	body := "hello, tester"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "test-server")
		w.Write([]byte(body))
	}))
	defer server.Close()

	result, err := Check(context.Background(), Params{URL: server.URL, FollowRedirects: true})
	if err != nil {
		t.Fatalf("Check returned an error: %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Status = %d, want 200", result.Status)
	}
	if result.StatusText != "200 OK" {
		t.Errorf("StatusText = %q, want %q", result.StatusText, "200 OK")
	}
	if result.BodyBytes != int64(len(body)) {
		t.Errorf("BodyBytes = %d, want %d", result.BodyBytes, len(body))
	}
	if result.FinalURL != server.URL+"/" {
		t.Errorf("FinalURL = %q, want %q", result.FinalURL, server.URL+"/")
	}
	if result.TLS != nil {
		t.Errorf("TLS = %+v, want nil for a plain HTTP address", result.TLS)
	}
	if result.ServerHeader != "test-server" {
		t.Errorf("ServerHeader = %q", result.ServerHeader)
	}
	if result.Level != "warn" {
		t.Fatalf("Level = %q, want warn (errors %v, warnings %v)", result.Level, result.Errors, result.Warnings)
	}
	if len(result.Warnings) == 0 || result.Warnings[0] != "This address is plain HTTP, so nothing sent to it is encrypted." {
		t.Errorf("warnings = %v, want the plain HTTP one", result.Warnings)
	}
	if result.IP == "" {
		t.Error("IP is empty, want the address that answered")
	}
	if result.CheckedAt == "" {
		t.Error("CheckedAt is empty")
	}
}

func redirectServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/b", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/c", http.StatusFound)
	})
	mux.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("arrived"))
	})
	return httptest.NewServer(mux)
}

func TestCheckFollowsRedirects(t *testing.T) {
	server := redirectServer(t)
	defer server.Close()

	followed, err := Check(context.Background(), Params{URL: server.URL + "/a", FollowRedirects: true})
	if err != nil {
		t.Fatalf("Check returned an error: %v", err)
	}
	if followed.Status != 200 {
		t.Errorf("Status = %d, want 200", followed.Status)
	}
	if followed.FinalURL != server.URL+"/c" {
		t.Errorf("FinalURL = %q, want %q", followed.FinalURL, server.URL+"/c")
	}
	if len(followed.Redirects) != 2 {
		t.Fatalf("Redirects = %+v, want two hops", followed.Redirects)
	}
	if followed.Redirects[0].URL != server.URL+"/a" || followed.Redirects[0].Status != 302 {
		t.Errorf("first hop = %+v", followed.Redirects[0])
	}
	if followed.Redirects[0].Location != "/b" {
		t.Errorf("first hop Location = %q, want %q", followed.Redirects[0].Location, "/b")
	}
	if followed.Redirects[1].URL != server.URL+"/b" {
		t.Errorf("second hop = %+v", followed.Redirects[1])
	}

	stopped, err := Check(context.Background(), Params{URL: server.URL + "/a", FollowRedirects: false})
	if err != nil {
		t.Fatalf("Check returned an error: %v", err)
	}
	if stopped.Status != 302 {
		t.Errorf("Status = %d, want 302 when redirects are not followed", stopped.Status)
	}
	if len(stopped.Redirects) != 1 {
		t.Fatalf("Redirects = %+v, want one hop", stopped.Redirects)
	}
	if stopped.Level != "warn" {
		t.Errorf("Level = %q, want warn", stopped.Level)
	}
}

func TestCheckRedirectLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer server.Close()

	result, err := Check(context.Background(), Params{URL: server.URL + "/loop", FollowRedirects: true})
	if err != nil {
		t.Fatalf("Check returned an error: %v", err)
	}
	if result.Level != "error" {
		t.Fatalf("Level = %q, want error", result.Level)
	}
	want := "Too many redirects. The site keeps bouncing between addresses and never settles."
	if result.Errors[0] != want {
		t.Errorf("Errors[0] = %q, want %q", result.Errors[0], want)
	}
	if len(result.Redirects) != maxRedirects {
		t.Errorf("Redirects = %d hops, want %d", len(result.Redirects), maxRedirects)
	}
}

func TestCheckTLSServer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secure"))
	}))
	defer server.Close()

	result, err := Check(context.Background(), Params{URL: server.URL, FollowRedirects: true})
	if err != nil {
		t.Fatalf("Check returned an error: %v", err)
	}
	if result.TLS == nil {
		t.Fatal("TLS is nil, want the certificate the test server presented")
	}
	if result.TLS.Version == "" || result.TLS.Version == "unknown" {
		t.Errorf("TLS.Version = %q, want a real version", result.TLS.Version)
	}
	if result.TLS.CipherSuite == "" {
		t.Error("TLS.CipherSuite is empty")
	}
	if !result.TLS.HostnameMatch {
		t.Error("HostnameMatch = false, want true: the test certificate covers 127.0.0.1")
	}
	if result.TLS.ChainValid {
		t.Error("ChainValid = true, want false: the test certificate is not in any trust store")
	}
	if result.TLS.ChainError == "" {
		t.Error("ChainError is empty, want the untrusted issuer wording")
	}
	if !result.TLS.SelfSigned {
		t.Error("SelfSigned = false, want true for the httptest certificate")
	}
	if result.Level != "error" {
		t.Fatalf("Level = %q, want error", result.Level)
	}
	if result.Errors[0] != result.TLS.ChainError {
		t.Errorf("Errors[0] = %q, want the chain error %q", result.Errors[0], result.TLS.ChainError)
	}
	// The bad certificate is reported by InspectTLS and Classify, not by refusing
	// the request, so the tech still sees what the device answered.
	if result.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200 from the server behind the untrusted certificate", result.Status)
	}
	if result.BodyBytes != int64(len("secure")) {
		t.Errorf("BodyBytes = %d, want %d", result.BodyBytes, len("secure"))
	}
}

func TestCheckInvalidParams(t *testing.T) {
	tests := []struct {
		name   string
		params Params
		want   string
	}{
		{
			"empty address",
			Params{URL: "   "},
			"Enter an address to check, for example https://example.com.",
		},
		{
			"not a web address",
			Params{URL: "ftp://example.com"},
			`"ftp://example.com" is not an address this can check. Use something like https://example.com or 192.168.1.10:8080.`,
		},
		{
			"wait too long",
			Params{URL: "https://example.com", TimeoutMS: 60001},
			"The wait must be between 1 and 60 seconds. 60001 ms is outside that.",
		},
		{
			"negative wait",
			Params{URL: "https://example.com", TimeoutMS: -1},
			"The wait must be between 1 and 60 seconds. -1 ms is outside that.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Check(context.Background(), tt.params)
			if err == nil {
				t.Fatal("Check accepted parameters it should have rejected")
			}
			if core.CodeOf(err) != core.CodeInvalidInput {
				t.Errorf("code = %q, want %q", core.CodeOf(err), core.CodeInvalidInput)
			}
			if err.Error() != tt.want {
				t.Errorf("message = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}
