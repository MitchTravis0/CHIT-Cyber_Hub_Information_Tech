package urlcheck

import "testing"

// The vectors are the ones printed in RFC 3492 section 7.1, so they are
// authoritative rather than round-tripped through this package's own code.
func TestDecodeLabelRFCVectors(t *testing.T) {
	cases := []struct {
		name    string
		encoded string
		want    string
	}{
		{"(A) Arabic", "egbpdaj6bu4bxfgehfvwxn", "ليهمابتكلموشعربي؟"},
		{"(B) Chinese simplified", "ihqwcrb4cv8a8dqg056pqjye", "他们为什么不说中文"},
		{"(C) Chinese traditional", "ihqwctvzc91f659drss3x8bo0yb", "他們爲什麽不說中文"},
		{"(D) Czech", "Proprostnemluvesky-uyb24dma41a", "Pročprostěnemluvíčesky"},
		{"(E) Hebrew", "4dbcagdahymbxekheh6e0a7fei0b", "למההםפשוטלאמדבריםעברית"},
		{"(L) Japanese", "3B-ww4c5e180e575a65lsy2b", "3年B組金八先生"},
		{"(O) ASCII only", "-> $1.00 <--", "-> $1.00 <-"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decodeLabel(tc.encoded)
			if !ok {
				t.Fatalf("decodeLabel(%q) reported invalid punycode", tc.encoded)
			}
			if got != tc.want {
				t.Errorf("decodeLabel(%q) = %q, want %q", tc.encoded, got, tc.want)
			}
		})
	}
}

func TestDecodeHost(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"xn--bcher-kva.de", "bücher.de"},
		{"www.xn--bcher-kva.de", "www.bücher.de"},
		{"xn--80ak6aa92e.com", "аррӏе.com"},
		{"example.com", "example.com"},
		// The prefix check ignores case, and RFC 3492 preserves the case of the
		// basic code points, so the decoded label keeps its capitals.
		{"XN--BCHER-KVA.DE", "BüCHER.DE"},
		{"xn--bcher-kva.xn--80ak6aa92e.com", "bücher.аррӏе.com"},
		{"", ""},
	}

	for _, tc := range cases {
		if got := DecodeHost(tc.host); got != tc.want {
			t.Errorf("DecodeHost(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

func TestDecodeLabelRejectsGarbage(t *testing.T) {
	cases := []struct {
		name string
		host string
	}{
		{"nothing after the prefix", "xn--"},
		{"punctuation is not base 36", "xn--!!!"},
		{"a digit that is not a valid code point", "xn--abc9999999999"},
		{"a non-ASCII byte in the basic part", "xn--é-encoded"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := decodeLabel(tc.host[4:]); ok {
				t.Errorf("decodeLabel(%q) accepted invalid punycode", tc.host[4:])
			}
			if got := DecodeHost(tc.host); got != tc.host {
				t.Errorf("DecodeHost(%q) = %q, want it left alone", tc.host, got)
			}
		})
	}
}
