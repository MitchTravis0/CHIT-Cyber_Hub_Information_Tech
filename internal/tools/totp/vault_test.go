package totp

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// TestPBKDF2SHA256 pins the key derivation to the published PBKDF2-HMAC-SHA256
// vectors in RFC 7914 section 11. These literals were confirmed against two
// independent implementations (python3 hashlib.pbkdf2_hmac and openssl kdf)
// before being written here.
func TestPBKDF2SHA256(t *testing.T) {
	tests := []struct {
		name       string
		password   string
		salt       string
		iterations int
		length     int
		want       string
	}{
		{
			name: "one iteration", password: "passwd", salt: "salt", iterations: 1, length: 64,
			want: "55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc" +
				"49ca9cccf179b645991664b39d77ef317c71b845b1e30bd509112041d3a19783",
		},
		{
			name: "eighty thousand iterations", password: "Password", salt: "NaCl", iterations: 80000, length: 64,
			want: "4ddcd8f60b98be21830cee5ef22701f9641a4418d04c0414aeff08876b34ab56" +
				"a1d425a1225833549adb841b51c9b3176a272bdebba1d078478f62b397f33c8d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pbkdf2SHA256([]byte(tt.password), []byte(tt.salt), tt.iterations, tt.length)
			if hex.EncodeToString(got) != tt.want {
				t.Errorf("pbkdf2SHA256 = %x\nwant %s", got, tt.want)
			}
		})
	}
}

// TestPBKDF2Length checks the block loop, which only runs more than once when
// the wanted length is longer than one SHA-256 digest.
func TestPBKDF2Length(t *testing.T) {
	full := pbkdf2SHA256([]byte("passwd"), []byte("salt"), 1, 64)
	for _, length := range []int{1, 31, 32, 33, 63, 64} {
		got := pbkdf2SHA256([]byte("passwd"), []byte("salt"), 1, length)
		if len(got) != length {
			t.Fatalf("length %d gave %d bytes", length, len(got))
		}
		// A shorter key is the prefix of a longer one, by construction.
		if hex.EncodeToString(got) != hex.EncodeToString(full[:length]) {
			t.Errorf("length %d is not the prefix of the 64 byte key", length)
		}
	}
}

func TestPBKDF2IterationsMatter(t *testing.T) {
	one := pbkdf2SHA256([]byte("passwd"), []byte("salt"), 1, 32)
	two := pbkdf2SHA256([]byte("passwd"), []byte("salt"), 2, 32)
	if hex.EncodeToString(one) == hex.EncodeToString(two) {
		t.Fatal("one and two iterations gave the same key, so the loop is not running")
	}
}

func sealForTest(t *testing.T, passphrase string, iterations int, accounts []secretAccount) vaultDoc {
	t.Helper()
	salt := []byte("0123456789abcdef")
	key := pbkdf2SHA256([]byte(passphrase), salt, iterations, keyLen)
	doc, err := sealVault(key, salt, iterations, plaintext{Accounts: accounts})
	if err != nil {
		t.Fatalf("sealVault: %v", err)
	}
	return doc
}

// TestVaultRoundTripAtShippingCost runs once at the real 600,000 iterations, so
// the constant the app ships with is exercised and not only the cheap one the
// rest of the suite uses.
func TestVaultRoundTripAtShippingCost(t *testing.T) {
	// The literal is deliberate: asserting against kdfIterations would only prove
	// the constant equals itself, and a lowered work factor would ship silently.
	const shipping = 600000
	if kdfIterations != shipping {
		t.Fatalf("the shipping work factor is %d, and this test expects %d. If that was on purpose, change the literal here too.", kdfIterations, shipping)
	}
	doc := sealForTest(t, "a passphrase for the vault", kdfIterations, []secretAccount{
		{ID: "otp-1", Issuer: "Firewall", Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
	})
	if doc.Iterations != shipping {
		t.Fatalf("Iterations = %d, want %d", doc.Iterations, shipping)
	}
	_, _, pt, err := openVault(doc, "a passphrase for the vault")
	if err != nil {
		t.Fatalf("openVault: %v", err)
	}
	if len(pt.Accounts) != 1 || pt.Accounts[0].Secret != "GEZDGNBVGY3TQOJQ" {
		t.Fatalf("round trip lost the account: %+v", pt.Accounts)
	}
}

func TestVaultRoundTrip(t *testing.T) {
	const pass = "a passphrase for the vault"

	t.Run("empty vault", func(t *testing.T) {
		doc := sealForTest(t, pass, 2000, []secretAccount{})
		_, _, pt, err := openVault(doc, pass)
		if err != nil {
			t.Fatalf("openVault: %v", err)
		}
		if len(pt.Accounts) != 0 {
			t.Fatalf("expected no accounts, got %d", len(pt.Accounts))
		}
	})

	t.Run("three accounts", func(t *testing.T) {
		want := []secretAccount{
			{ID: "otp-1", Issuer: "Firewall", Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
			{ID: "otp-2", Issuer: "Registrar", Label: "billing@example.com", Secret: "MFRGGZDFMZTWQ2LK", Digits: 8, Period: 60, Algorithm: algoSHA256},
			{ID: "otp-3", Issuer: "", Label: "spare", Secret: "MFRGGZDFMZ", Digits: 6, Period: 30, Algorithm: algoSHA512},
		}
		doc := sealForTest(t, pass, 2000, want)
		_, _, pt, err := openVault(doc, pass)
		if err != nil {
			t.Fatalf("openVault: %v", err)
		}
		if len(pt.Accounts) != len(want) {
			t.Fatalf("got %d accounts, want %d", len(pt.Accounts), len(want))
		}
		for i := range want {
			if pt.Accounts[i] != want[i] {
				t.Errorf("account %d = %+v, want %+v", i, pt.Accounts[i], want[i])
			}
		}
	})

	t.Run("wrong passphrase", func(t *testing.T) {
		doc := sealForTest(t, pass, 2000, []secretAccount{})
		_, _, _, err := openVault(doc, pass+"!")
		if err == nil {
			t.Fatal("the wrong passphrase opened the vault")
		}
		if err.Error() != msgWrongPassphrase {
			t.Errorf("message = %q, want %q", err.Error(), msgWrongPassphrase)
		}
	})

	t.Run("a fresh nonce every write", func(t *testing.T) {
		first := sealForTest(t, pass, 2000, []secretAccount{})
		second := sealForTest(t, pass, 2000, []secretAccount{})
		if first.Nonce == second.Nonce {
			t.Fatal("two writes reused the nonce")
		}
		if first.Ciphertext == second.Ciphertext {
			t.Fatal("the same plaintext encrypted twice produced the same bytes")
		}
	})
}

// flipByte returns base64 data with one bit changed in the byte at index i.
func flipByte(t *testing.T, encoded string, i int) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw[i] ^= 0x01
	return base64.StdEncoding.EncodeToString(raw)
}

func TestVaultTamper(t *testing.T) {
	const pass = "a passphrase for the vault"
	accounts := []secretAccount{{ID: "otp-1", Issuer: "Firewall", Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1}}

	tests := []struct {
		name string
		edit func(vaultDoc) vaultDoc
		want string
	}{
		{
			name: "a bit flipped in the ciphertext",
			edit: func(d vaultDoc) vaultDoc { d.Ciphertext = flipByte(t, d.Ciphertext, 0); return d },
			want: msgWrongPassphrase,
		},
		{
			name: "a bit flipped in the nonce",
			edit: func(d vaultDoc) vaultDoc { d.Nonce = flipByte(t, d.Nonce, 0); return d },
			want: msgWrongPassphrase,
		},
		{
			name: "the iteration count edited",
			edit: func(d vaultDoc) vaultDoc { d.Iterations = 2001; return d },
			want: msgWrongPassphrase,
		},
		{
			name: "the salt edited",
			edit: func(d vaultDoc) vaultDoc { d.Salt = flipByte(t, d.Salt, 0); return d },
			want: msgWrongPassphrase,
		},
		{
			name: "the ciphertext truncated",
			edit: func(d vaultDoc) vaultDoc {
				raw, err := base64.StdEncoding.DecodeString(d.Ciphertext)
				if err != nil {
					t.Fatal(err)
				}
				d.Ciphertext = base64.StdEncoding.EncodeToString(raw[:len(raw)-4])
				return d
			},
			want: msgWrongPassphrase,
		},
		{
			name: "the kdf renamed",
			edit: func(d vaultDoc) vaultDoc { d.KDF = "scrypt"; return d },
			want: msgTampered,
		},
		{
			name: "the salt replaced with a shorter one",
			edit: func(d vaultDoc) vaultDoc {
				d.Salt = base64.StdEncoding.EncodeToString([]byte("short"))
				return d
			},
			want: msgTampered,
		},
		{
			name: "the ciphertext replaced with something that is not base64",
			edit: func(d vaultDoc) vaultDoc { d.Ciphertext = "not base64 at all!!"; return d },
			want: msgTampered,
		},
		{
			name: "a version from the future",
			edit: func(d vaultDoc) vaultDoc { d.Version = docVersion + 1; return d },
			want: msgNewerVault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.edit(sealForTest(t, pass, 2000, accounts))
			_, _, _, err := openVault(doc, pass)
			if err == nil {
				t.Fatal("the edited vault opened")
			}
			if err.Error() != tt.want {
				t.Errorf("message = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestVaultHeaderLimits(t *testing.T) {
	const pass = "a passphrase for the vault"

	// Literals, not minIterations and maxIterations: a test that reads the bound
	// it is checking would accept any bound at all.
	tests := []struct {
		name       string
		iterations int
		ok         bool
	}{
		{"zero", 0, false},
		{"one below the minimum", 999, false},
		{"exactly the minimum", 1000, true},
		{"exactly the maximum", 10000000, true},
		{"one above the maximum", 10000001, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sealing at the real maximum would take minutes, so the header is
			// edited rather than derived. The point under test is the range
			// check, which runs before any key derivation.
			doc := sealForTest(t, pass, 2000, []secretAccount{})
			doc.Iterations = tt.iterations

			_, _, _, err := openVault(doc, pass)
			if tt.ok {
				// Inside the range it fails authentication instead, because the
				// AAD no longer matches, and that is a different message.
				if err == nil {
					t.Fatal("an edited iteration count opened the vault")
				}
				if err.Error() != msgWrongPassphrase {
					t.Errorf("message = %q, want the authentication failure %q", err.Error(), msgWrongPassphrase)
				}
				return
			}
			if err == nil {
				t.Fatal("an out-of-range iteration count was accepted")
			}
			if err.Error() != msgTampered {
				t.Errorf("message = %q, want %q", err.Error(), msgTampered)
			}
		})
	}
}

// TestSealedDocCarriesNoPlaintext is the property that makes exporting a vault
// safe: everything outside the ciphertext has to be useless to a reader.
func TestSealedDocCarriesNoPlaintext(t *testing.T) {
	doc := sealForTest(t, "a passphrase for the vault", 2000, []secretAccount{
		{ID: "otp-1", Issuer: "Firewall", Label: "admin@head-office", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
	})
	whole := doc.KDF + doc.Salt + doc.Nonce + doc.Ciphertext
	for _, leak := range []string{"GEZDGNBVGY3TQOJQ", "Firewall", "admin@head-office", "a passphrase for the vault"} {
		if strings.Contains(whole, leak) {
			t.Errorf("the sealed document contains %q in the clear", leak)
		}
	}
}
