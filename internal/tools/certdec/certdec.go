// Package certdec reads X.509 certificates out of text a tech pasted or out of
// a file they picked, and turns them into sentences a junior can read. It never
// opens a socket, never asks this machine's trust store anything, and never
// parses a private key: the only signatures it checks are the ones between the
// certificates it was handed.
package certdec

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chit/internal/core"
)

const (
	maxInputBytes = 1 << 20
	maxCerts      = 32
	pemBegin      = "-----BEGIN "
)

// The three ways an input can arrive, recorded in Result.Format.
const (
	formatPEM    = "PEM text"
	formatDER    = "binary DER"
	formatBase64 = "base64 without the BEGIN and END lines"
)

const (
	msgPFX = "That is a .pfx or .p12 file. It holds a private key as well as the certificate and is password protected, and CHIT cannot open one. Pull the certificate out first with this command, then open the .pem file here: openssl pkcs12 -in yourfile.pfx -clcerts -nokeys -out cert.pem"

	msgCorruptPEM = "That looks like a certificate, but the text between the BEGIN and END lines is damaged. A line has usually been wrapped or a character lost on the way through email. Ask for the file as an attachment instead of pasted into a message."

	msgPrivateKey = "That is a PRIVATE KEY, not a certificate. Do not paste it into a ticket, a chat or an email: anyone who has it can impersonate the server. CHIT has not shown it and has not saved it. The certificate you want is the block that starts with BEGIN CERTIFICATE."

	msgCSR = "That is a certificate signing request, which is what you send to a certificate authority. It is not a certificate yet. Open the file the authority sent back instead."

	msgOtherBlock = "This file also holds a \"%s\" block, which CHIT does not read."

	msgBadCertBlock = "One block in this file says it is a certificate but could not be read. It may have been damaged in transit."

	msgNotCertificate = "That is not a certificate. CHIT reads a .pem or .crt text file, a .cer or .der binary file, and base64 text pasted without the BEGIN and END lines. A .pfx, a .p7b or a Java keystore has to be converted first."

	msgTooMany = "That file holds more than 32 certificates. CHIT shows the first 32. If this is a trust store rather than a server certificate, open it with the operating system's certificate manager instead."

	msgTextTooBig = `That is a lot of text for a certificate. Paste just the certificate, or use "Choose file" to open the file directly.`
)

const (
	msgNoFile      = "No file was chosen."
	msgFileMissing = "Could not find %s. It may have been moved or deleted since you picked it."
	msgFileDenied  = "This computer would not let CHIT read %s. Check who owns the file, or copy it somewhere you can read."
	msgFileEmpty   = "%s is empty, so there is nothing to read."
	msgFileTooBig  = "%s is %s, which is far too big to be a certificate. A certificate file is a few kilobytes. Check you picked the right file."
	msgFileUnread  = "Could not read %s. Try copying the file somewhere local and opening it again."
)

// Result is everything one paste or one file produced.
type Result struct {
	// Certificates are in the order they appeared in the input, never reordered.
	Certificates []Certificate `json:"certificates"`
	// Format is how the input was read: "PEM text", "binary DER" or
	// "base64 without the BEGIN and END lines".
	Format string `json:"format"`
	// Source is "pasted text" or the file's base name.
	Source string `json:"source"`
	// ChainNote is one sentence about how the certificates fit together, or ""
	// when there is nothing worth saying.
	ChainNote string `json:"chainNote"`
	// InOrder is true when each certificate was signed by the one after it.
	InOrder bool `json:"inOrder"`
	// SuggestedOrder holds indices into Certificates, leaf first, when a single
	// chain could be worked out. Empty when it could not.
	SuggestedOrder []int `json:"suggestedOrder"`
	// Warnings are plain sentences about what else was in the input: a private
	// key, a signing request, a block that was not a certificate.
	Warnings []string `json:"warnings"`
}

// Name is one distinguished name, broken up so the UI can show the parts a tech
// actually reads.
type Name struct {
	CommonName         string   `json:"commonName"`
	Organization       []string `json:"organization"`
	OrganizationalUnit []string `json:"organizationalUnit"`
	Country            []string `json:"country"`
}

// Certificate is one decoded X.509 certificate, already turned into strings a
// junior tech can read. Nothing here is an OID and nothing here is raw ASN.1.
type Certificate struct {
	// Index is the position in the input, zero based.
	Index int `json:"index"`

	Subject Name `json:"subject"`
	Issuer  Name `json:"issuer"`
	// SubjectLine and IssuerLine are the readable one-line forms, for example
	// "CN=portal.example.com, O=Example Ltd, C=GB".
	SubjectLine string `json:"subjectLine"`
	IssuerLine  string `json:"issuerLine"`

	// SerialNumber is uppercase hex in colon-separated pairs, for example
	// "03:4F:A1:9C". A serial of zero renders as "00".
	SerialNumber string `json:"serialNumber"`

	// NotBefore and NotAfter are "2026-01-14 09:31:02 UTC", always in UTC.
	NotBefore string `json:"notBefore"`
	NotAfter  string `json:"notAfter"`
	// DaysRemaining is negative once the certificate has expired.
	DaysRemaining int  `json:"daysRemaining"`
	Expired       bool `json:"expired"`
	NotYetValid   bool `json:"notYetValid"`
	// DaysUntilValid is how long until the certificate starts, and is 0 unless
	// NotYetValid is true. Truncated toward zero the same way DaysRemaining is.
	DaysUntilValid int `json:"daysUntilValid"`

	// SignatureAlgorithm is Go's name, for example "SHA256-RSA".
	SignatureAlgorithm string `json:"signatureAlgorithm"`
	// WeakSignature is true for anything based on MD5 or SHA-1.
	WeakSignature bool `json:"weakSignature"`

	// PublicKeyAlgorithm is "RSA", "ECDSA", "Ed25519" or "DSA".
	PublicKeyAlgorithm string `json:"publicKeyAlgorithm"`
	// PublicKeyBits is the key size. 256 for Ed25519, 0 when unknown.
	PublicKeyBits int `json:"publicKeyBits"`
	// PublicKeyLabel is the readable form: "RSA 2048 bit", "ECDSA P-256",
	// "Ed25519".
	PublicKeyLabel string `json:"publicKeyLabel"`

	DNSNames       []string `json:"dnsNames"`
	IPAddresses    []string `json:"ipAddresses"`
	EmailAddresses []string `json:"emailAddresses"`
	URIs           []string `json:"uris"`

	// KeyUsage and ExtendedKeyUsage are English, never OIDs.
	KeyUsage         []string `json:"keyUsage"`
	ExtendedKeyUsage []string `json:"extendedKeyUsage"`

	IsCA                  bool `json:"isCa"`
	BasicConstraintsValid bool `json:"basicConstraintsValid"`
	// MaxPathLen is -1 when there is no limit or the certificate is not a CA.
	MaxPathLen int `json:"maxPathLen"`
	// PathLenText is the readable form, "" when the certificate is not a CA.
	PathLenText string `json:"pathLenText"`

	// Fingerprints are uppercase hex in colon-separated pairs, matching what
	// Windows and every browser show.
	SHA1Fingerprint   string `json:"sha1Fingerprint"`
	SHA256Fingerprint string `json:"sha256Fingerprint"`

	// SubjectKeyID and AuthorityKeyID are the same format, "" when absent.
	SubjectKeyID   string `json:"subjectKeyId"`
	AuthorityKeyID string `json:"authorityKeyId"`

	SelfSigned bool `json:"selfSigned"`
	// Version is the X.509 version, 3 for anything issued this century.
	Version int `json:"version"`

	// IssuerInFile is the index of the certificate in this same input that
	// signed this one, or -1 when it is not here.
	IssuerInFile int `json:"issuerInFile"`

	// Status is "ok", "warn" or "danger" and drives the StatusDot tone.
	Status string `json:"status"`
	// StatusLabel is the short label for the table, for example "Expires soon".
	StatusLabel string `json:"statusLabel"`
	// Headline is one sentence a junior can read out loud.
	Headline string `json:"headline"`
	// Notes are the extra sentences shown under the headline. May be empty.
	Notes []string `json:"notes"`

	// PEM is this certificate re-encoded as PEM, so a tech can copy it out of a
	// DER file or a mixed bundle.
	PEM string `json:"pem"`
}

// emptyResult allocates every slice, so a Result never marshals a JSON null.
func emptyResult() Result {
	return Result{
		Certificates:   make([]Certificate, 0),
		SuggestedOrder: make([]int, 0),
		Warnings:       make([]string, 0),
	}
}

// DecodeText reads certificates out of text. Source is "pasted text".
func DecodeText(text string) (Result, error) {
	if len(text) > maxInputBytes {
		return emptyResult(), core.Errorf(core.CodeInvalidInput, msgTextTooBig)
	}
	return Decode([]byte(text), "pasted text", "", time.Now())
}

// DecodeFile reads certificates out of a file. Source is the base name.
func DecodeFile(path string) (Result, error) {
	if path == "" {
		return emptyResult(), core.Errorf(core.CodeInvalidInput, msgNoFile)
	}
	name := filepath.Base(path)

	info, err := os.Stat(path)
	if err != nil {
		return emptyResult(), fileError(err, name)
	}
	if info.Size() == 0 {
		return emptyResult(), core.Errorf(core.CodeInvalidInput, msgFileEmpty, name)
	}
	if info.Size() > maxInputBytes {
		return emptyResult(), core.Errorf(core.CodeInvalidInput, msgFileTooBig, name, humanSize(info.Size()))
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return emptyResult(), fileError(err, name)
	}
	return Decode(raw, name, strings.ToLower(filepath.Ext(path)), time.Now())
}

// fileError turns whatever the operating system said into one of the three
// sentences a tech can act on. The raw text never leaves this function.
func fileError(err error, name string) error {
	switch {
	case os.IsNotExist(err):
		return core.Errorf(core.CodeNotFound, msgFileMissing, name)
	case os.IsPermission(err):
		return core.Errorf(core.CodePermission, msgFileDenied, name)
	}
	return core.Errorf(core.CodeInternal, msgFileUnread, name)
}

// Decode is the shared worker. now is a parameter so the tests can pin the
// clock instead of writing fixtures that expire.
func Decode(raw []byte, source, fileExt string, now time.Time) (Result, error) {
	if fileExt == ".pfx" || fileExt == ".p12" || looksLikePKCS12(raw) {
		return emptyResult(), core.Errorf(core.CodeInvalidInput, msgPFX)
	}

	res := emptyResult()
	res.Source = source

	var parsed []*x509.Certificate
	if bytes.Contains(raw, []byte(pemBegin)) {
		certs, err := decodePEMBlocks(raw, &res)
		if err != nil {
			return emptyResult(), err
		}
		parsed, res.Format = certs, formatPEM
	} else if certs, err := x509.ParseCertificates(raw); err == nil && len(certs) > 0 {
		parsed, res.Format = certs, formatDER
	} else if certs, ok := decodeBareBase64(raw); ok {
		parsed, res.Format = certs, formatBase64
	} else {
		return emptyResult(), core.Errorf(core.CodeInvalidInput, msgNotCertificate)
	}

	if len(parsed) > maxCerts {
		parsed = parsed[:maxCerts]
		res.Warnings = append(res.Warnings, msgTooMany)
	}

	res.Certificates = make([]Certificate, 0, len(parsed))
	for i, cert := range parsed {
		res.Certificates = append(res.Certificates, describe(cert, i, now))
	}
	analyseChain(parsed, &res)
	return res, nil
}

// looksLikePKCS12 spots a .pfx that has been renamed to .cer, which is a DER
// SEQUENCE whose first element is the INTEGER 3 (the PFX version). A real
// certificate has another SEQUENCE there, so the two cannot be confused.
func looksLikePKCS12(raw []byte) bool {
	return len(raw) >= 7 && raw[0] == 0x30 && raw[1] == 0x82 &&
		raw[4] == 0x02 && raw[5] == 0x01 && raw[6] == 0x03
}

// decodePEMBlocks walks every PEM block in raw. A private key block is counted
// so the user can be warned about it and is never handed to a parser, so no key
// material can reach the Result or a log.
func decodePEMBlocks(raw []byte, res *Result) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	blocks := 0
	sawKey, sawRequest := false, false

	rest := raw
	for {
		block, remainder := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remainder
		blocks++

		// The request type contains the word CERTIFICATE, so it has to be
		// matched first or every CSR reads as a damaged certificate.
		kind := strings.ToUpper(block.Type)
		switch {
		case strings.Contains(kind, "CERTIFICATE REQUEST"):
			sawRequest = true
		case strings.Contains(kind, "PRIVATE KEY"):
			sawKey = true
		case strings.Contains(kind, "CERTIFICATE"):
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				res.Warnings = append(res.Warnings, msgBadCertBlock)
				continue
			}
			certs = append(certs, cert)
		default:
			res.Warnings = append(res.Warnings, fmt.Sprintf(msgOtherBlock, block.Type))
		}
	}

	if blocks == 0 {
		return nil, core.Errorf(core.CodeInvalidInput, msgCorruptPEM)
	}
	if len(certs) == 0 {
		switch {
		case sawKey:
			return nil, core.Errorf(core.CodeInvalidInput, msgPrivateKey)
		case sawRequest:
			return nil, core.Errorf(core.CodeInvalidInput, msgCSR)
		}
		return nil, core.Errorf(core.CodeInvalidInput, msgNotCertificate)
	}
	if sawKey {
		res.Warnings = append(res.Warnings, msgPrivateKey)
	}
	if sawRequest {
		res.Warnings = append(res.Warnings, msgCSR)
	}
	return certs, nil
}

// decodeBareBase64 handles the certificate that arrived in a ticket with the
// BEGIN and END lines stripped off by a mail client.
func decodeBareBase64(raw []byte) ([]*x509.Certificate, bool) {
	body := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, string(raw))
	if body == "" {
		return nil, false
	}

	der, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		der, err = base64.RawStdEncoding.DecodeString(body)
		if err != nil {
			return nil, false
		}
	}
	certs, err := x509.ParseCertificates(der)
	if err != nil || len(certs) == 0 {
		return nil, false
	}
	return certs, true
}

// humanSize mirrors the frontend's formatBytes so a size shown in an error
// reads the same as a size shown anywhere else in the app.
func humanSize(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	digits := 0
	if value < 10 {
		digits = 1
	}
	text := strconv.FormatFloat(value, 'f', digits, 64)
	return strings.TrimSuffix(text, ".0") + " " + units[unit]
}
