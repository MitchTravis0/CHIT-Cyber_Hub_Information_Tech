package maildns

import "strings"

// MaxSelectorLength is the longest DNS label, which is what a selector is.
const MaxSelectorLength = 63

// CommonSelectors are the selector names CHIT probes, in probe order. Each is
// the documented default of a mail platform a field tech meets.
//
// The list is not authoritative and must never be presented as exhaustive: a
// domain may name its selector anything at all. The UI names what was tried and
// offers a box for a custom one.
var CommonSelectors = []string{
	"default", "google", "selector1", "selector2", "s1", "s2",
	"k1", "k2", "dkim", "mail", "smtp", "pm", "zoho", "protonmail",
}

// DKIMKey is one selector that answered.
type DKIMKey struct {
	Selector string `json:"selector"`
	// Record is the TXT value found at <selector>._domainkey.<domain>.
	Record string `json:"record"`
	// HasKey is false when the record exists but p= is empty, which is how a key
	// is revoked.
	HasKey bool `json:"hasKey"`
	// KeyType is the k tag, defaulting to "rsa" when absent.
	KeyType string `json:"keyType"`
}

// parseDKIM reads a selector's TXT records. ok is false when nothing there is a
// DKIM key record, which is the normal answer for a selector a domain does not
// use.
func parseDKIM(selector string, txts []string) (DKIMKey, bool) {
	for _, txt := range txts {
		trimmed := strings.TrimSpace(txt)
		if !hasDKIMPrefix(trimmed) {
			continue
		}
		key := DKIMKey{Selector: selector, Record: trimmed, KeyType: "rsa"}
		for _, part := range strings.Split(trimmed, ";") {
			name, value, ok := splitTag(part)
			if !ok {
				continue
			}
			switch name {
			case "k":
				if value != "" {
					key.KeyType = strings.ToLower(value)
				}
			case "p":
				// An empty p= is how a key is revoked, and it is the difference
				// between "signing works" and "every signature fails".
				key.HasKey = value != ""
			}
		}
		return key, true
	}
	return DKIMKey{}, false
}

func hasDKIMPrefix(txt string) bool {
	const tag = "v=dkim1"
	if len(txt) < len(tag) || !strings.EqualFold(txt[:len(tag)], tag) {
		return false
	}
	rest := strings.TrimSpace(txt[len(tag):])
	return rest == "" || strings.HasPrefix(rest, ";")
}
