package qrgen

import (
	"strings"
	"testing"

	"chit/internal/core"
)

func TestEscapeWifi(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "Guest", "Guest"},
		{"backslash alone", `\`, `\\`},
		{"semicolon alone", ";", `\;`},
		{"comma alone", ",", `\,`},
		{"colon alone", ":", `\:`},
		{"quote alone", `"`, `\"`},
		{"colon in a password", "p@ss:word", `p@ss\:word`},
		{"one backslash becomes two", `back\slash`, `back\\slash`},
		{"comma between letters", "a,b", `a\,b`},
		{"quoted words", `He said "hi"`, `He said \"hi\"`},
		{"all hex upper", "ABCDEF", `"ABCDEF"`},
		{"all hex digits", "123456", `"123456"`},
		{"all hex mixed", "abc123", `"abc123"`},
		{"a G is not hex", "ABCDEFG", "ABCDEFG"},
		{"empty", "", ""},
		{"every special at once", `a\b;c,d:e"f`, `a\\b\;c\,d\:e\"f`},
		{"non-ASCII is untouched", "Café", "Café"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeWifi(tc.value); got != tc.want {
				t.Errorf("escapeWifi(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestBuildWifiPayload(t *testing.T) {
	cases := []struct {
		name     string
		ssid     string
		password string
		security string
		hidden   bool
		want     string
	}{
		{"the usual case", "Guest-WiFi", "Welcome2026", "WPA", false,
			`WIFI:T:WPA;S:Guest-WiFi;P:Welcome2026;H:false;;`},
		{"WPA3 and hidden", "Guest-WiFi", "Welcome2026", "SAE", true,
			`WIFI:T:SAE;S:Guest-WiFi;P:Welcome2026;H:true;;`},
		{"open network drops the password field", "Coffee Shop", "ignored", "nopass", false,
			`WIFI:T:nopass;S:Coffee Shop;H:false;;`},
		{"open network with no password at all", "Coffee Shop", "", "nopass", false,
			`WIFI:T:nopass;S:Coffee Shop;H:false;;`},
		{"semicolon and colon escaped", "Acme;Corp", "p@ss:word", "WPA", false,
			`WIFI:T:WPA;S:Acme\;Corp;P:p@ss\:word;H:false;;`},
		{"all hex values are quoted", "ABCDEF", "123456", "WPA", false,
			`WIFI:T:WPA;S:"ABCDEF";P:"123456";H:false;;`},
		{"comma and backslash escaped", "a,b", `back\slash`, "WPA", false,
			`WIFI:T:WPA;S:a\,b;P:back\\slash;H:false;;`},
		{"double quotes escaped", `He said "hi"`, "x", "WPA", false,
			`WIFI:T:WPA;S:He said \"hi\";P:x;H:false;;`},
		{"WEP and hidden false", "Old-AP", "1234567890", "WEP", false,
			`WIFI:T:WEP;S:Old-AP;P:"1234567890";H:false;;`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildWifiPayload(tc.ssid, tc.password, tc.security, tc.hidden)
			if got != tc.want {
				t.Errorf("buildWifiPayload() = %q, want %q", got, tc.want)
			}
			if !strings.HasSuffix(got, ";;") {
				t.Errorf("payload %q does not end with two semicolons", got)
			}
		})
	}
}

func TestBuildTextPayload(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"outer spaces trimmed", "  https://example.com  ", "https://example.com"},
		{"trailing newline trimmed", "https://example.com\n", "https://example.com"},
		{"internal spaces kept", "  two  words  ", "two  words"},
		{"internal newlines kept", "line one\nline two", "line one\nline two"},
		{"one byte", "A", "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildTextPayload(tc.text)
			if err != nil {
				t.Fatalf("buildTextPayload(%q) returned %v", tc.text, err)
			}
			if got != tc.want {
				t.Errorf("buildTextPayload(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}

	for _, text := range []string{"", "   ", "\n\t "} {
		if _, err := buildTextPayload(text); err == nil {
			t.Errorf("buildTextPayload(%q) accepted whitespace only", text)
		} else if core.CodeOf(err) != core.CodeInvalidInput {
			t.Errorf("buildTextPayload(%q) code = %q, want %q", text, core.CodeOf(err), core.CodeInvalidInput)
		}
	}
}
