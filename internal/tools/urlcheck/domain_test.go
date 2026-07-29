package urlcheck

import (
	"strings"
	"testing"
)

func TestRegistrable(t *testing.T) {
	// Every entry in the list gets a case, so an added suffix cannot slip in
	// untested.
	for suffix := range multiSuffixes {
		host := "www.example." + suffix
		want := "example." + suffix
		if got := Registrable(host); got != want {
			t.Errorf("Registrable(%q) = %q, want %q", host, got, want)
		}
	}

	cases := []struct {
		host string
		want string
	}{
		{"example.com", "example.com"},
		{"www.example.com", "example.com"},
		{"a.b.c.example.com", "example.com"},
		{"localhost", "localhost"},
		{"co.uk", "co.uk"},
		{"example.co.uk.", "example.co.uk"},
		{"EXAMPLE.COM", "example.com"},
		{"192.168.1.1", "192.168.1.1"},
		{"2606:2800:220:1::1", "2606:2800:220:1::1"},
	}
	for _, tc := range cases {
		if got := Registrable(tc.host); got != tc.want {
			t.Errorf("Registrable(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

func TestComparableDomain(t *testing.T) {
	for suffix := range hostingSuffixes {
		evil, good := ComparableDomain("evil."+suffix), ComparableDomain("good."+suffix)
		if evil != "evil."+suffix {
			t.Errorf("ComparableDomain(%q) = %q, want %q", "evil."+suffix, evil, "evil."+suffix)
		}
		if evil == good {
			t.Errorf("%q and %q compare as the same owner", "evil."+suffix, "good."+suffix)
		}
	}

	cases := []struct {
		host string
		want string
	}{
		{"example.com", "example.com"},
		{"github.io", "github.io"},
		{"www.example.co.uk", "example.co.uk"},
	}
	for _, tc := range cases {
		if got := ComparableDomain(tc.host); got != tc.want {
			t.Errorf("ComparableDomain(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

func TestIsShortener(t *testing.T) {
	for host := range shorteners {
		if !IsShortener(host) {
			t.Errorf("IsShortener(%q) = false, want true", host)
		}
		if withWWW := "www." + host; !IsShortener(withWWW) {
			t.Errorf("IsShortener(%q) = false, want true", withWWW)
		}
	}

	for _, host := range []string{"example.com", "notbit.ly", "", "bit.ly.example.com"} {
		if IsShortener(host) {
			t.Errorf("IsShortener(%q) = true, want false", host)
		}
	}
}

// The three lists are typed by hand from the spec, so a duplicated or mistyped
// entry is worth catching here rather than in a report a tech is reading.
func TestSuffixListsAreWellFormed(t *testing.T) {
	for suffix := range multiSuffixes {
		if strings.Count(suffix, ".") != 1 {
			t.Errorf("multiSuffixes entry %q is not two labels", suffix)
		}
	}
	for suffix := range hostingSuffixes {
		if strings.Count(suffix, ".") < 1 {
			t.Errorf("hostingSuffixes entry %q is not a domain", suffix)
		}
	}
	for suffix := range shorteners {
		if strings.Count(suffix, ".") != 1 {
			t.Errorf("shorteners entry %q is not two labels", suffix)
		}
	}
}
