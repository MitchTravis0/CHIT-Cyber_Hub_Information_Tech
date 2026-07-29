package certdec

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

// now is the pinned clock every time-dependent test measures against, so no
// fixture in this file can expire and start failing in a year.
var now = time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

// certOpts carries everything a test varies, so no test writes its own x509
// boilerplate.
type certOpts struct {
	commonName         string
	organization       []string
	dnsNames           []string
	ipAddresses        []net.IP
	emails             []string
	notBefore          time.Time
	notAfter           time.Time
	isCA               bool
	maxPathLen         int
	maxPathLenZero     bool
	keyUsage           x509.KeyUsage
	extKeyUsage        []x509.ExtKeyUsage
	unknownExtKeyUsage []asn1.ObjectIdentifier
	sigAlg             x509.SignatureAlgorithm
	serial             *big.Int

	// Key choice. The default is P-256, which generates in microseconds.
	curve   elliptic.Curve
	rsaBits int
	rsaKey  *rsa.PrivateKey
	ed25519 bool

	parent    *x509.Certificate
	parentKey crypto.Signer
}

// makeCert builds a certificate for the tests. A nil parent means self-signed.
func makeCert(t *testing.T, o certOpts) (*x509.Certificate, crypto.Signer, []byte) {
	t.Helper()

	var key crypto.Signer
	var err error
	switch {
	case o.ed25519:
		_, priv, genErr := ed25519.GenerateKey(rand.Reader)
		key, err = priv, genErr
	case o.rsaKey != nil:
		key = o.rsaKey
	case o.rsaBits > 0:
		key, err = rsa.GenerateKey(rand.Reader, o.rsaBits)
	default:
		curve := o.curve
		if curve == nil {
			curve = elliptic.P256()
		}
		key, err = ecdsa.GenerateKey(curve, rand.Reader)
	}
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}

	serial := o.serial
	if serial == nil {
		serial = big.NewInt(1)
	}
	notBefore, notAfter := o.notBefore, o.notAfter
	if notBefore.IsZero() {
		notBefore = now.Add(-24 * time.Hour)
	}
	if notAfter.IsZero() {
		notAfter = now.Add(200 * 24 * time.Hour)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: o.commonName, Organization: o.organization},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  o.isCA,
		BasicConstraintsValid: true,
		MaxPathLen:            o.maxPathLen,
		MaxPathLenZero:        o.maxPathLenZero,
		KeyUsage:              o.keyUsage,
		ExtKeyUsage:           o.extKeyUsage,
		UnknownExtKeyUsage:    o.unknownExtKeyUsage,
		DNSNames:              o.dnsNames,
		IPAddresses:           o.ipAddresses,
		EmailAddresses:        o.emails,
		SignatureAlgorithm:    o.sigAlg,
	}

	parent, parentKey := tmpl, key
	if o.parent != nil {
		parent, parentKey = o.parent, o.parentKey
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, key.Public(), parentKey)
	if err != nil {
		t.Fatalf("creating a certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing the certificate just created: %v", err)
	}
	return cert, key, der
}

// certPEM re-armours DER so a test can build the bundle a supplier would send.
func certPEM(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// healthy is a leaf with names, 200 days left and nothing wrong with it, used
// as the baseline every "one thing is wrong" test varies from.
func healthy() certOpts {
	return certOpts{
		commonName: "portal.example.com",
		dnsNames:   []string{"portal.example.com"},
		keyUsage:   x509.KeyUsageDigitalSignature,
	}
}

// decodeOne runs one certificate through the whole pipeline, which is the only
// way a test sees the verdict and the chain note a user would see.
func decodeOne(t *testing.T, cert *x509.Certificate) Certificate {
	t.Helper()
	res, err := Decode([]byte(certPEM(cert.Raw)), "pasted text", "", now)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(res.Certificates) != 1 {
		t.Fatalf("got %d certificates, want 1", len(res.Certificates))
	}
	return res.Certificates[0]
}

func TestHexPairs(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"empty is a zero serial", nil, "00"},
		{"single zero byte", []byte{0x00}, "00"},
		{"single high byte is uppercase", []byte{0xAB}, "AB"},
		{"two bytes are joined", []byte{0x01, 0x00}, "01:00"},
		{"above 9F stays uppercase", []byte{0xFF, 0xA0, 0xDE}, "FF:A0:DE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hexPairs(tc.in); got != tc.want {
				t.Errorf("hexPairs(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("twenty bytes give twenty pairs", func(t *testing.T) {
		got := hexPairs(make([]byte, 20))
		if strings.Count(got, ":") != 19 {
			t.Errorf("got %q, want 19 colons", got)
		}
		if len(strings.Split(got, ":")) != 20 {
			t.Errorf("got %q, want 20 pairs", got)
		}
	})
}

func TestSerialNumberFormat(t *testing.T) {
	long, ok := new(big.Int).SetString("00B4C1DE9F2A7E5503C8917A6D4B2F0E9C3A5D71", 16)
	if !ok {
		t.Fatal("could not build the 20 byte serial")
	}

	cases := []struct {
		name   string
		serial *big.Int
		want   string
	}{
		{"zero", big.NewInt(0), "00"},
		{"one", big.NewInt(1), "01"},
		{"255", big.NewInt(255), "FF"},
		{"256", big.NewInt(256), "01:00"},
		{"twenty bytes", long, "B4:C1:DE:9F:2A:7E:55:03:C8:91:7A:6D:4B:2F:0E:9C:3A:5D:71"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hexPairs(tc.serial.Bytes()); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFingerprintAgainstKnownHash(t *testing.T) {
	// The published SHA-256 of the empty input, so this checks the hashing and
	// the formatting independently of any certificate.
	sum := sha256.Sum256(nil)
	const want = "E3:B0:C4:42:98:FC:1C:14:9A:FB:F4:C8:99:6F:B9:24:27:AE:41:E4:64:9B:93:4C:A4:95:99:1B:78:52:B8:55"
	if got := hexPairs(sum[:]); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDateFormat(t *testing.T) {
	instant := time.Date(2026, 1, 14, 9, 31, 2, 0, time.UTC)
	const want = "2026-01-14 09:31:02 UTC"

	if got := instant.UTC().Format(timeLayout); got != want {
		t.Errorf("UTC input rendered %q, want %q", got, want)
	}

	india := time.FixedZone("IST", 5*3600+1800)
	if got := instant.In(india).UTC().Format(timeLayout); got != want {
		t.Errorf("+05:30 input rendered %q, want %q", got, want)
	}
}

func TestDaysRemaining(t *testing.T) {
	cases := []struct {
		name  string
		after time.Duration
		want  int
	}{
		{"45 days", 45 * 24 * time.Hour, 45},
		{"30 days", 30 * 24 * time.Hour, 30},
		{"7 days", 7 * 24 * time.Hour, 7},
		{"12 hours rounds toward zero", 12 * time.Hour, 0},
		{"2 days ago", -2 * 24 * time.Hour, -2},
		{"1 day ago", -24 * time.Hour, -1},
		{"exactly now", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := healthy()
			opts.notBefore = now.Add(-365 * 24 * time.Hour)
			opts.notAfter = now.Add(tc.after)
			cert, _, _ := makeCert(t, opts)
			if got := decodeOne(t, cert).DaysRemaining; got != tc.want {
				t.Errorf("got %d days, want %d", got, tc.want)
			}
		})
	}
}

func TestReadableDN(t *testing.T) {
	cases := []struct {
		name string
		in   pkix.Name
		want string
	}{
		{
			"three attributes get a space after each comma",
			pkix.Name{CommonName: "a", Organization: []string{"b"}, Country: []string{"GB"}},
			"CN=a, O=b, C=GB",
		},
		{
			"a comma escaped inside a value is not a separator",
			pkix.Name{CommonName: "a", Organization: []string{"Example, Ltd"}},
			`CN=a, O=Example\, Ltd`,
		},
		{"an empty name is an empty string", pkix.Name{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readableDN(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWeakSignature(t *testing.T) {
	// Every algorithm crypto/x509 defines. A value a future Go release adds
	// fails this test rather than silently becoming "not weak".
	cases := map[x509.SignatureAlgorithm]bool{
		x509.UnknownSignatureAlgorithm: false,
		x509.MD2WithRSA:                true,
		x509.MD5WithRSA:                true,
		x509.SHA1WithRSA:               true,
		x509.SHA256WithRSA:             false,
		x509.SHA384WithRSA:             false,
		x509.SHA512WithRSA:             false,
		x509.DSAWithSHA1:               true,
		x509.DSAWithSHA256:             false,
		x509.ECDSAWithSHA1:             true,
		x509.ECDSAWithSHA256:           false,
		x509.ECDSAWithSHA384:           false,
		x509.ECDSAWithSHA512:           false,
		x509.SHA256WithRSAPSS:          false,
		x509.SHA384WithRSAPSS:          false,
		x509.SHA512WithRSAPSS:          false,
		x509.PureEd25519:               false,
	}
	for alg, want := range cases {
		t.Run(alg.String(), func(t *testing.T) {
			if got := weakSignature(alg); got != want {
				t.Errorf("weakSignature(%s) = %v, want %v", alg, got, want)
			}
		})
	}
	if len(cases) != int(x509.PureEd25519)+1 {
		t.Errorf("crypto/x509 now defines %d signature algorithms, this test covers %d",
			int(x509.PureEd25519)+1, len(cases))
	}
}

func TestPublicKeyLabel(t *testing.T) {
	rsa1024, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generating an RSA 1024 key: %v", err)
	}
	rsa2048, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating an RSA 2048 key: %v", err)
	}
	rsa4096, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatalf("generating an RSA 4096 key: %v", err)
	}

	cases := []struct {
		name      string
		opts      certOpts
		algorithm string
		bits      int
		label     string
	}{
		{"RSA 1024", certOpts{rsaKey: rsa1024}, "RSA", 1024, "RSA 1024 bit"},
		{"RSA 2048", certOpts{rsaKey: rsa2048}, "RSA", 2048, "RSA 2048 bit"},
		{"RSA 4096", certOpts{rsaKey: rsa4096}, "RSA", 4096, "RSA 4096 bit"},
		{"ECDSA P-256", certOpts{curve: elliptic.P256()}, "ECDSA", 256, "ECDSA P-256"},
		{"ECDSA P-384", certOpts{curve: elliptic.P384()}, "ECDSA", 384, "ECDSA P-384"},
		{"Ed25519", certOpts{ed25519: true}, "Ed25519", 256, "Ed25519"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			opts.commonName = "key.example.com"
			opts.dnsNames = []string{"key.example.com"}
			cert, _, _ := makeCert(t, opts)
			got := decodeOne(t, cert)
			if got.PublicKeyAlgorithm != tc.algorithm {
				t.Errorf("algorithm = %q, want %q", got.PublicKeyAlgorithm, tc.algorithm)
			}
			if got.PublicKeyBits != tc.bits {
				t.Errorf("bits = %d, want %d", got.PublicKeyBits, tc.bits)
			}
			if got.PublicKeyLabel != tc.label {
				t.Errorf("label = %q, want %q", got.PublicKeyLabel, tc.label)
			}
		})
	}
}

func TestPathLenText(t *testing.T) {
	cases := []struct {
		name    string
		cert    x509.Certificate
		wantLen int
		wantTxt string
	}{
		{
			"an end certificate says nothing about authority levels",
			x509.Certificate{IsCA: false, BasicConstraintsValid: true},
			-1, "",
		},
		{
			"a root with no constraint at all",
			x509.Certificate{IsCA: true, BasicConstraintsValid: true, MaxPathLen: -1},
			-1, "No limit on how many levels of authority sit below this one.",
		},
		{
			"a zero that was never encoded is still no limit",
			x509.Certificate{IsCA: true, BasicConstraintsValid: true, MaxPathLen: 0, MaxPathLenZero: false},
			-1, "No limit on how many levels of authority sit below this one.",
		},
		{
			"an encoded zero means end certificates only",
			x509.Certificate{IsCA: true, BasicConstraintsValid: true, MaxPathLen: 0, MaxPathLenZero: true},
			0, "This authority may only issue end certificates, not other authorities.",
		},
		{
			"two more levels",
			x509.Certificate{IsCA: true, BasicConstraintsValid: true, MaxPathLen: 2},
			2, "This authority may have up to 2 more levels of authority below it.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotLen, gotTxt := pathLen(&tc.cert)
			if gotLen != tc.wantLen {
				t.Errorf("MaxPathLen = %d, want %d", gotLen, tc.wantLen)
			}
			if gotTxt != tc.wantTxt {
				t.Errorf("PathLenText = %q, want %q", gotTxt, tc.wantTxt)
			}
		})
	}
}

func TestKeyUsageStrings(t *testing.T) {
	t.Run("three bits come back in table order", func(t *testing.T) {
		opts := healthy()
		opts.isCA = true
		opts.keyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign
		cert, _, _ := makeCert(t, opts)
		want := []string{"Digital signature", "Key encipherment", "Signing other certificates"}
		got := decodeOne(t, cert).KeyUsage
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("no bits give an empty slice", func(t *testing.T) {
		opts := healthy()
		opts.keyUsage = 0
		cert, _, _ := makeCert(t, opts)
		got := decodeOne(t, cert).KeyUsage
		if got == nil || len(got) != 0 {
			t.Errorf("got %#v, want an empty slice", got)
		}
	})
}

func TestExtendedKeyUsageStrings(t *testing.T) {
	t.Run("the two web purposes", func(t *testing.T) {
		opts := healthy()
		opts.extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
		cert, _, _ := makeCert(t, opts)
		got := decodeOne(t, cert).ExtendedKeyUsage
		want := []string{"Web server (TLS)", "Web client (TLS)"}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("an unknown OID stays visible", func(t *testing.T) {
		opts := healthy()
		opts.unknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 3, 6, 1, 4, 1, 311, 10, 3, 4}}
		cert, _, _ := makeCert(t, opts)
		got := decodeOne(t, cert).ExtendedKeyUsage
		want := "Unrecognised purpose (1.3.6.1.4.1.311.10.3.4)"
		if len(got) != 1 || got[0] != want {
			t.Errorf("got %v, want [%q]", got, want)
		}
	})

	t.Run("no purposes give an empty slice", func(t *testing.T) {
		cert, _, _ := makeCert(t, healthy())
		got := decodeOne(t, cert).ExtendedKeyUsage
		if got == nil || len(got) != 0 {
			t.Errorf("got %#v, want an empty slice", got)
		}
	})
}

func TestVerdictExpired(t *testing.T) {
	cases := []struct {
		name   string
		ago    time.Duration
		phrase string
	}{
		{"three days ago", 3 * 24 * time.Hour, "expired 3 days ago"},
		{"one day ago is singular", 24 * time.Hour, "expired 1 day ago"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := healthy()
			opts.notBefore = now.Add(-365 * 24 * time.Hour)
			opts.notAfter = now.Add(-tc.ago)
			cert, _, _ := makeCert(t, opts)
			got := decodeOne(t, cert)

			if got.Status != statusDanger {
				t.Errorf("Status = %q, want %q", got.Status, statusDanger)
			}
			if got.StatusLabel != "Expired" {
				t.Errorf("StatusLabel = %q, want %q", got.StatusLabel, "Expired")
			}
			if !strings.Contains(got.Headline, tc.phrase) {
				t.Errorf("Headline %q does not contain %q", got.Headline, tc.phrase)
			}
			if !strings.Contains(got.Headline, got.NotAfter) {
				t.Errorf("Headline %q does not contain the date %q", got.Headline, got.NotAfter)
			}
		})
	}
}

func TestVerdictNotYetValid(t *testing.T) {
	opts := healthy()
	opts.notBefore = now.Add(5 * 24 * time.Hour)
	opts.notAfter = now.Add(400 * 24 * time.Hour)
	cert, _, _ := makeCert(t, opts)
	got := decodeOne(t, cert)

	if got.Status != statusDanger {
		t.Errorf("Status = %q, want %q", got.Status, statusDanger)
	}
	if got.StatusLabel != "Not yet valid" {
		t.Errorf("StatusLabel = %q, want %q", got.StatusLabel, "Not yet valid")
	}
	want := "This certificate does not start until " + got.NotBefore +
		", which is 5 days away. Either it was issued early, or the clock on the machine that issued it is wrong."
	if got.Headline != want {
		t.Errorf("Headline = %q, want %q", got.Headline, want)
	}
	// The literal 5 is written in rather than read back off the struct, so the
	// field and the sentence cannot drift apart without a failure here.
	if got.DaysUntilValid != 5 {
		t.Errorf("DaysUntilValid = %d, want 5", got.DaysUntilValid)
	}
	if !got.NotYetValid {
		t.Error("NotYetValid = false, want true")
	}
}

// DaysUntilValid is only meaningful while the certificate has not started. On a
// live certificate the subtraction is a large negative number, and shipping that
// to the page would render "starts in -395 days".
func TestDaysUntilValidIsZeroOnceTheCertificateHasStarted(t *testing.T) {
	cases := []struct {
		name       string
		notBefore  time.Duration
		notAfter   time.Duration
		wantUntil  int
		wantNotYet bool
	}{
		{"healthy", -10 * 24 * time.Hour, 400 * 24 * time.Hour, 0, false},
		{"started an hour ago", -time.Hour, 400 * 24 * time.Hour, 0, false},
		{"already expired", -800 * 24 * time.Hour, -30 * 24 * time.Hour, 0, false},
		{"starts in 5 days", 5 * 24 * time.Hour, 400 * 24 * time.Hour, 5, true},
		{"starts in 90 days", 90 * 24 * time.Hour, 400 * 24 * time.Hour, 90, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := healthy()
			opts.notBefore = now.Add(tc.notBefore)
			opts.notAfter = now.Add(tc.notAfter)
			cert, _, _ := makeCert(t, opts)
			got := decodeOne(t, cert)
			if got.DaysUntilValid != tc.wantUntil {
				t.Errorf("DaysUntilValid = %d, want %d", got.DaysUntilValid, tc.wantUntil)
			}
			if got.NotYetValid != tc.wantNotYet {
				t.Errorf("NotYetValid = %v, want %v", got.NotYetValid, tc.wantNotYet)
			}
		})
	}
}

func TestVerdictExpiringSoon(t *testing.T) {
	// Issued by an authority, so nothing but the expiry date is ever wrong with
	// it and the headline cannot be won by the self-signed sentence.
	root, rootKey, _ := makeCert(t, certOpts{
		commonName: "Example Root CA",
		isCA:       true,
		keyUsage:   x509.KeyUsageCertSign,
	})

	cases := []struct {
		name   string
		left   time.Duration
		status string
		label  string
		phrase string
	}{
		{"31 days is still fine", 31 * 24 * time.Hour, statusOK, "Valid", "which is 31 days away"},
		{"30 days books the renewal", 30 * 24 * time.Hour, statusWarn, "Expires soon", "Get the renewal booked in now"},
		{"8 days still books the renewal", 8 * 24 * time.Hour, statusWarn, "Expires soon", "Get the renewal booked in now"},
		{"7 days is this week", 7 * 24 * time.Hour, statusWarn, "Expires soon", "Renew it this week."},
		{"1 day is singular", 24 * time.Hour, statusWarn, "Expires soon", "expires in 1 day,"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := healthy()
			opts.notAfter = now.Add(tc.left)
			opts.parent, opts.parentKey = root, rootKey
			cert, _, _ := makeCert(t, opts)
			got := decodeOne(t, cert)

			if got.Status != tc.status {
				t.Errorf("Status = %q, want %q", got.Status, tc.status)
			}
			if got.StatusLabel != tc.label {
				t.Errorf("StatusLabel = %q, want %q", got.StatusLabel, tc.label)
			}
			if !strings.Contains(got.Headline, tc.phrase) {
				t.Errorf("Headline %q does not contain %q", got.Headline, tc.phrase)
			}
		})
	}
}

func TestVerdictSelfSigned(t *testing.T) {
	// A self-signed leaf that is not a CA is the case CheckSignatureFrom gets
	// wrong, so it is the important one.
	opts := healthy()
	cert, _, _ := makeCert(t, opts)
	got := decodeOne(t, cert)

	if !got.SelfSigned {
		t.Error("SelfSigned = false, want true for a certificate that signed itself")
	}
	if got.Status != statusWarn {
		t.Errorf("Status = %q, want %q", got.Status, statusWarn)
	}
	if got.StatusLabel != "Self-signed" {
		t.Errorf("StatusLabel = %q, want %q", got.StatusLabel, "Self-signed")
	}
	if got.Headline != sentenceSelfSigned {
		t.Errorf("Headline = %q, want the self-signed sentence", got.Headline)
	}
}

func TestVerdictCA(t *testing.T) {
	opts := healthy()
	opts.commonName = "Example Issuing CA"
	opts.dnsNames = nil
	opts.isCA = true
	opts.maxPathLen = 2
	opts.keyUsage = x509.KeyUsageCertSign
	cert, _, _ := makeCert(t, opts)
	got := decodeOne(t, cert)

	if !got.IsCA {
		t.Fatal("IsCA = false, want true")
	}
	want := "This is a certificate authority certificate. It exists to sign other certificates, not to identify a website. " +
		"This authority may have up to 2 more levels of authority below it."
	found := false
	for _, note := range got.Notes {
		if note == want {
			found = true
		}
	}
	if !found {
		t.Errorf("Notes %q do not contain %q", got.Notes, want)
	}
}

func TestVerdictNoSANs(t *testing.T) {
	root, rootKey, _ := makeCert(t, certOpts{
		commonName: "Example Root CA",
		isCA:       true,
		keyUsage:   x509.KeyUsageCertSign,
	})

	t.Run("a leaf with no names at all is refused by browsers", func(t *testing.T) {
		cert, _, _ := makeCert(t, certOpts{
			commonName: "portal.example.com",
			keyUsage:   x509.KeyUsageDigitalSignature,
			parent:     root,
			parentKey:  rootKey,
		})
		got := decodeOne(t, cert)

		if got.Status != statusWarn {
			t.Errorf("Status = %q, want %q", got.Status, statusWarn)
		}
		if got.StatusLabel != "No names listed" {
			t.Errorf("StatusLabel = %q, want %q", got.StatusLabel, "No names listed")
		}
		if len(got.DNSNames) != 0 || len(got.IPAddresses) != 0 ||
			len(got.EmailAddresses) != 0 || len(got.URIs) != 0 {
			t.Error("expected every name list to be empty")
		}
	})

	t.Run("an authority with no names is not labelled that way", func(t *testing.T) {
		got := decodeOne(t, root)
		if got.StatusLabel == "No names listed" {
			t.Error("a certificate authority must not be told it lists no names")
		}
	})
}

func TestVerdictSHA1(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating an RSA key: %v", err)
	}
	opts := healthy()
	opts.rsaKey = key
	// Go still permits signing with SHA-1; it only refuses to verify it.
	opts.sigAlg = x509.SHA1WithRSA
	cert, _, _ := makeCert(t, opts)
	got := decodeOne(t, cert)

	if !got.WeakSignature {
		t.Error("WeakSignature = false, want true")
	}
	if got.Status != statusDanger {
		t.Errorf("Status = %q, want %q", got.Status, statusDanger)
	}
	if got.StatusLabel != "Weak signature" {
		t.Errorf("StatusLabel = %q, want %q", got.StatusLabel, "Weak signature")
	}
	if got.Headline != sentenceSHA1 {
		t.Errorf("Headline = %q, want the SHA-1 sentence", got.Headline)
	}
}

func TestVerdictWeakKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generating an RSA 1024 key: %v", err)
	}
	root, rootKey, _ := makeCert(t, certOpts{
		commonName: "Example Root CA",
		isCA:       true,
		keyUsage:   x509.KeyUsageCertSign,
	})
	opts := healthy()
	opts.rsaKey = key
	opts.parent, opts.parentKey = root, rootKey
	cert, _, _ := makeCert(t, opts)
	got := decodeOne(t, cert)

	if got.Status != statusWarn {
		t.Errorf("Status = %q, want %q", got.Status, statusWarn)
	}
	if got.StatusLabel != "Weak key" {
		t.Errorf("StatusLabel = %q, want %q", got.StatusLabel, "Weak key")
	}
	if !strings.Contains(got.Headline, "1024 bit RSA key") {
		t.Errorf("Headline %q does not name the key size", got.Headline)
	}
}

func TestVerdictOK(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating an RSA key: %v", err)
	}
	root, rootKey, _ := makeCert(t, certOpts{
		commonName: "Example Root CA",
		isCA:       true,
		keyUsage:   x509.KeyUsageCertSign,
	})
	opts := healthy()
	opts.rsaKey = key
	opts.notAfter = now.Add(200 * 24 * time.Hour)
	opts.parent, opts.parentKey = root, rootKey
	cert, _, _ := makeCert(t, opts)
	got := decodeOne(t, cert)

	if got.Status != statusOK {
		t.Errorf("Status = %q, want %q", got.Status, statusOK)
	}
	if got.StatusLabel != "Valid" {
		t.Errorf("StatusLabel = %q, want %q", got.StatusLabel, "Valid")
	}
	want := "This certificate is valid until " + got.NotAfter + ", which is 200 days away."
	if got.Headline != want {
		t.Errorf("Headline = %q, want %q", got.Headline, want)
	}
	if len(got.Notes) != 0 {
		t.Errorf("Notes = %q, want none", got.Notes)
	}
}

// chainFixture builds the root, intermediate and leaf every chain test uses.
func chainFixture(t *testing.T) (root, intermediate, leaf *x509.Certificate) {
	t.Helper()
	root, rootKey, _ := makeCert(t, certOpts{
		commonName: "Example Root CA",
		isCA:       true,
		keyUsage:   x509.KeyUsageCertSign,
	})
	intermediate, intKey, _ := makeCert(t, certOpts{
		commonName: "Example Issuing CA",
		isCA:       true,
		maxPathLen: 0, maxPathLenZero: true,
		keyUsage:  x509.KeyUsageCertSign,
		serial:    big.NewInt(2),
		parent:    root,
		parentKey: rootKey,
	})
	leaf, _, _ = makeCert(t, certOpts{
		commonName: "portal.example.com",
		dnsNames:   []string{"portal.example.com"},
		keyUsage:   x509.KeyUsageDigitalSignature,
		serial:     big.NewInt(3),
		parent:     intermediate,
		parentKey:  intKey,
	})
	return root, intermediate, leaf
}

func decodeBundle(t *testing.T, certs ...*x509.Certificate) Result {
	t.Helper()
	var sb strings.Builder
	for _, cert := range certs {
		sb.WriteString(certPEM(cert.Raw))
	}
	res, err := Decode([]byte(sb.String()), "pasted text", "", now)
	if err != nil {
		t.Fatalf("decoding the bundle: %v", err)
	}
	return res
}

func TestChainInOrder(t *testing.T) {
	root, intermediate, leaf := chainFixture(t)
	res := decodeBundle(t, leaf, intermediate, root)

	if !res.InOrder {
		t.Error("InOrder = false, want true")
	}
	want := []int{1, 2, -1}
	for i, expect := range want {
		if got := res.Certificates[i].IssuerInFile; got != expect {
			t.Errorf("IssuerInFile[%d] = %d, want %d", i, got, expect)
		}
	}
	const wantNote = "These 3 certificates are a complete chain in the right order: the server certificate first, then each authority above it, ending at a root that signed itself."
	if res.ChainNote != wantNote {
		t.Errorf("ChainNote = %q, want %q", res.ChainNote, wantNote)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %q, want none", res.Warnings)
	}
}

func TestChainOutOfOrder(t *testing.T) {
	root, intermediate, leaf := chainFixture(t)
	res := decodeBundle(t, root, leaf, intermediate)

	if res.InOrder {
		t.Error("InOrder = true, want false")
	}
	want := []int{1, 2, 0}
	if len(res.SuggestedOrder) != len(want) {
		t.Fatalf("SuggestedOrder = %v, want %v", res.SuggestedOrder, want)
	}
	for i := range want {
		if res.SuggestedOrder[i] != want[i] {
			t.Fatalf("SuggestedOrder = %v, want %v", res.SuggestedOrder, want)
		}
	}
	const wantNote = "These certificates are not in issuing order. Server software wants the server certificate first, then each certificate that signed it. The right order here is: portal.example.com, then Example Issuing CA, then Example Root CA."
	if res.ChainNote != wantNote {
		t.Errorf("ChainNote = %q, want %q", res.ChainNote, wantNote)
	}
}

func TestChainMissingIntermediate(t *testing.T) {
	root, _, leaf := chainFixture(t)
	res := decodeBundle(t, leaf, root)

	if res.InOrder {
		t.Error("InOrder = true, want false")
	}
	if len(res.SuggestedOrder) != 0 {
		t.Errorf("SuggestedOrder = %v, want empty", res.SuggestedOrder)
	}
	const wantNote = `The certificate for "portal.example.com" was signed by "Example Issuing CA", which is not in this file. If that is an intermediate authority rather than a public root, the server is missing it, and the site will work in some browsers and fail on most phones.`
	if res.ChainNote != wantNote {
		t.Errorf("ChainNote = %q, want %q", res.ChainNote, wantNote)
	}
}

func TestChainParentNotACA(t *testing.T) {
	// An "authority" that was never marked as one, which is exactly what a
	// hand-made certificate from a device tends to look like.
	notACA, notACAKey, _ := makeCert(t, certOpts{
		commonName: "Example Issuing CA",
		keyUsage:   x509.KeyUsageDigitalSignature,
	})
	leaf, _, _ := makeCert(t, certOpts{
		commonName: "portal.example.com",
		dnsNames:   []string{"portal.example.com"},
		keyUsage:   x509.KeyUsageDigitalSignature,
		serial:     big.NewInt(2),
		parent:     notACA,
		parentKey:  notACAKey,
	})
	res := decodeBundle(t, leaf, notACA)

	if got := res.Certificates[0].IssuerInFile; got != -1 {
		t.Errorf("IssuerInFile[0] = %d, want -1", got)
	}
	const wantWarning = `"portal.example.com" says it was issued by "Example Issuing CA", and that certificate is in this file, but it is not marked as a certificate authority. Nothing will accept the pair.`
	found := false
	for _, warning := range res.Warnings {
		if warning == wantWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings %q do not contain %q", res.Warnings, wantWarning)
	}
}

func TestChainSingleCertificate(t *testing.T) {
	t.Run("one issued certificate asks for the intermediates", func(t *testing.T) {
		_, _, leaf := chainFixture(t)
		res := decodeBundle(t, leaf)
		const want = "Only one certificate is in this file. If this is for a web server, the intermediate certificates that signed it are usually needed alongside it, or some browsers and most phones will refuse the site."
		if res.ChainNote != want {
			t.Errorf("ChainNote = %q, want %q", res.ChainNote, want)
		}
		if !res.InOrder {
			t.Error("InOrder = false, want true for a single certificate")
		}
	})

	t.Run("one self-signed certificate says nothing about a chain", func(t *testing.T) {
		cert, _, _ := makeCert(t, healthy())
		res := decodeBundle(t, cert)
		if res.ChainNote != "" {
			t.Errorf("ChainNote = %q, want empty", res.ChainNote)
		}
	})
}

func TestDecodePEMBundle(t *testing.T) {
	root, intermediate, leaf := chainFixture(t)
	// What a supplier's file actually looks like: a comment line, blank lines
	// and Windows line endings.
	text := "Here is the new certificate for the portal.\n\n" +
		certPEM(leaf.Raw) + "\n" +
		"# intermediate\n" +
		certPEM(intermediate.Raw) + "\n\n" +
		certPEM(root.Raw)
	text = strings.ReplaceAll(text, "\n", "\r\n")

	res, err := Decode([]byte(text), "bundle.pem", ".pem", now)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(res.Certificates) != 3 {
		t.Fatalf("got %d certificates, want 3", len(res.Certificates))
	}
	if res.Format != formatPEM {
		t.Errorf("Format = %q, want %q", res.Format, formatPEM)
	}
	if res.Source != "bundle.pem" {
		t.Errorf("Source = %q, want %q", res.Source, "bundle.pem")
	}
}

func TestDecodeBareBase64(t *testing.T) {
	cert, _, der := makeCert(t, healthy())
	wrapped := base64.StdEncoding.EncodeToString(der)

	var lines strings.Builder
	for i := 0; i < len(wrapped); i += 64 {
		end := min(i+64, len(wrapped))
		lines.WriteString(wrapped[i:end])
		lines.WriteString("\n")
	}

	cases := map[string]string{
		"one long line":            wrapped,
		"wrapped at 64 characters": lines.String(),
		"unpadded":                 base64.RawStdEncoding.EncodeToString(der),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := Decode([]byte(input), "pasted text", "", now)
			if err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if len(res.Certificates) != 1 {
				t.Fatalf("got %d certificates, want 1", len(res.Certificates))
			}
			if res.Format != formatBase64 {
				t.Errorf("Format = %q, want %q", res.Format, formatBase64)
			}
			if res.Certificates[0].SHA256Fingerprint != decodeOne(t, cert).SHA256Fingerprint {
				t.Error("the certificate decoded from base64 is not the one that went in")
			}
		})
	}
}

func TestDecodeRawDER(t *testing.T) {
	first, _, firstDER := makeCert(t, healthy())
	second := healthy()
	second.commonName = "other.example.com"
	second.dnsNames = []string{"other.example.com"}
	second.serial = big.NewInt(2)
	_, _, secondDER := makeCert(t, second)

	res, err := Decode(append(append([]byte{}, firstDER...), secondDER...), "pair.der", ".der", now)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(res.Certificates) != 2 {
		t.Fatalf("got %d certificates, want 2", len(res.Certificates))
	}
	if res.Format != formatDER {
		t.Errorf("Format = %q, want %q", res.Format, formatDER)
	}
	if res.Certificates[0].Subject.CommonName != first.Subject.CommonName {
		t.Errorf("first certificate is %q, want %q",
			res.Certificates[0].Subject.CommonName, first.Subject.CommonName)
	}
}

func TestDecodeCorruptBase64(t *testing.T) {
	const text = "-----BEGIN CERTIFICATE-----\nthis is not base64 at all !!!!\n-----END CERTIFICATE-----\n"
	_, err := Decode([]byte(text), "pasted text", "", now)
	assertUserError(t, err, core.CodeInvalidInput, msgCorruptPEM)
}

func TestDecodePrivateKeyOnly(t *testing.T) {
	keyPEM, body := privateKeyPEM(t)

	_, err := Decode([]byte(keyPEM), "pasted text", "", now)
	assertUserError(t, err, core.CodeInvalidInput, msgPrivateKey)

	// A regression that started echoing the input back would leak the key, so
	// check that no run of the key's base64 appears in what a user would see.
	assertNoKeyMaterial(t, err.Error(), body)
}

func TestDecodePrivateKeyWithCertificate(t *testing.T) {
	keyPEM, body := privateKeyPEM(t)
	cert, _, _ := makeCert(t, healthy())

	res, err := Decode([]byte(keyPEM+certPEM(cert.Raw)), "pasted text", "", now)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(res.Certificates) != 1 {
		t.Fatalf("got %d certificates, want 1", len(res.Certificates))
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != msgPrivateKey {
		t.Errorf("Warnings = %q, want just the private key sentence", res.Warnings)
	}

	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshalling the result: %v", err)
	}
	assertNoKeyMaterial(t, string(encoded), body)
}

// privateKeyPEM generates a throwaway key and returns it armoured plus its
// base64 body, so a test can prove none of that body escapes. Generating it
// beats pasting a real key into a source file, which trips secret scanners.
func privateKeyPEM(t *testing.T) (armoured, body string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling the key: %v", err)
	}
	armoured = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
	return armoured, base64.StdEncoding.EncodeToString(der)
}

func assertNoKeyMaterial(t *testing.T, haystack, keyBody string) {
	t.Helper()
	for i := 0; i+16 <= len(keyBody); i += 8 {
		if strings.Contains(haystack, keyBody[i:i+16]) {
			t.Fatalf("key material leaked into %q", haystack)
		}
	}
}

func TestDecodeCSR(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "portal.example.com"},
		DNSNames: []string{"portal.example.com"},
	}, key)
	if err != nil {
		t.Fatalf("creating the request: %v", err)
	}
	csrPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
	newCSRPEM := string(pem.EncodeToMemory(&pem.Block{Type: "NEW CERTIFICATE REQUEST", Bytes: der}))
	cert, _, _ := makeCert(t, healthy())

	t.Run("a request on its own", func(t *testing.T) {
		_, err := Decode([]byte(csrPEM), "pasted text", "", now)
		assertUserError(t, err, core.CodeInvalidInput, msgCSR)
	})

	t.Run("the Microsoft spelling is still a request", func(t *testing.T) {
		_, err := Decode([]byte(newCSRPEM), "pasted text", "", now)
		assertUserError(t, err, core.CodeInvalidInput, msgCSR)
	})

	t.Run("a request alongside a certificate is a warning", func(t *testing.T) {
		res, err := Decode([]byte(csrPEM+certPEM(cert.Raw)), "pasted text", "", now)
		if err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if len(res.Certificates) != 1 {
			t.Fatalf("got %d certificates, want 1", len(res.Certificates))
		}
		if len(res.Warnings) != 1 || res.Warnings[0] != msgCSR {
			t.Errorf("Warnings = %q, want just the request sentence", res.Warnings)
		}
	})
}

func TestDecodeOtherBlock(t *testing.T) {
	cert, key, _ := makeCert(t, healthy())
	pub, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		t.Fatalf("marshalling the public key: %v", err)
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))

	res, err := Decode([]byte(pubPEM+certPEM(cert.Raw)), "pasted text", "", now)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(res.Certificates) != 1 {
		t.Fatalf("got %d certificates, want 1", len(res.Certificates))
	}
	const want = `This file also holds a "PUBLIC KEY" block, which CHIT does not read.`
	if len(res.Warnings) != 1 || res.Warnings[0] != want {
		t.Errorf("Warnings = %q, want [%q]", res.Warnings, want)
	}
}

func TestDecodeGarbage(t *testing.T) {
	cases := map[string][]byte{
		"a sentence":        []byte("hello world"),
		"nothing at all":    {},
		"whitespace only":   []byte("   \n\t\r\n  "),
		"the start of JPEG": {0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46},
		"a JSON document":   []byte(`{"certificate": "yes please"}`),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(input, "pasted text", "", now)
			assertUserError(t, err, core.CodeInvalidInput, msgNotCertificate)
		})
	}
}

func TestDecodePFX(t *testing.T) {
	t.Run("recognised by its bytes even with no extension", func(t *testing.T) {
		raw := append([]byte{0x30, 0x82, 0x04, 0x00, 0x02, 0x01, 0x03}, make([]byte, 64)...)
		_, err := Decode(raw, "keystore", "", now)
		assertUserError(t, err, core.CodeInvalidInput, msgPFX)
	})

	t.Run("the extension wins over the contents", func(t *testing.T) {
		_, _, der := makeCert(t, healthy())
		_, err := Decode(der, "portal.pfx", ".pfx", now)
		assertUserError(t, err, core.CodeInvalidInput, msgPFX)
	})

	t.Run("p12 is the same file", func(t *testing.T) {
		_, _, der := makeCert(t, healthy())
		_, err := Decode(der, "portal.p12", ".p12", now)
		assertUserError(t, err, core.CodeInvalidInput, msgPFX)
	})
}

func TestDecodeTooManyCertificates(t *testing.T) {
	var sb strings.Builder
	for i := range 33 {
		opts := healthy()
		opts.serial = big.NewInt(int64(i + 1))
		_, _, der := makeCert(t, opts)
		sb.WriteString(certPEM(der))
	}

	res, err := Decode([]byte(sb.String()), "trust-store.pem", ".pem", now)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(res.Certificates) != maxCerts {
		t.Fatalf("got %d certificates, want %d", len(res.Certificates), maxCerts)
	}
	found := false
	for _, warning := range res.Warnings {
		if warning == msgTooMany {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings %q do not contain the cap warning", res.Warnings)
	}
}

func TestDecodeFileErrors(t *testing.T) {
	dir := t.TempDir()

	t.Run("no path at all", func(t *testing.T) {
		_, err := DecodeFile("")
		assertUserError(t, err, core.CodeInvalidInput, msgNoFile)
	})

	t.Run("a file that is not there", func(t *testing.T) {
		_, err := DecodeFile(filepath.Join(dir, "gone.cer"))
		assertUserError(t, err, core.CodeNotFound,
			"Could not find gone.cer. It may have been moved or deleted since you picked it.")
	})

	t.Run("an empty file", func(t *testing.T) {
		path := filepath.Join(dir, "empty.cer")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		_, err := DecodeFile(path)
		assertUserError(t, err, core.CodeInvalidInput, "empty.cer is empty, so there is nothing to read.")
	})

	t.Run("a file far too big to be a certificate", func(t *testing.T) {
		path := filepath.Join(dir, "disk-image.cer")
		if err := os.WriteFile(path, make([]byte, 2<<20), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		_, err := DecodeFile(path)
		assertUserError(t, err, core.CodeInvalidInput,
			"disk-image.cer is 2 MB, which is far too big to be a certificate. A certificate file is a few kilobytes. Check you picked the right file.")
	})

	t.Run("a real file decodes", func(t *testing.T) {
		_, _, der := makeCert(t, healthy())
		path := filepath.Join(dir, "portal.cer")
		if err := os.WriteFile(path, der, 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		res, err := DecodeFile(path)
		if err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if len(res.Certificates) != 1 {
			t.Fatalf("got %d certificates, want 1", len(res.Certificates))
		}
		if res.Source != "portal.cer" {
			t.Errorf("Source = %q, want %q", res.Source, "portal.cer")
		}
		if res.Format != formatDER {
			t.Errorf("Format = %q, want %q", res.Format, formatDER)
		}
	})
}

func TestNoNilSlices(t *testing.T) {
	// A minimal certificate: no names, no key usage, no extended key usage.
	cert, _, _ := makeCert(t, certOpts{commonName: "minimal.example.com"})
	res, err := Decode([]byte(certPEM(cert.Raw)), "pasted text", "", now)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshalling the result: %v", err)
	}
	if strings.Contains(string(encoded), "null") {
		t.Errorf("the JSON holds a null, so a slice was left unallocated: %s", encoded)
	}
}

func assertUserError(t *testing.T, err error, code, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error, want %q", message)
	}
	if got := core.CodeOf(err); got != code {
		t.Errorf("code = %q, want %q", got, code)
	}
	if err.Error() != message {
		t.Errorf("message = %q, want %q", err.Error(), message)
	}
}
