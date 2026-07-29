package main

import (
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/tools/certdec"
)

// DecodeCertificateText reads certificates out of text a tech pasted: PEM
// armour, or bare base64 with the BEGIN and END lines lost in an email.
func (a *App) DecodeCertificateText(text string) (certdec.Result, error) {
	return certdec.DecodeText(text)
}

// DecodeCertificateFile reads certificates from a file on disk. The file is
// read here rather than in the browser because a .cer or .der file is binary
// and would not survive the trip through JSON as a string.
func (a *App) DecodeCertificateFile(path string) (certdec.Result, error) {
	return certdec.DecodeFile(path)
}

// PickCertFile opens the native file chooser. A cancelled dialog returns an
// empty path and no error, which is not a failure.
func (a *App) PickCertFile() (string, error) {
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose a certificate file",
		Filters: []wruntime.FileFilter{
			{DisplayName: "Certificates", Pattern: "*.pem;*.crt;*.cer;*.der;*.p12;*.pfx;*.txt"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
}
