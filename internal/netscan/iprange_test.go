package netscan

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"chit/internal/core"
)

// TestParseRangeSharedCases runs the golden file that the frontend copy of this
// parser (frontend/src/tools/ip-range-scanner/range.ts, exercised by
// frontend/tests/range.test.ts) is held to as well. The grid the user sees is
// laid out from the frontend copy while the scan comes from this one, so the
// two drifting apart misaligns the grid or scans a range the UI called valid.
func TestParseRangeSharedCases(t *testing.T) {
	var golden struct {
		Cases []struct {
			Input string `json:"input"`
			OK    bool   `json:"ok"`
			Start string `json:"start"`
			End   string `json:"end"`
			Count int    `json:"count"`
			Text  string `json:"text"`
			Error string `json:"error"`
		} `json:"cases"`
	}

	raw, err := os.ReadFile("../../testdata/iprange-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Cases) < 50 {
		t.Fatalf("golden file holds only %d cases, it is probably truncated", len(golden.Cases))
	}

	for _, tc := range golden.Cases {
		t.Run(tc.Input, func(t *testing.T) {
			r, err := ParseRange(tc.Input)
			if !tc.OK {
				if err == nil {
					t.Fatalf("ParseRange(%q) = %+v, want error %q", tc.Input, r, tc.Error)
				}
				if msg := core.MessageOf(err); msg != tc.Error {
					t.Errorf("error = %q, want %q", msg, tc.Error)
				}
				if code := core.CodeOf(err); code != core.CodeInvalidInput {
					t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRange(%q) returned %v", tc.Input, err)
			}
			if got := r.Start.String(); got != tc.Start {
				t.Errorf("start = %s, want %s", got, tc.Start)
			}
			if got := r.End.String(); got != tc.End {
				t.Errorf("end = %s, want %s", got, tc.End)
			}
			if r.Count != tc.Count {
				t.Errorf("count = %d, want %d", r.Count, tc.Count)
			}
			if r.Text != tc.Text {
				t.Errorf("text = %q, want %q", r.Text, tc.Text)
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	cases := []struct {
		name  string
		input string
		start string
		end   string
		count int
		text  string
	}{
		{"single ip", "192.168.1.50", "192.168.1.50", "192.168.1.50", 1, "192.168.1.50"},
		{"single ip padded", "  10.0.0.1  ", "10.0.0.1", "10.0.0.1", 1, "10.0.0.1"},
		{"slash 24 drops network and broadcast", "192.168.1.0/24", "192.168.1.1", "192.168.1.254", 254, "192.168.1.0/24"},
		{"slash 24 from a host address", "192.168.1.77/24", "192.168.1.1", "192.168.1.254", 254, "192.168.1.0/24"},
		{"slash 30", "10.1.2.0/30", "10.1.2.1", "10.1.2.2", 2, "10.1.2.0/30"},
		{"slash 31 keeps both", "10.1.2.0/31", "10.1.2.0", "10.1.2.1", 2, "10.1.2.0/31"},
		{"slash 32 keeps one", "10.1.2.7/32", "10.1.2.7", "10.1.2.7", 1, "10.1.2.7/32"},
		{"slash 16 at the cap", "10.0.0.0/16", "10.0.0.1", "10.0.255.254", 65534, "10.0.0.0/16"},
		{"start end full", "192.168.1.1-192.168.1.255", "192.168.1.1", "192.168.1.255", 255, "192.168.1.1-192.168.1.255"},
		{"start end across octets", "10.0.0.250-10.0.1.5", "10.0.0.250", "10.0.1.5", 12, "10.0.0.250-10.0.1.5"},
		{"start end same", "10.0.0.5-10.0.0.5", "10.0.0.5", "10.0.0.5", 1, "10.0.0.5-10.0.0.5"},
		{"short end", "192.168.1.1-255", "192.168.1.1", "192.168.1.255", 255, "192.168.1.1-192.168.1.255"},
		{"short end zero", "192.168.1.0-0", "192.168.1.0", "192.168.1.0", 1, "192.168.1.0-192.168.1.0"},
		{"short end with spaces", "192.168.1.10 - 20", "192.168.1.10", "192.168.1.20", 11, "192.168.1.10-192.168.1.20"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ParseRange(tc.input)
			if err != nil {
				t.Fatalf("ParseRange(%q) returned %v", tc.input, err)
			}
			if got := r.Start.String(); got != tc.start {
				t.Errorf("start = %s, want %s", got, tc.start)
			}
			if got := r.End.String(); got != tc.end {
				t.Errorf("end = %s, want %s", got, tc.end)
			}
			if r.Count != tc.count {
				t.Errorf("count = %d, want %d", r.Count, tc.count)
			}
			if r.Text != tc.text {
				t.Errorf("text = %q, want %q", r.Text, tc.text)
			}
		})
	}
}

func TestParseRangeErrors(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"spaces only", "   "},
		{"not an address", "hello"},
		{"hostname", "server.local"},
		{"bad octet", "192.168.1.300"},
		{"too few octets", "192.168.1"},
		{"bad prefix", "192.168.1.0/33"},
		{"prefix without number", "192.168.1.0/"},
		{"ipv6 single", "fe80::1"},
		{"ipv6 cidr", "2001:db8::/64"},
		{"ipv4 mapped ipv6", "::ffff:192.168.1.1"},
		{"backwards", "192.168.1.100-192.168.1.10"},
		{"backwards short", "192.168.1.100-10"},
		{"missing end", "192.168.1.1-"},
		{"end out of range", "192.168.1.1-300"},
		{"negative end", "192.168.1.1--5"},
		{"end not a number", "192.168.1.1-abc"},
		{"partial end address", "10.0.0.1-1.20"},
		{"too big slash 8", "10.0.0.0/8"},
		{"too big slash 0", "0.0.0.0/0"},
		{"too big start end", "10.0.0.0-10.2.0.0"},
		{"mixed forms", "192.168.1.1-192.168.1.10/24"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRange(tc.input)
			if err == nil {
				t.Fatalf("ParseRange(%q) accepted an input it should reject", tc.input)
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
			}
			msg := core.MessageOf(err)
			if msg == "" || !strings.HasSuffix(strings.TrimSpace(msg), ".") {
				t.Errorf("message %q should be a written sentence", msg)
			}
		})
	}
}

func TestRangeAddresses(t *testing.T) {
	r, err := ParseRange("192.168.1.254-192.168.2.2")
	if err != nil {
		t.Fatal(err)
	}
	addrs := r.Addresses()
	if len(addrs) != r.Count {
		t.Fatalf("got %d addresses, want %d", len(addrs), r.Count)
	}
	want := []string{"192.168.1.254", "192.168.1.255", "192.168.2.0", "192.168.2.1", "192.168.2.2"}
	for i, w := range want {
		if addrs[i].String() != w {
			t.Errorf("address %d = %s, want %s", i, addrs[i], w)
		}
	}
}

func TestRangeAddressesLastAddressDoesNotWrap(t *testing.T) {
	r, err := ParseRange("255.255.255.250-255.255.255.255")
	if err != nil {
		t.Fatal(err)
	}
	addrs := r.Addresses()
	if len(addrs) != 6 {
		t.Fatalf("got %d addresses, want 6", len(addrs))
	}
	if last := addrs[len(addrs)-1].String(); last != "255.255.255.255" {
		t.Errorf("last address = %s, want 255.255.255.255", last)
	}
}

func TestRangeAddressesEmptyRange(t *testing.T) {
	if got := (Range{}).Addresses(); got != nil {
		t.Errorf("zero range produced %v, want nil", got)
	}
}

func TestRangeAddressesFullSlash24(t *testing.T) {
	r, err := ParseRange("10.20.30.0/24")
	if err != nil {
		t.Fatal(err)
	}
	addrs := r.Addresses()
	if len(addrs) != 254 {
		t.Fatalf("got %d addresses, want 254", len(addrs))
	}
	if addrs[0].String() != "10.20.30.1" || addrs[253].String() != "10.20.30.254" {
		t.Errorf("range runs %s..%s, want 10.20.30.1..10.20.30.254", addrs[0], addrs[253])
	}
}
