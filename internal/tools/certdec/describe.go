package certdec

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The three tones a card can carry, matching the StatusDot tones the page uses.
const (
	statusOK     = "ok"
	statusWarn   = "warn"
	statusDanger = "danger"
)

// The sentences that can be either the headline or a note, so the two can never
// drift apart.
const (
	sentenceSelfSigned = "This certificate signed itself, so nothing will trust it until somebody installs it as trusted. That is normal on a printer, a NAS or a firewall admin page, and wrong on a public website."

	sentenceNoSANs = "This certificate lists no subject alternative names, so it does not say which addresses it covers. Every browser since about 2017 ignores the common name and will refuse it. It has to be reissued with the address in the SAN list."

	sentenceMD5 = "This certificate is signed with MD5, which has been broken since 2008 and can be forged. Treat anything that trusts it as untrusted and get it reissued."

	sentenceSHA1 = "This certificate is signed with SHA-1, which has been refused by browsers, Windows and Java since 2017. It has to be reissued with SHA-256, not renewed with the same settings."

	sentenceWeakKey = "This certificate uses a %d bit RSA key. Anything under 2048 bits is considered breakable and modern software refuses it."

	sentenceCA = "This is a certificate authority certificate. It exists to sign other certificates, not to identify a website. %s"

	sentenceSelfSignedUnverified = "The name on this certificate matches its own issuer, so it signed itself, but the signature uses an algorithm CHIT will not verify."
)

// The two thresholds, deliberately the same numbers internal/tools/sitecheck
// uses, so a tech who checks one certificate with both tools is never told two
// different things about it.
const (
	certWarnDays   = 30
	certUrgentDays = 7
)

// timeLayout renders every date the way openssl and every browser do, in UTC
// with the zone spelled out, so two windows side by side show the same number.
const timeLayout = "2006-01-02 15:04:05 MST"

// describe turns one parsed certificate into the strings the page shows. It is
// pure apart from the two hashes.
func describe(cert *x509.Certificate, index int, now time.Time) Certificate {
	algorithm, bits, label := publicKeyInfo(cert)
	maxPathLen, pathLenText := pathLen(cert)
	sha1Sum := sha1.Sum(cert.Raw)
	sha256Sum := sha256.Sum256(cert.Raw)

	ips := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		ips = append(ips, ip.String())
	}
	uris := make([]string, 0, len(cert.URIs))
	for _, uri := range cert.URIs {
		uris = append(uris, uri.String())
	}

	c := Certificate{
		Index:       index,
		Subject:     certName(cert.Subject),
		Issuer:      certName(cert.Issuer),
		SubjectLine: readableDN(cert.Subject),
		IssuerLine:  readableDN(cert.Issuer),

		SerialNumber: hexPairs(cert.SerialNumber.Bytes()),

		NotBefore: cert.NotBefore.UTC().Format(timeLayout),
		NotAfter:  cert.NotAfter.UTC().Format(timeLayout),
		// Truncated toward zero, so 36 hours left reads as 1 day and an expired
		// certificate reads as a negative number.
		DaysRemaining: int(cert.NotAfter.Sub(now).Hours() / 24),
		Expired:       now.After(cert.NotAfter),
		NotYetValid:   now.Before(cert.NotBefore),
		// Only meaningful while NotYetValid, and zeroed below otherwise so the
		// page cannot render "starts in -300 days" for a live certificate.
		DaysUntilValid: int(cert.NotBefore.Sub(now).Hours() / 24),

		SignatureAlgorithm: cert.SignatureAlgorithm.String(),
		WeakSignature:      weakSignature(cert.SignatureAlgorithm),

		PublicKeyAlgorithm: algorithm,
		PublicKeyBits:      bits,
		PublicKeyLabel:     label,

		DNSNames:       copyStrings(cert.DNSNames),
		IPAddresses:    ips,
		EmailAddresses: copyStrings(cert.EmailAddresses),
		URIs:           uris,

		KeyUsage:         keyUsageStrings(cert.KeyUsage),
		ExtendedKeyUsage: extKeyUsageStrings(cert),

		IsCA:                  cert.IsCA,
		BasicConstraintsValid: cert.BasicConstraintsValid,
		MaxPathLen:            maxPathLen,
		PathLenText:           pathLenText,

		SHA1Fingerprint:   hexPairs(sha1Sum[:]),
		SHA256Fingerprint: hexPairs(sha256Sum[:]),

		SubjectKeyID:   keyID(cert.SubjectKeyId),
		AuthorityKeyID: keyID(cert.AuthorityKeyId),

		Version: cert.Version,
		// Filled in by analyseChain once every certificate in the input is known.
		IssuerInFile: -1,

		Notes: make([]string, 0),
		PEM:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})),
	}

	selfSignedNote := ""
	c.SelfSigned, selfSignedNote = selfSigned(cert)

	if !c.NotYetValid {
		c.DaysUntilValid = 0
	}
	verdict(&c)
	if selfSignedNote != "" {
		c.Notes = append(c.Notes, selfSignedNote)
	}
	return c
}

// verdict sets Status, StatusLabel and Headline, then gathers into Notes every
// sentence that applied but did not become the headline, so a certificate with
// several problems at once loses none of them.
func verdict(c *Certificate) {
	noSANs := len(c.DNSNames) == 0 && len(c.IPAddresses) == 0 &&
		len(c.EmailAddresses) == 0 && len(c.URIs) == 0 && !c.IsCA
	weakKey := c.PublicKeyAlgorithm == "RSA" && c.PublicKeyBits < 2048

	weakSigSentence := sentenceSHA1
	if strings.Contains(c.SignatureAlgorithm, "MD5") || strings.Contains(c.SignatureAlgorithm, "MD2") {
		weakSigSentence = sentenceMD5
	}
	weakKeySentence := fmt.Sprintf(sentenceWeakKey, c.PublicKeyBits)

	switch {
	case c.Expired || c.NotYetValid || c.WeakSignature:
		c.Status = statusDanger
	case c.DaysRemaining <= certWarnDays || c.SelfSigned || noSANs || weakKey:
		c.Status = statusWarn
	default:
		c.Status = statusOK
	}

	// Whichever sentence became the headline, so it is not repeated as a note.
	headlined := ""
	switch {
	case c.Expired:
		c.StatusLabel = "Expired"
		c.Headline = fmt.Sprintf(
			"This certificate expired %s ago, on %s. Anything relying on it is already broken and it has to be replaced, not renewed in place.",
			dayCount(-c.DaysRemaining), c.NotAfter)
	case c.NotYetValid:
		c.StatusLabel = "Not yet valid"
		c.Headline = fmt.Sprintf(
			"This certificate does not start until %s, which is %s away. Either it was issued early, or the clock on the machine that issued it is wrong.",
			c.NotBefore, dayCount(c.DaysUntilValid))
	case c.WeakSignature:
		c.StatusLabel = "Weak signature"
		c.Headline, headlined = weakSigSentence, weakSigSentence
	case c.DaysRemaining <= certUrgentDays:
		c.StatusLabel = "Expires soon"
		c.Headline = fmt.Sprintf(
			"This certificate expires in %s, on %s. Renew it this week.",
			dayCount(c.DaysRemaining), c.NotAfter)
	case c.DaysRemaining <= certWarnDays:
		c.StatusLabel = "Expires soon"
		c.Headline = fmt.Sprintf(
			"This certificate expires in %s, on %s. Get the renewal booked in now: getting one issued and installed usually takes longer than people expect.",
			dayCount(c.DaysRemaining), c.NotAfter)
	case c.SelfSigned:
		c.StatusLabel = "Self-signed"
		c.Headline, headlined = sentenceSelfSigned, sentenceSelfSigned
	case noSANs:
		c.StatusLabel = "No names listed"
		c.Headline, headlined = sentenceNoSANs, sentenceNoSANs
	case weakKey:
		c.StatusLabel = "Weak key"
		c.Headline, headlined = weakKeySentence, weakKeySentence
	default:
		c.StatusLabel = "Valid"
		c.Headline = fmt.Sprintf("This certificate is valid until %s, which is %s away.",
			c.NotAfter, dayCount(c.DaysRemaining))
	}

	if c.SelfSigned && headlined != sentenceSelfSigned {
		c.Notes = append(c.Notes, sentenceSelfSigned)
	}
	if noSANs && headlined != sentenceNoSANs {
		c.Notes = append(c.Notes, sentenceNoSANs)
	}
	if weakKey && headlined != weakKeySentence {
		c.Notes = append(c.Notes, weakKeySentence)
	}
	if c.WeakSignature && headlined != weakSigSentence {
		c.Notes = append(c.Notes, weakSigSentence)
	}
	if c.IsCA {
		c.Notes = append(c.Notes, fmt.Sprintf(sentenceCA, c.PathLenText))
	}
}

// selfSigned reports whether cert signed itself. Comparing the issuer and
// subject names alone is not enough: any certificate can claim its own name
// without holding the key, so the signature is checked too.
func selfSigned(cert *x509.Certificate) (bool, string) {
	// The raw DER, not the string forms: two names can render identically and
	// encode differently.
	if !bytes.Equal(cert.RawIssuer, cert.RawSubject) {
		return false, ""
	}

	// CheckSignature, never CheckSignatureFrom. CheckSignatureFrom enforces the
	// CA rules first and returns a constraint violation whenever the parent is
	// not marked as an authority, and almost every self-signed device
	// certificate is not one. It would therefore report the commonest
	// self-signed certificate there is as not self-signed.
	err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature)
	var insecure x509.InsecureAlgorithmError
	switch {
	case err == nil:
		return true, ""
	case errors.As(err, &insecure):
		// Go refuses to verify MD5 at all and SHA-1 in a certificate, so this is
		// a real branch: an old appliance's own certificate lands here.
		return true, sentenceSelfSignedUnverified
	}
	return false, ""
}

// certName splits a distinguished name into the parts a tech actually reads.
func certName(n pkix.Name) Name {
	return Name{
		CommonName:         n.CommonName,
		Organization:       copyStrings(n.Organization),
		OrganizationalUnit: copyStrings(n.OrganizationalUnit),
		Country:            copyStrings(n.Country),
	}
}

// readableDN renders a distinguished name on one line with a space after every
// separator, so a long name wraps instead of running off the card. The comma
// that separates two attributes has to be told apart from a comma escaped
// inside one value ("O=Example\, Ltd"), which a plain replace would split.
func readableDN(n pkix.Name) string {
	var out strings.Builder
	escaped := false
	for _, r := range n.String() {
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

// shortName is what a sentence calls a certificate: its common name when it has
// one, and the whole name when it does not.
func shortName(n pkix.Name) string {
	if n.CommonName != "" {
		return n.CommonName
	}
	return readableDN(n)
}

// hexPairs is the form Windows certificate properties and Firefox show, which
// is the whole point: fingerprints and serials get compared by eye.
func hexPairs(raw []byte) string {
	if len(raw) == 0 {
		return "00"
	}
	parts := make([]string, len(raw))
	for i, b := range raw {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// keyID formats a subject or authority key id, which is often absent.
func keyID(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return hexPairs(raw)
}

func copyStrings(in []string) []string {
	return append(make([]string, 0, len(in)), in...)
}

// dayCount writes a whole number of days so a headline never reads "1 days".
func dayCount(n int) string {
	if n == 1 {
		return "1 day"
	}
	return strconv.Itoa(n) + " days"
}

// weakSignature covers everything built on MD2, MD5 or SHA-1. It is a switch
// rather than a name match so that a signature algorithm a future Go release
// adds is a deliberate decision here instead of a silent "not weak".
func weakSignature(alg x509.SignatureAlgorithm) bool {
	switch alg {
	case x509.MD2WithRSA, x509.MD5WithRSA, x509.SHA1WithRSA, x509.DSAWithSHA1, x509.ECDSAWithSHA1:
		return true
	}
	return false
}

// publicKeyInfo names the key in a way a tech can compare against a supplier's
// requirements. crypto/dsa is deliberately not imported: it is deprecated, and
// the one DSA certificate a decade falls into the last branch and reads "DSA".
func publicKeyInfo(cert *x509.Certificate) (algorithm string, bits int, label string) {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return "RSA", pub.N.BitLen(), "RSA " + strconv.Itoa(pub.N.BitLen()) + " bit"
	case *ecdsa.PublicKey:
		params := pub.Curve.Params()
		return "ECDSA", params.BitSize, "ECDSA " + params.Name
	case ed25519.PublicKey:
		return "Ed25519", 256, "Ed25519"
	}
	name := cert.PublicKeyAlgorithm.String()
	return name, 0, name
}

// pathLen reads the basic constraints. Go's MaxPathLenZero flag exists because
// a zero value is ambiguous, and getting it wrong reports every root authority
// as "may only issue end certificates", which is the opposite of the truth.
func pathLen(cert *x509.Certificate) (int, string) {
	switch {
	case !cert.IsCA:
		return -1, ""
	case cert.MaxPathLen < 0, cert.MaxPathLen == 0 && !cert.MaxPathLenZero:
		return -1, "No limit on how many levels of authority sit below this one."
	case cert.MaxPathLen == 0:
		return 0, "This authority may only issue end certificates, not other authorities."
	}
	return cert.MaxPathLen, fmt.Sprintf(
		"This authority may have up to %d more levels of authority below it.", cert.MaxPathLen)
}

// keyUsageBits is the whole table in section 6 of the spec, in the order the
// bits are shown, so no x509 constant name ever reaches the screen.
var keyUsageBits = []struct {
	bit  x509.KeyUsage
	text string
}{
	{x509.KeyUsageDigitalSignature, "Digital signature"},
	{x509.KeyUsageContentCommitment, "Non-repudiation"},
	{x509.KeyUsageKeyEncipherment, "Key encipherment"},
	{x509.KeyUsageDataEncipherment, "Data encipherment"},
	{x509.KeyUsageKeyAgreement, "Key agreement"},
	{x509.KeyUsageCertSign, "Signing other certificates"},
	{x509.KeyUsageCRLSign, "Signing revocation lists"},
	{x509.KeyUsageEncipherOnly, "Encipher only"},
	{x509.KeyUsageDecipherOnly, "Decipher only"},
}

func keyUsageStrings(usage x509.KeyUsage) []string {
	out := make([]string, 0, len(keyUsageBits))
	for _, entry := range keyUsageBits {
		if usage&entry.bit != 0 {
			out = append(out, entry.text)
		}
	}
	return out
}

func extKeyUsageStrings(cert *x509.Certificate) []string {
	out := make([]string, 0, len(cert.ExtKeyUsage)+len(cert.UnknownExtKeyUsage))
	for _, usage := range cert.ExtKeyUsage {
		out = append(out, extKeyUsageText(usage))
	}
	for _, oid := range cert.UnknownExtKeyUsage {
		out = append(out, fmt.Sprintf("Unrecognised purpose (%s)", oid.String()))
	}
	return out
}

func extKeyUsageText(usage x509.ExtKeyUsage) string {
	switch usage {
	case x509.ExtKeyUsageAny:
		return "Any purpose"
	case x509.ExtKeyUsageServerAuth:
		return "Web server (TLS)"
	case x509.ExtKeyUsageClientAuth:
		return "Web client (TLS)"
	case x509.ExtKeyUsageCodeSigning:
		return "Code signing"
	case x509.ExtKeyUsageEmailProtection:
		return "Email (S/MIME)"
	case x509.ExtKeyUsageIPSECEndSystem:
		return "IPsec end system"
	case x509.ExtKeyUsageIPSECTunnel:
		return "IPsec tunnel"
	case x509.ExtKeyUsageIPSECUser:
		return "IPsec user"
	case x509.ExtKeyUsageTimeStamping:
		return "Timestamping"
	case x509.ExtKeyUsageOCSPSigning:
		return "OCSP signing"
	case x509.ExtKeyUsageMicrosoftServerGatedCrypto:
		return "Microsoft server gated crypto"
	case x509.ExtKeyUsageNetscapeServerGatedCrypto:
		return "Netscape server gated crypto"
	case x509.ExtKeyUsageMicrosoftCommercialCodeSigning:
		return "Microsoft commercial code signing"
	case x509.ExtKeyUsageMicrosoftKernelCodeSigning:
		return "Microsoft kernel code signing"
	}
	// A purpose a future Go release adds stays visible rather than vanishing.
	return fmt.Sprintf("Unrecognised purpose (%d)", usage)
}
