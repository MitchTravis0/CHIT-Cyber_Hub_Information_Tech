package tracert

import (
	"reflect"
	"strings"
	"testing"

	"chit/internal/core"
)

func TestBuildArgs(t *testing.T) {
	cases := []struct {
		name string
		goos string
		set  settings
		want []string
	}{
		{
			name: "windows defaults",
			goos: "windows",
			set:  settings{MaxHops: 30, Queries: 3, TimeoutMS: 2000},
			want: []string{"-h", "30", "-w", "2000", "8.8.8.8"},
		},
		{
			name: "windows without names",
			goos: "windows",
			set:  settings{MaxHops: 30, Queries: 3, TimeoutMS: 2000, NoNames: true},
			want: []string{"-h", "30", "-w", "2000", "-d", "8.8.8.8"},
		},
		{
			name: "linux defaults",
			goos: "linux",
			set:  settings{MaxHops: 30, Queries: 3, TimeoutMS: 2000},
			want: []string{"-m", "30", "-q", "3", "-w", "2", "8.8.8.8"},
		},
		{
			name: "darwin matches linux",
			goos: "darwin",
			set:  settings{MaxHops: 30, Queries: 3, TimeoutMS: 2000},
			want: []string{"-m", "30", "-q", "3", "-w", "2", "8.8.8.8"},
		},
		{
			name: "one second is one second",
			goos: "linux",
			set:  settings{MaxHops: 15, Queries: 1, TimeoutMS: 1000},
			want: []string{"-m", "15", "-q", "1", "-w", "1", "8.8.8.8"},
		},
		{
			name: "a second and a half rounds up",
			goos: "linux",
			set:  settings{MaxHops: 30, Queries: 3, TimeoutMS: 1500},
			want: []string{"-m", "30", "-q", "3", "-w", "2", "8.8.8.8"},
		},
		{
			name: "the shortest wait still asks for a whole second",
			goos: "linux",
			set:  settings{MaxHops: 30, Queries: 3, TimeoutMS: 200},
			want: []string{"-m", "30", "-q", "3", "-w", "1", "8.8.8.8"},
		},
		{
			name: "linux without names",
			goos: "linux",
			set:  settings{MaxHops: 64, Queries: 5, TimeoutMS: 4000, NoNames: true},
			want: []string{"-m", "64", "-q", "5", "-w", "4", "-n", "8.8.8.8"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildArgs(c.goos, "8.8.8.8", c.set)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("buildArgs(%q) = %v, want %v", c.goos, got, c.want)
			}
		})
	}
}

// TestLookupToolMessages checks the wording a tech sees when the system tool is
// missing. Whether it is actually installed depends on the machine, so that is
// deliberately not asserted.
func TestLookupToolMessages(t *testing.T) {
	cases := []struct {
		goos string
		want string
	}{
		{"windows", "This computer does not have the tracert command, which is unusual for Windows. CHIT cannot follow the path without it."},
		{"darwin", "This Mac does not have the traceroute command, so CHIT cannot follow the path."},
		{"linux", "This computer does not have the traceroute command installed, so CHIT cannot follow the path. On Ubuntu or Debian install it with: sudo apt install traceroute. On Fedora or RHEL: sudo dnf install traceroute. On Arch: sudo pacman -S traceroute."},
	}

	for _, c := range cases {
		t.Run(c.goos, func(t *testing.T) {
			err := toolMissing(c.goos)
			if code := core.CodeOf(err); code != core.CodeNotFound {
				t.Errorf("code = %s, want %s", code, core.CodeNotFound)
			}
			if err.Error() != c.want {
				t.Errorf("message = %q, want %q", err.Error(), c.want)
			}
		})
	}
}

func TestToolCandidates(t *testing.T) {
	if got := toolCandidates("windows"); !strings.HasSuffix(got[0], "tracert.exe") || got[1] != "tracert" {
		t.Errorf("windows candidates = %v", got)
	}
	if got := toolCandidates("darwin"); got[0] != "/usr/sbin/traceroute" || got[1] != "traceroute" {
		t.Errorf("darwin candidates = %v", got)
	}
	// /usr/sbin is listed because an app started from a desktop icon does not
	// always inherit a login shell's PATH.
	want := []string{"traceroute", "/usr/sbin/traceroute", "/usr/bin/traceroute"}
	if got := toolCandidates("linux"); !reflect.DeepEqual(got, want) {
		t.Errorf("linux candidates = %v, want %v", got, want)
	}
}
