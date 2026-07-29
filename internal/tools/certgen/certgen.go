// Package certgen makes a private key with either a self-signed certificate or
// a certificate signing request. It is the write half of the Certificate
// Decoder: decode reads them, this one produces them.
//
// Nothing here touches the filesystem or the store. The key exists in memory
// for the length of one call and then in whatever the user downloads.
package certgen

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"regexp"
	"strings"
	"time"

	"chit/internal/core"
)

// The two key types the tool offers. RSA 4096 is deliberately absent: it takes
// long enough to generate that a request/response call would look frozen, and
// nothing a field tech meets needs it.
const (
	KeyECDSAP256 = "ecdsa-p256"
	KeyRSA2048   = "rsa-2048"
)

const (
	DefaultDays = 397
	MinDays     = 1
	MaxDays     = 3650
	// Past this, Safari and Chrome refuse a server certificate outright.
	BrowserMaxDays = 398

	MaxCommonName = 64
	MaxFieldLen   = 64
	MaxSANs       = 100
)

// timeLayout is the one internal/tools/certdec uses, so a certificate made here
// and read there shows the same dates in both windows.
const timeLayout = "2006-01-02 15:04:05 MST"

// notBeforeSkew backdates the certificate a little, so a device whose clock is
// a few minutes fast does not reject a certificate made seconds ago.
const notBeforeSkew = 5 * time.Minute

// Params is what the page sends. Every zero value that has a sensible default
// gets one, so a request with only CommonName filled in is a good request.
type Params struct {
	CommonName   string   `json:"commonName"`
	SANs         []string `json:"sans"`
	Organization string   `json:"organization"`
	OrgUnit      string   `json:"orgUnit"`
	Country      string   `json:"country"`
	State        string   `json:"state"`
	Locality     string   `json:"locality"`
	Email        string   `json:"email"`
	KeyType      string   `json:"keyType"`
	Days         int      `json:"days"`
}

// Result carries everything the page shows. PrivateKeyPEM is the only secret
// here and it is never stored anywhere by CHIT.
type Result struct {
	PrivateKeyPEM string `json:"privateKeyPem"`
	// Set for a self-signed certificate, "" for a CSR.
	CertificatePEM string `json:"certificatePem"`
	// Set for a CSR, "" for a self-signed certificate.
	CSRPEM string `json:"csrPem"`

	Subject     string   `json:"subject"`
	DNSNames    []string `json:"dnsNames"`
	IPAddresses []string `json:"ipAddresses"`
	KeyLabel    string   `json:"keyLabel"`
	// "" for a CSR, which has no validity of its own.
	NotBefore string `json:"notBefore"`
	NotAfter  string `json:"notAfter"`
	Days      int    `json:"days"`
	// Colon-separated uppercase hex, "" for a CSR.
	SerialNumber string `json:"serialNumber"`
	Fingerprint  string `json:"fingerprint"`
	// Base file name with no extension, e.g. "nas-branch-local".
	SuggestedName string `json:"suggestedName"`
	// Plain sentences about this result. Never nil.
	Warnings []string `json:"warnings"`
}

// hostPattern is deliberately looser than the RFC: an underscore is invalid in
// a host name and turns up constantly in internal Active Directory names, and
// refusing one would help nobody.
var hostPattern = regexp.MustCompile(`^(\*\.)?([A-Za-z0-9_-]+\.)*[A-Za-z0-9_-]+$`)

var unsafeName = regexp.MustCompile(`[^a-z0-9]+`)

type names struct {
	dns []string
	ips []net.IP
}

// classify splits the common name and the extra names into DNS names and IP
// addresses. The common name is always included, because no browser has read
// the CN field for years and a certificate without it in the SAN list matches
// nothing.
func classify(commonName string, sans []string) (names, error) {
	var out names
	seen := map[string]bool{}

	for _, raw := range append([]string{commonName}, sans...) {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true

		if ip := net.ParseIP(value); ip != nil {
			out.ips = append(out.ips, ip)
			continue
		}
		if !hostPattern.MatchString(value) || len(value) > 253 {
			if strings.ContainsAny(value, " \t") {
				return out, core.Errorf(core.CodeInvalidInput,
					"%q is not a host name or an IP address. Host names have no spaces: try %s.",
					value, strings.ReplaceAll(value, " ", "."))
			}
			return out, core.Errorf(core.CodeInvalidInput,
				"%q is not a host name or an IP address. Use something like nas.branch.local or 192.168.1.50.",
				value)
		}
		for _, label := range strings.Split(strings.TrimPrefix(value, "*."), ".") {
			if len(label) > 63 {
				return out, core.Errorf(core.CodeInvalidInput,
					"%q has a part longer than 63 characters, which no certificate can carry.", value)
			}
		}
		out.dns = append(out.dns, value)
	}
	return out, nil
}

func checkField(label, value string) error {
	if len(value) > MaxFieldLen {
		return core.Errorf(core.CodeInvalidInput, "%s can be at most %d characters.", label, MaxFieldLen)
	}
	return nil
}

// validate catches everything a user can get wrong before a key is generated,
// so a bad request never costs the time of an RSA keygen.
func (p Params) validate(selfSigned bool) (Params, error) {
	p.CommonName = strings.TrimSpace(p.CommonName)
	if p.CommonName == "" {
		return p, core.Errorf(core.CodeInvalidInput,
			"Type the name the device is reached by, for example nas.branch.local or 192.168.1.50.")
	}
	if len(p.CommonName) > MaxCommonName {
		return p, core.Errorf(core.CodeInvalidInput,
			"A common name can be at most %d characters. Put the longer name in the subject alternative names instead.",
			MaxCommonName)
	}
	if len(p.SANs) > MaxSANs {
		return p, core.Errorf(core.CodeInvalidInput,
			"That is more than %d subject alternative names. Keep the list to the names the device is actually reached by.",
			MaxSANs)
	}

	p.Organization = strings.TrimSpace(p.Organization)
	p.OrgUnit = strings.TrimSpace(p.OrgUnit)
	p.State = strings.TrimSpace(p.State)
	p.Locality = strings.TrimSpace(p.Locality)
	p.Email = strings.TrimSpace(p.Email)
	p.Country = strings.ToUpper(strings.TrimSpace(p.Country))

	for _, field := range []struct{ label, value string }{
		{"Organisation", p.Organization},
		{"Department", p.OrgUnit},
		{"State or county", p.State},
		{"Town or city", p.Locality},
		{"Email", p.Email},
	} {
		if err := checkField(field.label, field.value); err != nil {
			return p, err
		}
	}
	if p.Country != "" && len(p.Country) != 2 {
		return p, core.Errorf(core.CodeInvalidInput,
			"A country is the two letter code, for example GB or US. Leave it empty if you are not sure.")
	}

	if p.KeyType == "" {
		p.KeyType = KeyECDSAP256
	}
	if p.KeyType != KeyECDSAP256 && p.KeyType != KeyRSA2048 {
		return p, core.Errorf(core.CodeInvalidInput,
			"%q is not a key type CHIT can make. Choose ECDSA P-256 or RSA 2048.", p.KeyType)
	}

	if !selfSigned {
		p.Days = 0
	} else {
		if p.Days == 0 {
			p.Days = DefaultDays
		}
		if p.Days < MinDays || p.Days > MaxDays {
			return p, core.Errorf(core.CodeInvalidInput,
				"A certificate has to be valid for between %d and %d days. %d is ten years, which is already longer than any device will still be there.",
				MinDays, MaxDays, MaxDays)
		}
	}

	if _, err := classify(p.CommonName, p.SANs); err != nil {
		return p, err
	}
	return p, nil
}

func generateKey(keyType string) (crypto.Signer, string, string, error) {
	switch keyType {
	case KeyRSA2048:
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, "", "", keyFailure()
		}
		block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
		return key, string(pem.EncodeToMemory(block)), "RSA 2048", nil
	default:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, "", "", keyFailure()
		}
		der, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, "", "", keyFailure()
		}
		block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}
		return key, string(pem.EncodeToMemory(block)), "ECDSA P-256", nil
	}
}

func keyFailure() error {
	return core.Errorf(core.CodeInternal,
		"The private key could not be generated on this machine. Try again, and if it keeps happening restart CHIT.")
}

func subjectOf(p Params) pkix.Name {
	name := pkix.Name{CommonName: p.CommonName}
	if p.Organization != "" {
		name.Organization = []string{p.Organization}
	}
	if p.OrgUnit != "" {
		name.OrganizationalUnit = []string{p.OrgUnit}
	}
	if p.Locality != "" {
		name.Locality = []string{p.Locality}
	}
	if p.State != "" {
		name.Province = []string{p.State}
	}
	if p.Country != "" {
		name.Country = []string{p.Country}
	}
	return name
}

// SubjectLine renders the subject the way internal/tools/certdec renders one,
// so a certificate made here and read there shows the same line. Both go
// through pkix.Name.String() and then space out the separators, which is why
// the order is CN, OU, O, L, ST, C and not the order the form asks for them in:
// letting Go decide is what stops the two tools drifting apart.
func SubjectLine(p Params) string {
	var out strings.Builder
	escaped := false
	for _, r := range subjectOf(p).String() {
		switch {
		case escaped:
			out.WriteRune(r)
			escaped = false
		case r == '\\':
			out.WriteRune(r)
			escaped = true
		case r == ',':
			out.WriteString(", ")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// SuggestedName turns the common name into something every filesystem accepts.
func SuggestedName(commonName string) string {
	value := strings.ToLower(strings.TrimSpace(commonName))
	value = strings.ReplaceAll(value, "*", "wildcard")
	value = unsafeName.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "certificate"
	}
	return value
}

func warningsFor(p Params, selfSigned bool) []string {
	out := []string{}
	if selfSigned && p.Days > BrowserMaxDays {
		out = append(out, fmt.Sprintf(
			"Browsers reject a server certificate valid for more than %d days. Safari and Chrome will both refuse this one even after you trust it. %d days is the safe maximum.",
			BrowserMaxDays, DefaultDays))
	}
	if net.ParseIP(p.CommonName) != nil {
		out = append(out, fmt.Sprintf(
			"%s is an IP address, so the certificate is tied to that address. If the device is ever renumbered the certificate stops matching.",
			p.CommonName))
	} else if !strings.Contains(p.CommonName, ".") {
		out = append(out, fmt.Sprintf(
			"%q has no domain in it. It will only match when the device is reached by exactly that short name, so add the full name (%s.branch.local) as another name too.",
			p.CommonName, p.CommonName))
	}
	return out
}

func fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

func serialText(serial *big.Int) string {
	text := fmt.Sprintf("%X", serial)
	if len(text)%2 == 1 {
		text = "0" + text
	}
	parts := make([]string, 0, len(text)/2)
	for i := 0; i < len(text); i += 2 {
		parts = append(parts, text[i:i+2])
	}
	return strings.Join(parts, ":")
}

// nonNil keeps a nil Go slice from marshalling to JSON null, which the page
// would then have to guard against.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func ipStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out
}

func emails(p Params) []string {
	if p.Email == "" {
		return nil
	}
	return []string{p.Email}
}

// SelfSigned makes a key and a certificate signed by that key.
func SelfSigned(p Params) (Result, error) {
	p, err := p.validate(true)
	if err != nil {
		return Result{}, err
	}
	who, err := classify(p.CommonName, p.SANs)
	if err != nil {
		return Result{}, err
	}
	key, keyPEM, keyLabel, err := generateKey(p.KeyType)
	if err != nil {
		return Result{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return Result{}, keyFailure()
	}
	serial.Add(serial, big.NewInt(1))

	notBefore := time.Now().Add(-notBeforeSkew)
	notAfter := notBefore.Add(time.Duration(p.Days) * 24 * time.Hour)

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               subjectOf(p),
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              who.dns,
		IPAddresses:           who.ips,
		EmailAddresses:        emails(p),
	}
	if p.KeyType == KeyRSA2048 {
		// Only an RSA key can be used to encrypt a session key, and setting the
		// bit on an EC certificate makes some validators unhappy.
		template.KeyUsage |= x509.KeyUsageKeyEncipherment
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return Result{}, core.Errorf(core.CodeInternal,
			"The certificate could not be built on this machine. Try again, and if it keeps happening restart CHIT.")
	}

	return Result{
		PrivateKeyPEM:  keyPEM,
		CertificatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		Subject:        SubjectLine(p),
		DNSNames:       nonNil(who.dns),
		IPAddresses:    ipStrings(who.ips),
		KeyLabel:       keyLabel,
		NotBefore:      notBefore.UTC().Format(timeLayout),
		NotAfter:       notAfter.UTC().Format(timeLayout),
		Days:           p.Days,
		SerialNumber:   serialText(serial),
		Fingerprint:    fingerprint(der),
		SuggestedName:  SuggestedName(p.CommonName),
		Warnings:       warningsFor(p, true),
	}, nil
}

// CSR makes a key and a signing request for a real certificate authority.
func CSR(p Params) (Result, error) {
	p, err := p.validate(false)
	if err != nil {
		return Result{}, err
	}
	who, err := classify(p.CommonName, p.SANs)
	if err != nil {
		return Result{}, err
	}
	key, keyPEM, keyLabel, err := generateKey(p.KeyType)
	if err != nil {
		return Result{}, err
	}

	template := &x509.CertificateRequest{
		Subject:        subjectOf(p),
		DNSNames:       who.dns,
		IPAddresses:    who.ips,
		EmailAddresses: emails(p),
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return Result{}, core.Errorf(core.CodeInternal,
			"The signing request could not be built on this machine. Try again, and if it keeps happening restart CHIT.")
	}

	return Result{
		PrivateKeyPEM: keyPEM,
		CSRPEM:        string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})),
		Subject:       SubjectLine(p),
		DNSNames:      nonNil(who.dns),
		IPAddresses:   ipStrings(who.ips),
		KeyLabel:      keyLabel,
		SuggestedName: SuggestedName(p.CommonName),
		Warnings:      warningsFor(p, false),
	}, nil
}
