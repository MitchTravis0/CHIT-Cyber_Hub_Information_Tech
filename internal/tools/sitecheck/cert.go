package sitecheck

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// TLSInfo is the certificate the server presented plus the verdict on it.
type TLSInfo struct {
	Version     string `json:"version"`
	CipherSuite string `json:"cipherSuite"`

	Subject    string   `json:"subject"`
	CommonName string   `json:"commonName"`
	Issuer     string   `json:"issuer"`
	SANs       []string `json:"sans"`

	NotBefore     string `json:"notBefore"`
	NotAfter      string `json:"notAfter"`
	DaysRemaining int    `json:"daysRemaining"`
	Expired       bool   `json:"expired"`
	NotYetValid   bool   `json:"notYetValid"`

	HostnameMatch bool   `json:"hostnameMatch"`
	ChainValid    bool   `json:"chainValid"`
	ChainError    string `json:"chainError"`
	SelfSigned    bool   `json:"selfSigned"`
	// ChainSubjects lists what the server sent, leaf first, so a missing
	// intermediate is visible.
	ChainSubjects []string `json:"chainSubjects"`

	SerialNumber       string `json:"serialNumber"`
	SignatureAlgorithm string `json:"signatureAlgorithm"`
	KeyType            string `json:"keyType"`
	SHA256Fingerprint  string `json:"sha256Fingerprint"`
}

// InspectTLS opens its own TLS connection, separate from the HTTP request, so a
// certificate that fails verification is still described instead of vanishing
// behind a handshake error. The returned error is only used to decide that
// there is nothing to describe; it is never shown to a user.
func InspectTLS(ctx context.Context, host, port string) (*TLSInfo, error) {
	conn, err := (&tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true, // the point of this tool is to show a bad certificate, not to refuse it
			ServerName:         host,
		},
	}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	info := DescribeCert(conn.(*tls.Conn).ConnectionState(), host, time.Now())
	if info == nil {
		return nil, errors.New("no certificate was presented")
	}
	return info, nil
}

// DescribeCert maps a finished handshake onto TLSInfo. now is a parameter so
// the expiry arithmetic can be pinned in tests.
func DescribeCert(state tls.ConnectionState, host string, now time.Time) *TLSInfo {
	if len(state.PeerCertificates) == 0 {
		return nil
	}
	leaf := state.PeerCertificates[0]

	subjects := make([]string, 0, len(state.PeerCertificates))
	for _, c := range state.PeerCertificates {
		subjects = append(subjects, c.Subject.String())
	}

	pool := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		pool.AddCert(c)
	}
	_, verifyErr := leaf.Verify(x509.VerifyOptions{DNSName: host, Intermediates: pool})

	selfSigned := leaf.Subject.String() == leaf.Issuer.String()
	fingerprint := sha256.Sum256(leaf.Raw)

	sans := make([]string, 0, len(leaf.DNSNames)+len(leaf.IPAddresses))
	sans = append(sans, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		sans = append(sans, ip.String())
	}

	issuer := leaf.Issuer.CommonName
	if issuer == "" {
		issuer = leaf.Issuer.String()
	}

	return &TLSInfo{
		Version:     versionName(state.Version),
		CipherSuite: tls.CipherSuiteName(state.CipherSuite),

		Subject:    leaf.Subject.String(),
		CommonName: leaf.Subject.CommonName,
		Issuer:     issuer,
		SANs:       sans,

		NotBefore: leaf.NotBefore.Format(time.RFC3339),
		NotAfter:  leaf.NotAfter.Format(time.RFC3339),
		// Truncated toward zero, so 36 hours left reads as 1 day and an expired
		// certificate reads as a negative number.
		DaysRemaining: int(leaf.NotAfter.Sub(now).Hours() / 24),
		Expired:       now.After(leaf.NotAfter),
		NotYetValid:   now.Before(leaf.NotBefore),

		HostnameMatch: leaf.VerifyHostname(host) == nil,
		ChainValid:    verifyErr == nil,
		ChainError:    chainErrorText(verifyErr, selfSigned),
		SelfSigned:    selfSigned,
		ChainSubjects: subjects,

		SerialNumber:       hexColons(leaf.SerialNumber.Bytes()),
		SignatureAlgorithm: leaf.SignatureAlgorithm.String(),
		KeyType:            keyType(leaf.PublicKey),
		SHA256Fingerprint:  hexColons(fingerprint[:]),
	}
}

// chainErrorText turns a verification failure into a sentence a junior tech can
// act on. The raw Go text never leaves this function.
func chainErrorText(err error, selfSigned bool) string {
	if err == nil {
		return ""
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
		return "The certificate has expired."
	}
	var hostname x509.HostnameError
	if errors.As(err, &hostname) {
		return "The certificate does not cover this address. It was issued for a different name."
	}
	var authority x509.UnknownAuthorityError
	if errors.As(err, &authority) {
		if selfSigned {
			return "The certificate signed itself, so no browser will trust it. That is expected on a printer, a NAS or a lab box, and wrong on a public site."
		}
		return "The chain is incomplete or the issuer is not trusted on this machine. The server may be missing its intermediate certificate."
	}
	return "The certificate could not be verified on this machine."
}

func versionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	}
	return "unknown"
}

func keyType(pub any) string {
	switch key := pub.(type) {
	case *rsa.PublicKey:
		return "RSA " + strconv.Itoa(key.N.BitLen())
	case *ecdsa.PublicKey:
		return "ECDSA " + key.Curve.Params().Name
	case ed25519.PublicKey:
		return "Ed25519"
	}
	return "unknown"
}

func hexColons(raw []byte) string {
	parts := make([]string, len(raw))
	for i, b := range raw {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}
