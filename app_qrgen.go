package main

import "chit/internal/tools/qrgen"

// GenerateQR builds the QR module matrix for a Wi-Fi network or a piece of text.
// The frontend draws the matrix; nothing about the request is kept.
func (a *App) GenerateQR(p qrgen.Params) (qrgen.Code, error) {
	return qrgen.Generate(p)
}
