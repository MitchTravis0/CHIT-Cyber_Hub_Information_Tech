package hashfile

import (
	"fmt"
	"strings"
)

// Verdict is the answer to "is this the file the vendor published?".
type Verdict struct {
	// State is "none", "match", "mismatch" or "unknown".
	State string `json:"state"`
	// Algorithm is "MD5", "SHA-1" or "SHA-256", and empty when State is
	// "none" or "unknown".
	Algorithm string `json:"algorithm"`
	// Expected is the normalised pasted value, lowercase hex. Empty when
	// State is "none".
	Expected string `json:"expected"`
	// Actual is the file's hash for that algorithm. Empty unless State is
	// "match" or "mismatch".
	Actual string `json:"actual"`
	// Message is one sentence for the user. Empty only when State is "none".
	Message string `json:"message"`
}

const (
	matchMessage    = "This is the same file. The %s hash matches the value you pasted, so the file is complete and has not been altered since it was published."
	mismatchMessage = "This is NOT the same file. The %s hash does not match the value you pasted. Do not install or run it. Download it again, and if it still does not match, get the file from somewhere else."
	nonHexMessage   = "That is the right length for a hash but it has characters in it that are not 0 to 9 or a to f. Copy it again from the vendor's page, and make sure nothing extra came with it."
	notAHashMessage = "That does not look like a hash. Paste the MD5, SHA-1 or SHA-256 value from the vendor's page: 32, 40 or 64 letters and numbers."
)

// NormalizeExpected turns whatever a tech pasted into bare lowercase hex. It
// tolerates the four forms people actually copy: bare hex, a whole sha256sum
// line, openssl dgst output, and any of those with stray newlines round it.
func NormalizeExpected(raw string) string {
	fields := strings.Fields(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(raw))
	if len(fields) == 0 {
		return ""
	}

	// Which end of the line the hash is on depends on the form, and the two forms
	// are told apart by the first field. openssl dgst prints "ALGO(name)= <hash>",
	// whose first field is never bare hex. A sha256sum line is "<hash>  <name>",
	// whose first field always is, and where an "=" can only be in the file name:
	// splitting on the last "=" regardless of position threw the hash away and
	// reported a correct value as "that does not look like a hash".
	text := fields[0]
	if !looksHex(strings.TrimPrefix(fields[0], `\`)) && strings.Contains(raw, "=") {
		text = fields[len(fields)-1]
	}
	// "SHA256(setup.exe)=<hash>", with no space, arrives as a single field.
	if i := strings.LastIndex(text, "="); i >= 0 {
		text = text[i+1:]
	}
	// GNU coreutils prefixes the line with a backslash when the file name
	// contains a backslash or a newline.
	text = strings.TrimPrefix(text, `\`)

	return strings.ToLower(text)
}

// Compare decides the verdict. It is pure: no clock, no file system, no
// network, so the whole of the expected-value table drives it directly.
func Compare(raw string, d Digests) Verdict {
	expected := NormalizeExpected(raw)
	if expected == "" {
		return Verdict{State: "none"}
	}

	algorithm, actual := "", ""
	switch len(expected) {
	case 32:
		algorithm, actual = "MD5", d.MD5
	case 40:
		algorithm, actual = "SHA-1", d.SHA1
	case 64:
		algorithm, actual = "SHA-256", d.SHA256
	default:
		return Verdict{State: "unknown", Expected: expected, Message: notAHashMessage}
	}
	if !isHex(expected) {
		return Verdict{State: "unknown", Expected: expected, Message: nonHexMessage}
	}

	state, message := "mismatch", mismatchMessage
	if expected == actual {
		state, message = "match", matchMessage
	}
	return Verdict{
		State:     state,
		Algorithm: algorithm,
		Expected:  expected,
		Actual:    actual,
		Message:   fmt.Sprintf(message, algorithm),
	}
}

// looksHex reports whether s could be a bare hash token, ignoring case. Length
// is deliberately not checked: Compare turns the length into the algorithm and
// into its own message, and this only has to pick the right field off the line.
func looksHex(s string) bool {
	return s != "" && isHex(strings.ToLower(s))
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
