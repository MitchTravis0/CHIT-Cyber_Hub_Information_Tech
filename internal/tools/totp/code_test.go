package totp

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// The RFC 6238 appendix B seeds. The RFC writes them as ASCII, repeated to the
// length each hash needs.
var (
	seedSHA1   = []byte("12345678901234567890")
	seedSHA256 = []byte("12345678901234567890123456789012")
	seedSHA512 = []byte("1234567890123456789012345678901234567890123456789012345678901234")
)

// TestComputeRFC6238 pins the code generator to the published vectors in RFC
// 6238 appendix B. These literals were confirmed against an independent
// implementation (python3 hashlib/hmac) before being written here, because a
// generator checked only against itself proves nothing.
func TestComputeRFC6238(t *testing.T) {
	tests := []struct {
		unix   int64
		sha1   string
		sha256 string
		sha512 string
	}{
		{59, "94287082", "46119246", "90693936"},
		{1111111109, "07081804", "68084774", "25091201"},
		{1111111111, "14050471", "67062674", "99943326"},
		{1234567890, "89005924", "91819424", "93441116"},
		{2000000000, "69279037", "90698825", "38618901"},
		{20000000000, "65353130", "77737706", "47863826"},
	}

	for _, tt := range tests {
		at := time.Unix(tt.unix, 0).UTC()
		for _, c := range []struct {
			algorithm string
			secret    []byte
			want      string
		}{
			{algoSHA1, seedSHA1, tt.sha1},
			{algoSHA256, seedSHA256, tt.sha256},
			{algoSHA512, seedSHA512, tt.sha512},
		} {
			t.Run(c.algorithm+"/"+at.Format(time.RFC3339), func(t *testing.T) {
				got, err := Compute(c.secret, at, 30, 8, c.algorithm)
				if err != nil {
					t.Fatalf("Compute: %v", err)
				}
				if got != c.want {
					t.Errorf("Compute at %d = %q, want %q", tt.unix, got, c.want)
				}
			})
		}
	}
}

// TestComputeSixDigits checks the truncation at the length everything actually
// uses. The expected values are the last six digits of the eight-digit vectors
// above, which is what RFC 4226 dynamic truncation produces.
func TestComputeSixDigits(t *testing.T) {
	tests := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1234567890, "005924"},
	}
	for _, tt := range tests {
		got, err := Compute(seedSHA1, time.Unix(tt.unix, 0).UTC(), 30, 6, algoSHA1)
		if err != nil {
			t.Fatalf("Compute: %v", err)
		}
		if got != tt.want {
			t.Errorf("Compute(%d, 6 digits) = %q, want %q", tt.unix, got, tt.want)
		}
	}
}

func TestComputeWindows(t *testing.T) {
	// 1111111109 and 1111111111 are on either side of a 30 second boundary
	// (1111111110), so they must differ, and two instants inside one window
	// must not.
	before, err := Compute(seedSHA1, time.Unix(1111111109, 0), 30, 8, algoSHA1)
	if err != nil {
		t.Fatal(err)
	}
	after, err := Compute(seedSHA1, time.Unix(1111111111, 0), 30, 8, algoSHA1)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatalf("codes either side of a period boundary are the same: %q", before)
	}

	sameWindow, err := Compute(seedSHA1, time.Unix(1111111129, 0), 30, 8, algoSHA1)
	if err != nil {
		t.Fatal(err)
	}
	if sameWindow != after {
		t.Errorf("two instants in one window gave %q and %q", after, sameWindow)
	}
}

func TestComputeRejects(t *testing.T) {
	at := time.Unix(59, 0)
	tests := []struct {
		name      string
		period    int
		digits    int
		algorithm string
	}{
		{"period below the minimum", 14, 6, algoSHA1},
		{"period above the maximum", 301, 6, algoSHA1},
		{"digits below the minimum", 30, 5, algoSHA1},
		{"digits above the maximum", 30, 9, algoSHA1},
		{"unknown algorithm", 30, 6, "MD5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Compute(seedSHA1, at, tt.period, tt.digits, tt.algorithm); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSecondsLeft(t *testing.T) {
	tests := []struct {
		unix   int64
		period int
		want   int
	}{
		{0, 30, 30},
		{1, 30, 29},
		{29, 30, 1},
		{30, 30, 30},
		{59, 30, 1},
		{0, 60, 60},
		{45, 60, 15},
	}
	for _, tt := range tests {
		if got := secondsLeft(time.Unix(tt.unix, 0), tt.period); got != tt.want {
			t.Errorf("secondsLeft(%d, %d) = %d, want %d", tt.unix, tt.period, got, tt.want)
		}
	}
}

func TestParseSecret(t *testing.T) {
	// Base32 of "12345678901234567890", from the same independent oracle.
	const seed1Base32 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{"the RFC seed", seed1Base32, hex.EncodeToString(seedSHA1), ""},
		{"lower case", strings.ToLower(seed1Base32), hex.EncodeToString(seedSHA1), ""},
		{"in groups of four", "GEZD GNBV GY3T QOJQ GEZD GNBV GY3T QOJQ", hex.EncodeToString(seedSHA1), ""},
		{"separated by dashes", "GEZD-GNBV-GY3T-QOJQ-GEZD-GNBV-GY3T-QOJQ", hex.EncodeToString(seedSHA1), ""},
		{"padding already there", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", hex.EncodeToString(seedSHA1), ""},
		{"sixteen characters, no padding needed", "MFRGGZDFMZTWQ2LK", "6162636465666768696a", ""},
		{"exactly ten characters", "MFRGGZDFMZ", "616263646566", ""},
		{"nine characters", "MFRGGZDFM", "", msgSecretShort},
		{"empty", "", "", msgSecretShort},
		{"contains 1", "MFRGGZDFM1", "", msgSecretFormat},
		{"contains 0", "MFRGGZDFM0", "", msgSecretFormat},
		{"contains 8", "MFRGGZDFM8", "", msgSecretFormat},
		{"impossible length", "MFRGGZDFMZT", "", msgSecretFormat},
		{"far too long", strings.Repeat("A", maxSecretChars+1), "", msgSecretLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSecret(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error, got %x", got)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("message = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSecret: %v", err)
			}
			if hex.EncodeToString(got) != tt.want {
				t.Errorf("parseSecret(%q) = %x, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeAlgorithm(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", algoSHA1, false},
		{"sha1", algoSHA1, false},
		{"SHA1", algoSHA1, false},
		{" sha256 ", algoSHA256, false},
		{"SHA512", algoSHA512, false},
		{"MD5", "", true},
		{"SHA-256", "", true},
	}
	for _, tt := range tests {
		got, err := normalizeAlgorithm(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("normalizeAlgorithm(%q) accepted it", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeAlgorithm(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("normalizeAlgorithm(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
