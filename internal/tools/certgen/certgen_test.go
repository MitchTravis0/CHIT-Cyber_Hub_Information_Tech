package certgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

func base() Params {
	return Params{CommonName: "nas.branch.local", KeyType: KeyECDSAP256}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name       string
		params     Params
		selfSigned bool
		want       string
	}{
		{
			"empty common name",
			Params{},
			true,
			"Type the name the device is reached by, for example nas.branch.local or 192.168.1.50.",
		},
		{
			"whitespace common name",
			Params{CommonName: "   "},
			true,
			"Type the name the device is reached by, for example nas.branch.local or 192.168.1.50.",
		},
		{
			"common name too long",
			Params{CommonName: strings.Repeat("a", 65)},
			true,
			"A common name can be at most 64 characters. Put the longer name in the subject alternative names instead.",
		},
		{
			"unknown key type",
			Params{CommonName: "nas", KeyType: "dsa"},
			true,
			`"dsa" is not a key type CHIT can make. Choose ECDSA P-256 or RSA 2048.`,
		},
		{
			"days too low",
			Params{CommonName: "nas", Days: -1},
			true,
			"A certificate has to be valid for between 1 and 3650 days. 3650 is ten years, which is already longer than any device will still be there.",
		},
		{
			"days too high",
			Params{CommonName: "nas", Days: 3651},
			true,
			"A certificate has to be valid for between 1 and 3650 days. 3650 is ten years, which is already longer than any device will still be there.",
		},
		{
			"country not two letters",
			Params{CommonName: "nas", Country: "GBR"},
			true,
			"A country is the two letter code, for example GB or US. Leave it empty if you are not sure.",
		},
		{
			"organisation too long",
			Params{CommonName: "nas", Organization: strings.Repeat("b", 65)},
			true,
			"Organisation can be at most 64 characters.",
		},
		{
			"too many names",
			Params{CommonName: "nas", SANs: make([]string, 101)},
			true,
			"That is more than 100 subject alternative names. Keep the list to the names the device is actually reached by.",
		},
		{
			"name with a space",
			Params{CommonName: "nas branch local"},
			true,
			`"nas branch local" is not a host name or an IP address. Host names have no spaces: try nas.branch.local.`,
		},
		{
			"name that is neither",
			Params{CommonName: "nas@@branch"},
			true,
			`"nas@@branch" is not a host name or an IP address. Use something like nas.branch.local or 192.168.1.50.`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.params.validate(tt.selfSigned)
			if err == nil {
				t.Fatalf("expected a rejection")
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("message:\n got %q\nwant %q", got, tt.want)
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %q, want %q", code, core.CodeInvalidInput)
			}
		})
	}
}

// The literals below are the numbers that appear in the messages a user reads,
// written out rather than read back off the constants they are checking.
func TestValidateDefaults(t *testing.T) {
	if DefaultDays != 397 {
		t.Errorf("DefaultDays = %d, want 397", DefaultDays)
	}
	if MinDays != 1 {
		t.Errorf("MinDays = %d, want 1", MinDays)
	}
	if MaxDays != 3650 {
		t.Errorf("MaxDays = %d, want 3650", MaxDays)
	}
	if BrowserMaxDays != 398 {
		t.Errorf("BrowserMaxDays = %d, want 398", BrowserMaxDays)
	}
	if MaxCommonName != 64 {
		t.Errorf("MaxCommonName = %d, want 64", MaxCommonName)
	}
	if MaxFieldLen != 64 {
		t.Errorf("MaxFieldLen = %d, want 64", MaxFieldLen)
	}
	if MaxSANs != 100 {
		t.Errorf("MaxSANs = %d, want 100", MaxSANs)
	}

	got, err := Params{CommonName: "nas.local"}.validate(true)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.Days != 397 {
		t.Errorf("Days = %d, want the 397 day default", got.Days)
	}
	if got.KeyType != KeyECDSAP256 {
		t.Errorf("KeyType = %q, want %q", got.KeyType, KeyECDSAP256)
	}

	for _, days := range []int{1, 3650} {
		if _, err := (Params{CommonName: "nas.local", Days: days}).validate(true); err != nil {
			t.Errorf("%d days should be accepted: %v", days, err)
		}
	}
	for _, days := range []int{-1, 3651} {
		if _, err := (Params{CommonName: "nas.local", Days: days}).validate(true); err == nil {
			t.Errorf("%d days should be rejected", days)
		}
	}

	// A CSR has no validity of its own, so the field is dropped rather than
	// rejected when it is out of range.
	csr, err := Params{CommonName: "nas.local", Days: 99999}.validate(false)
	if err != nil {
		t.Fatalf("csr validate: %v", err)
	}
	if csr.Days != 0 {
		t.Errorf("csr Days = %d, want 0", csr.Days)
	}

	// 64 characters is the limit, not 65. The name is split into labels so it
	// does not trip the separate 63 character limit on one DNS label.
	longName := strings.Repeat("a", 60) + ".loc"
	if len(longName) != 64 {
		t.Fatalf("the test name is %d characters, not 64", len(longName))
	}
	if _, err := (Params{CommonName: longName}).validate(true); err != nil {
		t.Errorf("64 characters should be accepted: %v", err)
	}
	if _, err := (Params{CommonName: "nas", SANs: make([]string, 100)}).validate(true); err != nil {
		t.Errorf("100 names should be accepted: %v", err)
	}
}

func TestClassifyNames(t *testing.T) {
	who, err := classify("nas.branch.local", []string{"nas", "*.branch.local", "192.168.1.50", "2001:db8::1", "  ", "NAS.BRANCH.LOCAL"})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	wantDNS := []string{"nas.branch.local", "nas", "*.branch.local"}
	if !reflect.DeepEqual(who.dns, wantDNS) {
		t.Errorf("dns = %v, want %v", who.dns, wantDNS)
	}
	if len(who.ips) != 2 || who.ips[0].String() != "192.168.1.50" || who.ips[1].String() != "2001:db8::1" {
		t.Errorf("ips = %v", who.ips)
	}

	// The common name is added exactly once even when it is already listed.
	again, err := classify("nas.local", []string{"nas.local"})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if !reflect.DeepEqual(again.dns, []string{"nas.local"}) {
		t.Errorf("dns = %v, want one entry", again.dns)
	}

	// An IP common name goes in as an IP and not as a DNS name.
	ipOnly, err := classify("192.168.1.50", nil)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if len(ipOnly.dns) != 0 || len(ipOnly.ips) != 1 {
		t.Errorf("dns = %v, ips = %v", ipOnly.dns, ipOnly.ips)
	}

	if _, err := classify("nas.local", []string{"bad name"}); err == nil {
		t.Error("a name with a space should be rejected")
	}
	if _, err := classify("nas.local", []string{strings.Repeat("a", 64) + ".local"}); err == nil {
		t.Error("a label over 63 characters should be rejected")
	}
}

func TestSubjectLine(t *testing.T) {
	full := Params{
		CommonName:   "nas.branch.local",
		Organization: "Branch Ltd",
		OrgUnit:      "IT",
		Locality:     "Leeds",
		State:        "West Yorkshire",
		Country:      "GB",
	}
	// The order comes from Go's pkix.Name.String(), which is what the shipped
	// certificate decoder also renders, confirmed by reading a generated
	// certificate back through it.
	want := "CN=nas.branch.local, OU=IT, O=Branch Ltd, L=Leeds, ST=West Yorkshire, C=GB"
	if got := SubjectLine(full); got != want {
		t.Errorf("subject:\n got %q\nwant %q", got, want)
	}
	if got := SubjectLine(Params{CommonName: "nas"}); got != "CN=nas" {
		t.Errorf("subject = %q, want CN=nas", got)
	}
}

func TestSuggestedName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"nas.branch.local", "nas-branch-local"},
		{"*.branch.local", "wildcard-branch-local"},
		{"192.168.1.50", "192-168-1-50"},
		{"a/b:c", "a-b-c"},
		{"NAS.Branch.Local", "nas-branch-local"},
		{"...", "certificate"},
		{"", "certificate"},
	}
	for _, tt := range tests {
		if got := SuggestedName(tt.in); got != tt.want {
			t.Errorf("SuggestedName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWarnings(t *testing.T) {
	long := warningsFor(Params{CommonName: "nas.branch.local", Days: 400}, true)
	if len(long) != 1 {
		t.Fatalf("warnings = %v, want one", long)
	}
	want := "Browsers reject a server certificate valid for more than 398 days. Safari and Chrome will both refuse this one even after you trust it. 397 days is the safe maximum."
	if long[0] != want {
		t.Errorf("warning:\n got %q\nwant %q", long[0], want)
	}

	if got := warningsFor(Params{CommonName: "nas.branch.local", Days: 397}, true); len(got) != 0 {
		t.Errorf("397 days should not warn: %v", got)
	}
	// 398 is the last day that does not warn.
	if got := warningsFor(Params{CommonName: "nas.branch.local", Days: 398}, true); len(got) != 0 {
		t.Errorf("398 days should not warn: %v", got)
	}
	if got := warningsFor(Params{CommonName: "nas.branch.local", Days: 399}, true); len(got) != 1 {
		t.Errorf("399 days should warn: %v", got)
	}

	short := warningsFor(Params{CommonName: "nas", Days: 397}, true)
	if len(short) != 1 || !strings.Contains(short[0], "has no domain in it") {
		t.Errorf("short name warning = %v", short)
	}

	ip := warningsFor(Params{CommonName: "192.168.1.50", Days: 397}, true)
	if len(ip) != 1 || !strings.Contains(ip[0], "is an IP address") {
		t.Errorf("ip warning = %v", ip)
	}

	// A CSR has no validity, so the 398 day rule cannot apply to it.
	if got := warningsFor(Params{CommonName: "nas.branch.local", Days: 4000}, false); len(got) != 0 {
		t.Errorf("csr should not warn about days: %v", got)
	}
}

func decodeCert(t *testing.T, result Result) *x509.Certificate {
	t.Helper()
	block, rest := pem.Decode([]byte(result.CertificatePEM))
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("certificate PEM block = %v", block)
	}
	if strings.TrimSpace(string(rest)) != "" {
		t.Errorf("trailing data after the certificate: %q", rest)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return cert
}

func decodeKey(t *testing.T, result Result) (any, string) {
	t.Helper()
	block, _ := pem.Decode([]byte(result.PrivateKeyPEM))
	if block == nil {
		t.Fatal("no private key PEM block")
	}
	switch block.Type {
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			t.Fatalf("parse EC key: %v", err)
		}
		return key, block.Type
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			t.Fatalf("parse RSA key: %v", err)
		}
		return key, block.Type
	}
	t.Fatalf("unexpected key type %q", block.Type)
	return nil, ""
}

func TestSelfSignedIsUsable(t *testing.T) {
	for _, keyType := range []string{KeyECDSAP256, KeyRSA2048} {
		t.Run(keyType, func(t *testing.T) {
			params := base()
			params.KeyType = keyType
			params.SANs = []string{"nas", "192.168.1.50"}
			params.Days = 30

			result, err := SelfSigned(params)
			if err != nil {
				t.Fatalf("SelfSigned: %v", err)
			}
			if result.CertificatePEM == "" || result.PrivateKeyPEM == "" {
				t.Fatal("a self-signed result must carry both PEM blocks")
			}
			if result.CSRPEM != "" {
				t.Error("a self-signed result must not carry a CSR")
			}

			cert := decodeCert(t, result)
			key, blockType := decodeKey(t, result)

			wantBlock := "EC PRIVATE KEY"
			if keyType == KeyRSA2048 {
				wantBlock = "RSA PRIVATE KEY"
			}
			if blockType != wantBlock {
				t.Errorf("key block = %q, want %q", blockType, wantBlock)
			}

			if !reflect.DeepEqual(cert.DNSNames, []string{"nas.branch.local", "nas"}) {
				t.Errorf("DNSNames = %v", cert.DNSNames)
			}
			if len(cert.IPAddresses) != 1 || cert.IPAddresses[0].String() != "192.168.1.50" {
				t.Errorf("IPAddresses = %v", cert.IPAddresses)
			}
			if cert.Subject.CommonName != "nas.branch.local" {
				t.Errorf("CommonName = %q", cert.Subject.CommonName)
			}

			if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
				t.Errorf("the certificate does not verify against its own key: %v", err)
			}

			switch k := key.(type) {
			case *ecdsa.PrivateKey:
				if !k.PublicKey.Equal(cert.PublicKey) {
					t.Error("the certificate does not carry the returned key's public half")
				}
			case *rsa.PrivateKey:
				if !k.PublicKey.Equal(cert.PublicKey) {
					t.Error("the certificate does not carry the returned key's public half")
				}
			}

			if got := cert.NotAfter.Sub(cert.NotBefore); got != 30*24*time.Hour {
				t.Errorf("validity = %v, want 720h", got)
			}
			if result.Days != 30 {
				t.Errorf("Days = %d, want 30", result.Days)
			}
		})
	}
}

func TestCertificateShape(t *testing.T) {
	first, err := SelfSigned(base())
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	cert := decodeCert(t, first)

	if !cert.BasicConstraintsValid {
		t.Error("BasicConstraintsValid should be set")
	}
	if cert.IsCA {
		t.Error("a server certificate must not be a CA")
	}
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("DigitalSignature should be set")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		t.Error("KeyEncipherment should not be set on an EC certificate")
	}
	if !reflect.DeepEqual(cert.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}) {
		t.Errorf("ExtKeyUsage = %v", cert.ExtKeyUsage)
	}
	if cert.SerialNumber.Sign() <= 0 {
		t.Error("the serial must be positive")
	}
	if cert.SerialNumber.BitLen() < 64 {
		t.Errorf("serial is only %d bits", cert.SerialNumber.BitLen())
	}

	rsaParams := base()
	rsaParams.KeyType = KeyRSA2048
	rsaResult, err := SelfSigned(rsaParams)
	if err != nil {
		t.Fatalf("SelfSigned rsa: %v", err)
	}
	rsaCert := decodeCert(t, rsaResult)
	if rsaCert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Error("KeyEncipherment should be set on an RSA certificate")
	}

	second, err := SelfSigned(base())
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	if first.SerialNumber == second.SerialNumber {
		t.Error("two certificates share a serial number")
	}
	if first.PrivateKeyPEM == second.PrivateKeyPEM {
		t.Error("two certificates share a private key")
	}
}

func TestCSRIsUsable(t *testing.T) {
	params := base()
	params.SANs = []string{"nas", "192.168.1.50"}
	params.Organization = "Branch Ltd"

	result, err := CSR(params)
	if err != nil {
		t.Fatalf("CSR: %v", err)
	}
	if result.CSRPEM == "" || result.PrivateKeyPEM == "" {
		t.Fatal("a CSR result must carry both PEM blocks")
	}
	for name, value := range map[string]string{
		"CertificatePEM": result.CertificatePEM,
		"SerialNumber":   result.SerialNumber,
		"Fingerprint":    result.Fingerprint,
		"NotBefore":      result.NotBefore,
		"NotAfter":       result.NotAfter,
	} {
		if value != "" {
			t.Errorf("%s should be empty on a CSR, got %q", name, value)
		}
	}

	block, _ := pem.Decode([]byte(result.CSRPEM))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatalf("CSR PEM block = %v", block)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Errorf("the request does not verify: %v", err)
	}
	if csr.Subject.CommonName != "nas.branch.local" {
		t.Errorf("CommonName = %q", csr.Subject.CommonName)
	}
	if !reflect.DeepEqual(csr.Subject.Organization, []string{"Branch Ltd"}) {
		t.Errorf("Organization = %v", csr.Subject.Organization)
	}
	if !reflect.DeepEqual(csr.DNSNames, []string{"nas.branch.local", "nas"}) {
		t.Errorf("DNSNames = %v", csr.DNSNames)
	}
	if len(csr.IPAddresses) != 1 || csr.IPAddresses[0].String() != "192.168.1.50" {
		t.Errorf("IPAddresses = %v", csr.IPAddresses)
	}
}

func TestFingerprintMatchesTheDER(t *testing.T) {
	result, err := SelfSigned(base())
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	cert := decodeCert(t, result)
	sum := sha256.Sum256(cert.Raw)

	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	want := strings.Join(parts, ":")
	if result.Fingerprint != want {
		t.Errorf("fingerprint:\n got %s\nwant %s", result.Fingerprint, want)
	}
	if len(result.Fingerprint) != 32*3-1 {
		t.Errorf("fingerprint is %d characters, want 95", len(result.Fingerprint))
	}
	if strings.ToUpper(result.Fingerprint) != result.Fingerprint {
		t.Error("the fingerprint should be uppercase hex")
	}
	if fingerprint([]byte{}) == "" {
		t.Error("fingerprint should never be empty")
	}
	// The formatting rule, written out: two uppercase hex digits per byte,
	// separated by colons.
	if got := fingerprint([]byte("abc")); got != "BA:78:16:BF:8F:01:CF:EA:41:41:40:DE:5D:AE:22:23:B0:03:61:A3:96:17:7A:9C:B4:10:FF:61:F2:00:15:AD" {
		t.Errorf(`fingerprint("abc") = %s`, got)
	}
}

func TestNoSliceIsNil(t *testing.T) {
	// The IP-only case is the one that matters: with no host name anywhere,
	// DNSNames has nothing appended to it and would marshal as JSON null.
	inputs := map[string]Params{
		"host name":      base(),
		"ip only":        {CommonName: "192.168.1.50", KeyType: KeyECDSAP256},
		"no extra names": {CommonName: "nas.branch.local", KeyType: KeyECDSAP256},
	}
	for _, mode := range []struct {
		name string
		run  func(Params) (Result, error)
	}{
		{"self-signed", SelfSigned},
		{"csr", CSR},
	} {
		for label, params := range inputs {
			checkNoNilSlice(t, mode.name+" "+label, mode.run, params)
		}
		result, err := mode.run(base())
		if err != nil {
			t.Fatalf("%s: %v", mode.name, err)
		}
		value := reflect.ValueOf(result)
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if field.Kind() == reflect.Slice && field.IsNil() {
				t.Errorf("%s: %s is nil and would marshal as JSON null", mode.name, value.Type().Field(i).Name)
			}
		}
	}
}

func checkNoNilSlice(t *testing.T, label string, run func(Params) (Result, error), params Params) {
	t.Helper()
	result, err := run(params)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	value := reflect.ValueOf(result)
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if field.Kind() == reflect.Slice && field.IsNil() {
			t.Errorf("%s: %s is nil and would marshal as JSON null", label, value.Type().Field(i).Name)
		}
	}
}

// The security property: this package must never write anything anywhere.
func TestNothingIsPersisted(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	banned := []string{"a.store", "os.WriteFile", "os.Create", "os.OpenFile", "ioutil"}
	checked := 0
	for _, item := range entries {
		name := item.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		checked++
		for _, needle := range banned {
			if strings.Contains(string(body), needle) {
				t.Errorf("%s contains %q: this package must not touch disk or the store", name, needle)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source files were checked, so this test proved nothing")
	}
}

func TestNoSecretLeaksIntoTheSummary(t *testing.T) {
	result, err := SelfSigned(base())
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	body := strings.Join(strings.Split(result.PrivateKeyPEM, "\n")[1:], "")
	body = strings.TrimSuffix(strings.TrimSpace(body), "-----END EC PRIVATE KEY-----")
	if len(body) < 40 {
		t.Fatalf("could not isolate the key body: %q", body)
	}
	fields := append([]string{result.Subject, result.SuggestedName, result.KeyLabel, result.Fingerprint, result.SerialNumber}, result.Warnings...)
	for _, field := range fields {
		if field != "" && strings.Contains(field, body[:40]) {
			t.Errorf("the key body leaked into %q", field)
		}
	}
}

func TestRSAKeyIsTheRightSize(t *testing.T) {
	params := base()
	params.KeyType = KeyRSA2048
	result, err := SelfSigned(params)
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	key, _ := decodeKey(t, result)
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("key is %T, want *rsa.PrivateKey", key)
	}
	if bits := rsaKey.N.BitLen(); bits != 2048 {
		t.Errorf("RSA key is %d bits, want 2048", bits)
	}
	if result.KeyLabel != "RSA 2048" {
		t.Errorf("KeyLabel = %q", result.KeyLabel)
	}

	ecResult, err := SelfSigned(base())
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	ecKey, _ := decodeKey(t, ecResult)
	ec, ok := ecKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("key is %T, want *ecdsa.PrivateKey", ecKey)
	}
	if ec.Curve != elliptic.P256() {
		t.Errorf("curve = %v, want P-256", ec.Curve.Params().Name)
	}
	if ecResult.KeyLabel != "ECDSA P-256" {
		t.Errorf("KeyLabel = %q", ecResult.KeyLabel)
	}
}

func TestSerialText(t *testing.T) {
	result, err := SelfSigned(base())
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	cert := decodeCert(t, result)
	if got := serialText(cert.SerialNumber); got != result.SerialNumber {
		t.Errorf("serial:\n got %s\nwant %s", result.SerialNumber, got)
	}
	for _, part := range strings.Split(result.SerialNumber, ":") {
		if len(part) != 2 {
			t.Fatalf("serial group %q is not two hex digits", part)
		}
	}
}
