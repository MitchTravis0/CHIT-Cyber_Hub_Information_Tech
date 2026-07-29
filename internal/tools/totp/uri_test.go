package totp

import "testing"

func TestParseURI(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    parsedAccount
		wantErr string
	}{
		{
			name: "everything spelled out",
			in:   "otpauth://totp/Firewall:admin@head-office?secret=GEZDGNBVGY3TQOJQ&issuer=Firewall&algorithm=SHA256&digits=8&period=60",
			want: parsedAccount{Issuer: "Firewall", Label: "admin@head-office", Secret: "GEZDGNBVGY3TQOJQ", Digits: 8, Period: 60, Algorithm: algoSHA256},
		},
		{
			name: "only the secret, everything else defaulted",
			in:   "otpauth://totp/admin?secret=GEZDGNBVGY3TQOJQ",
			want: parsedAccount{Issuer: "", Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{
			name: "the issuer taken from the label prefix",
			in:   "otpauth://totp/Registrar:billing@example.com?secret=GEZDGNBVGY3TQOJQ",
			want: parsedAccount{Issuer: "Registrar", Label: "billing@example.com", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{
			name: "the issuer parameter wins over the label prefix",
			in:   "otpauth://totp/Old:admin?secret=GEZDGNBVGY3TQOJQ&issuer=New",
			want: parsedAccount{Issuer: "New", Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{
			name: "percent-encoded issuer and label",
			in:   "otpauth://totp/Head%20Office:admin%20user?secret=GEZDGNBVGY3TQOJQ&issuer=Head%20Office",
			want: parsedAccount{Issuer: "Head Office", Label: "admin user", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{
			name: "spaces around the whole link",
			in:   "  otpauth://totp/admin?secret=GEZDGNBVGY3TQOJQ  ",
			want: parsedAccount{Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{
			name: "an upper case scheme",
			in:   "OTPAUTH://totp/admin?secret=GEZDGNBVGY3TQOJQ",
			want: parsedAccount{Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},

		{name: "a counter based link", in: "otpauth://hotp/admin?secret=GEZDGNBVGY3TQOJQ&counter=1", wantErr: msgHOTP},
		{name: "not an otpauth link", in: "https://example.com/?secret=GEZDGNBVGY3TQOJQ", wantErr: msgNotOtpauth},
		{name: "an unknown otpauth type", in: "otpauth://steam/admin?secret=GEZDGNBVGY3TQOJQ", wantErr: msgNotOtpauth},
		{name: "empty", in: "", wantErr: msgNotOtpauth},
		{name: "just the word", in: "otpauth", wantErr: msgNotOtpauth},
		{name: "no secret", in: "otpauth://totp/Firewall:admin?issuer=Firewall", wantErr: msgNoSecretInURI},
		{name: "an empty secret", in: "otpauth://totp/admin?secret=", wantErr: msgNoSecretInURI},
		{name: "five digits", in: "otpauth://totp/admin?secret=GEZDGNBVGY3TQOJQ&digits=5", wantErr: "That account asks for 5 digit codes. CHIT handles 6, 7 and 8."},
		{name: "nine digits", in: "otpauth://totp/admin?secret=GEZDGNBVGY3TQOJQ&digits=9", wantErr: "That account asks for 9 digit codes. CHIT handles 6, 7 and 8."},
		{name: "digits that are not a number", in: "otpauth://totp/admin?secret=GEZDGNBVGY3TQOJQ&digits=six", wantErr: "That account asks for six digit codes. CHIT handles 6, 7 and 8."},
		{name: "a ten second period", in: "otpauth://totp/admin?secret=GEZDGNBVGY3TQOJQ&period=10", wantErr: "That account asks for a 10 second code. CHIT handles 15 to 300 seconds, and almost everything uses 30."},
		{name: "a four hundred second period", in: "otpauth://totp/admin?secret=GEZDGNBVGY3TQOJQ&period=400", wantErr: "That account asks for a 400 second code. CHIT handles 15 to 300 seconds, and almost everything uses 30."},
		{name: "an unknown algorithm", in: "otpauth://totp/admin?secret=GEZDGNBVGY3TQOJQ&algorithm=MD5", wantErr: "That account uses MD5. CHIT handles SHA1, SHA256 and SHA512."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseURI(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error, got %+v", got)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("message = %q,\nwant %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseURI: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseURI(%q) =\n%+v\nwant\n%+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseURIBoundaries(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want parsedAccount
	}{
		{"six digits", "otpauth://totp/a?secret=GEZDGNBVGY3TQOJQ&digits=6", parsedAccount{Label: "a", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1}},
		{"eight digits", "otpauth://totp/a?secret=GEZDGNBVGY3TQOJQ&digits=8", parsedAccount{Label: "a", Secret: "GEZDGNBVGY3TQOJQ", Digits: 8, Period: 30, Algorithm: algoSHA1}},
		{"fifteen seconds", "otpauth://totp/a?secret=GEZDGNBVGY3TQOJQ&period=15", parsedAccount{Label: "a", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 15, Algorithm: algoSHA1}},
		{"three hundred seconds", "otpauth://totp/a?secret=GEZDGNBVGY3TQOJQ&period=300", parsedAccount{Label: "a", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 300, Algorithm: algoSHA1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseURI(tt.in)
			if err != nil {
				t.Fatalf("parseURI: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDraftFrom(t *testing.T) {
	tests := []struct {
		name    string
		in      NewAccount
		want    parsedAccount
		wantErr string
	}{
		{
			name: "a link is used when both are filled in",
			in:   NewAccount{URI: "otpauth://totp/Firewall:admin?secret=GEZDGNBVGY3TQOJQ", Secret: "MFRGGZDFMZ", Issuer: "Ignored"},
			want: parsedAccount{Issuer: "Firewall", Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{
			name: "a typed secret with an issuer and a label",
			in:   NewAccount{Secret: " GEZDGNBVGY3TQOJQ ", Issuer: " Firewall ", Label: " admin "},
			want: parsedAccount{Issuer: "Firewall", Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{
			name: "a typed secret with only an issuer",
			in:   NewAccount{Secret: "GEZDGNBVGY3TQOJQ", Issuer: "Firewall"},
			want: parsedAccount{Issuer: "Firewall", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{
			name: "a typed secret with only a label",
			in:   NewAccount{Secret: "GEZDGNBVGY3TQOJQ", Label: "admin"},
			want: parsedAccount{Label: "admin", Secret: "GEZDGNBVGY3TQOJQ", Digits: 6, Period: 30, Algorithm: algoSHA1},
		},
		{name: "nothing at all", in: NewAccount{}, wantErr: msgNothingToAdd},
		{name: "only whitespace", in: NewAccount{URI: "   ", Secret: "  "}, wantErr: msgNothingToAdd},
		{name: "a secret with no name", in: NewAccount{Secret: "GEZDGNBVGY3TQOJQ"}, wantErr: msgNoName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := draftFrom(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error, got %+v", got)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("message = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("draftFrom: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
