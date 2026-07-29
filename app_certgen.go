package main

import "chit/internal/tools/certgen"

// GenerateSelfSigned makes a private key and a certificate signed by that key.
// Nothing is written to disk: the PEM comes back for the page to show and the
// user to download.
func (a *App) GenerateSelfSigned(p certgen.Params) (certgen.Result, error) {
	return certgen.SelfSigned(p)
}

// GenerateCSR makes a private key and a certificate signing request for a real
// certificate authority to sign. Nothing is written to disk.
func (a *App) GenerateCSR(p certgen.Params) (certgen.Result, error) {
	return certgen.CSR(p)
}
