package certdec

import (
	"bytes"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
)

// The relationships one certificate can have with a candidate parent.
const (
	linkUnrelated  = "unrelated"
	linkVerified   = "verified"
	linkUnverified = "unverified"
	linkConstraint = "constraint"
	linkMismatch   = "mismatch"
)

// signedBy reports the relationship between a child and a candidate parent.
// Only the certificates in this one input are ever compared: the machine's own
// trust store is never asked anything, because this laptop is not the machine
// that has to trust the certificate.
func signedBy(child, parent *x509.Certificate) (string, string) {
	// The raw DER, not the string forms: two names can render identically and
	// encode differently.
	if !bytes.Equal(child.RawIssuer, parent.RawSubject) {
		return linkUnrelated, ""
	}

	err := child.CheckSignatureFrom(parent)
	childName := shortName(child.Subject)
	parentName := shortName(parent.Subject)

	var constraint x509.ConstraintViolationError
	var insecure x509.InsecureAlgorithmError
	switch {
	case err == nil:
		return linkVerified, ""
	case errors.As(err, &constraint):
		return linkConstraint, fmt.Sprintf(
			"\"%s\" says it was issued by \"%s\", and that certificate is in this file, but it is not marked as a certificate authority. Nothing will accept the pair.",
			childName, parentName)
	case errors.As(err, &insecure):
		return linkUnverified, fmt.Sprintf(
			"The signature on \"%s\" uses an algorithm CHIT will not verify (usually SHA-1). The names line up with \"%s\", so the order is almost certainly right, but the signature itself was not checked.",
			childName, parentName)
	}
	return linkMismatch, fmt.Sprintf(
		"\"%s\" claims to have been issued by \"%s\", but the signature does not match that certificate. Two different authorities may be using the same name, or one of the files is not what it says it is.",
		childName, parentName)
}

// analyseChain works out which certificate in the input signed which, whether
// the order is the one server software wants, and what to say about it. It
// fills in IssuerInFile on every card plus InOrder, SuggestedOrder, ChainNote
// and any warnings.
func analyseChain(certs []*x509.Certificate, res *Result) {
	n := len(certs)

	seen := make(map[string]bool)
	for i := range n {
		for j := range n {
			if i == j {
				continue
			}
			status, note := signedBy(certs[i], certs[j])
			if (status == linkVerified || status == linkUnverified) &&
				res.Certificates[i].IssuerInFile == -1 {
				res.Certificates[i].IssuerInFile = j
			}
			if note != "" && !seen[note] {
				seen[note] = true
				res.Warnings = append(res.Warnings, note)
			}
		}
	}

	res.InOrder = true
	for i := 0; i < n-1; i++ {
		if res.Certificates[i].IssuerInFile != i+1 {
			res.InOrder = false
			break
		}
	}

	res.SuggestedOrder = suggestOrder(res.Certificates)
	res.ChainNote = chainNote(certs, res)
}

// suggestOrder walks from the leaf up through the issuers. It returns an order
// only when the walk covers every certificate, because a partial walk is not an
// order anybody can act on.
func suggestOrder(certs []Certificate) []int {
	n := len(certs)
	isParent := make([]bool, n)
	for _, c := range certs {
		if c.IssuerInFile >= 0 {
			isParent[c.IssuerInFile] = true
		}
	}

	leaf := -1
	for i := range n {
		if !isParent[i] && !certs[i].SelfSigned {
			leaf = i
			break
		}
	}
	if leaf < 0 {
		return make([]int, 0)
	}

	visited := make([]bool, n)
	order := make([]int, 0, n)
	for i := leaf; i >= 0 && !visited[i]; i = certs[i].IssuerInFile {
		visited[i] = true
		order = append(order, i)
	}
	if len(order) != n {
		return make([]int, 0)
	}
	return order
}

func chainNote(certs []*x509.Certificate, res *Result) string {
	n := len(certs)
	switch {
	case n == 1 && res.Certificates[0].SelfSigned:
		// The self-signed sentence on the card already says it.
		return ""
	case n == 1:
		return "Only one certificate is in this file. If this is for a web server, the intermediate certificates that signed it are usually needed alongside it, or some browsers and most phones will refuse the site."
	case res.InOrder && res.Certificates[n-1].SelfSigned:
		return fmt.Sprintf("These %d certificates are a complete chain in the right order: the server certificate first, then each authority above it, ending at a root that signed itself.", n)
	case res.InOrder:
		return fmt.Sprintf("These %d certificates are in the right order. The one at the top was signed by \"%s\", which is not in this file. That is normal: a public root is already installed on every machine.",
			n, shortName(certs[n-1].Issuer))
	case len(res.SuggestedOrder) > 0:
		names := make([]string, 0, len(res.SuggestedOrder))
		for _, idx := range res.SuggestedOrder {
			names = append(names, shortName(certs[idx].Subject))
		}
		return fmt.Sprintf("These certificates are not in issuing order. Server software wants the server certificate first, then each certificate that signed it. The right order here is: %s.",
			strings.Join(names, ", then "))
	}

	for i := range n {
		if res.Certificates[i].IssuerInFile == -1 && !res.Certificates[i].SelfSigned {
			return fmt.Sprintf("The certificate for \"%s\" was signed by \"%s\", which is not in this file. If that is an intermediate authority rather than a public root, the server is missing it, and the site will work in some browsers and fail on most phones.",
				shortName(certs[i].Subject), shortName(certs[i].Issuer))
		}
	}
	return "These certificates do not form a single chain. This file holds more than one unrelated certificate, which is normal for a bundle of trusted roots and wrong for a server certificate."
}
