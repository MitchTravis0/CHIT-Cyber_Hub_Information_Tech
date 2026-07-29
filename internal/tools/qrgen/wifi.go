package qrgen

import (
	"strings"

	"chit/internal/core"
)

// The payload formats. Nothing in this file knows anything about QR codes.

// wifiSpecials are the five characters a WIFI: payload must backslash-escape.
const wifiSpecials = `\;,:"`

// escapeWifi prepares one SSID or password for a WIFI: payload. The five
// special characters are backslash-escaped, and a value made only of hex
// digits is wrapped in quotes so a reader does not decode it as raw hex.
func escapeWifi(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range value {
		// One left to right pass: escaping the backslash last would double-escape
		// the backslashes the other four rules just inserted.
		if strings.ContainsRune(wifiSpecials, r) {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	if isAllHex(value) {
		return `"` + out.String() + `"`
	}
	return out.String()
}

// isAllHex reports whether a non-empty value is made only of hex digits. Such a
// value is ambiguous in a WIFI: payload, because a reader is entitled to decode
// it as raw bytes, so it gets wrapped in quotes.
func isAllHex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// buildTextPayload is the text exactly as typed. Only the outer whitespace
// goes: a trailing newline from a paste is never intended and silently changes
// the code.
func buildTextPayload(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", core.Errorf(core.CodeInvalidInput,
			"Type or paste the text or link to put in the code.")
	}
	return trimmed, nil
}

// buildWifiPayload writes the fields in a fixed order, T then S then P then H,
// so the text a tech copies always has the same shape. The record ends with two
// semicolons: one closing H: and one closing the record.
func buildWifiPayload(ssid, password, security string, hidden bool) string {
	var out strings.Builder
	out.WriteString("WIFI:T:")
	out.WriteString(security)
	out.WriteString(";S:")
	out.WriteString(escapeWifi(ssid))
	out.WriteString(";")
	if security != securityNone {
		out.WriteString("P:")
		out.WriteString(escapeWifi(password))
		out.WriteString(";")
	}
	if hidden {
		out.WriteString("H:true;;")
	} else {
		out.WriteString("H:false;;")
	}
	return out.String()
}
