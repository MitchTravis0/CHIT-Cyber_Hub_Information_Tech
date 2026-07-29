package portscan

import (
	"strings"
	"testing"

	"chit/internal/core"
)

func TestParsePorts(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want []int
	}{
		{"one port", "80", []int{80}},
		{"a list", "80,443", []int{80, 443}},
		{"a list out of order is sorted", "443,80", []int{80, 443}},
		{"space separated", "80 443", []int{80, 443}},
		{"padding is ignored", " 80 , 443 ", []int{80, 443}},
		{"a small range", "1-5", []int{1, 2, 3, 4, 5}},
		{"duplicates are dropped", "80,80,443", []int{80, 443}},
		{"a list and a range together", "80,8000-8002", []int{80, 8000, 8001, 8002}},
		{"the highest port", "65535", []int{65535}},
		{"a range of one", "80-80", []int{80}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parsePorts(c.spec)
			if err != nil {
				t.Fatalf("parsePorts(%q) returned %v", c.spec, err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("parsePorts(%q) = %v, want %v", c.spec, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("parsePorts(%q) = %v, want %v", c.spec, got, c.want)
				}
			}
		})
	}
}

func TestParsePortsCounts(t *testing.T) {
	cases := []struct {
		spec  string
		want  int
		first int
		last  int
	}{
		{"1-1024", 1024, 1, 1024},
		{"1-65535", 65535, 1, 65535},
	}

	for _, c := range cases {
		t.Run(c.spec, func(t *testing.T) {
			got, err := parsePorts(c.spec)
			if err != nil {
				t.Fatalf("parsePorts(%q) returned %v", c.spec, err)
			}
			if len(got) != c.want {
				t.Fatalf("parsePorts(%q) gave %d ports, want %d", c.spec, len(got), c.want)
			}
			if got[0] != c.first || got[len(got)-1] != c.last {
				t.Fatalf("parsePorts(%q) runs %d..%d, want %d..%d",
					c.spec, got[0], got[len(got)-1], c.first, c.last)
			}
		})
	}
}

func TestParsePortsRejects(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want string
	}{
		{"empty", "", "Enter at least one port to check"},
		{"whitespace only", "   ", "Enter at least one port to check"},
		{"a lone comma", ",", "Enter at least one port to check"},
		{"letters", "abc", "is not a port or a port range"},
		{"a letter in a number", "8o", "is not a port or a port range"},
		{"zero", "0", "Ports run from 1 to 65535, so 0 is not one."},
		{"one past the maximum", "65536", "Ports run from 1 to 65535, so 65536 is not one."},
		{"a leading zero", "080", "Ports run from 1 to 65535, so 080 is not one."},
		{"a signed number", "+80", "Ports run from 1 to 65535, so +80 is not one."},
		{"a missing start", "-80", "is not a port or a port range"},
		{"a missing end", "80-", "is not a port or a port range"},
		{"backwards", "443-80", "The range 443-80 runs backwards."},
		{"a range past the maximum", "1-70000", "Ports run from 1 to 65535, so 70000 is not one."},
		{"a good port and a bad one", "80,abc", `"abc" is not a port or a port range`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parsePorts(c.spec)
			if err == nil {
				t.Fatalf("parsePorts(%q) accepted it and gave %v", c.spec, got)
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("message %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}
