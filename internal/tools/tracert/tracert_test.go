package tracert

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

func TestNormalizeDefaults(t *testing.T) {
	s, err := Params{Host: "8.8.8.8"}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	if s.Host != "8.8.8.8" {
		t.Errorf("host = %q, want 8.8.8.8", s.Host)
	}
	if s.MaxHops != 30 {
		t.Errorf("maxHops = %d, want 30", s.MaxHops)
	}
	if s.Queries != 3 {
		t.Errorf("queries = %d, want 3", s.Queries)
	}
	if s.TimeoutMS != 2000 {
		t.Errorf("timeoutMs = %d, want 2000", s.TimeoutMS)
	}
	if s.NoNames {
		t.Error("noNames should stay off by default")
	}
}

func TestNormalizeAccepts(t *testing.T) {
	cases := []struct {
		name   string
		params Params
		host   string
	}{
		{"trimmed", Params{Host: "  google.com  "}, "google.com"},
		{"trailing dot", Params{Host: "google.com."}, "google.com."},
		{"single label", Params{Host: "router"}, "router"},
		{"hyphenated label", Params{Host: "ae-1.border.example.net"}, "ae-1.border.example.net"},
		{"lowest limits", Params{Host: "8.8.8.8", MaxHops: 1, Queries: 1, TimeoutMS: 200}, "8.8.8.8"},
		{"highest limits", Params{Host: "8.8.8.8", MaxHops: 64, Queries: 5, TimeoutMS: 10000}, "8.8.8.8"},
		{"ipv6 literal is a valid address here", Params{Host: "2001:db8::1"}, "2001:db8::1"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := c.params.normalize()
			if err != nil {
				t.Fatalf("normalize() returned %v", err)
			}
			if s.Host != c.host {
				t.Errorf("host = %q, want %q", s.Host, c.host)
			}
		})
	}
}

func TestNormalizeRejects(t *testing.T) {
	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{"empty host", Params{}, "Enter a host name or IP address to trace"},
		{"whitespace host", Params{Host: "   "}, "Enter a host name or IP address to trace"},
		{"host too long", Params{Host: strings.Repeat("a", 300)}, "is not a host name or an IP address"},
		{"host with a space", Params{Host: "my router"}, "is not a host name or an IP address"},
		{"label too long", Params{Host: strings.Repeat("a", 64) + ".example.com"}, "is not a host name or an IP address"},
		{"label ending in a hyphen", Params{Host: "bad-.example.com"}, "is not a host name or an IP address"},
		{"empty label", Params{Host: "example..com"}, "is not a host name or an IP address"},
		{"underscore in a label", Params{Host: "bad_host.example.com"}, "is not a host name or an IP address"},
		{"negative hops", Params{Host: "8.8.8.8", MaxHops: -1}, "Follow between 1 and 64 hops"},
		{"too many hops", Params{Host: "8.8.8.8", MaxHops: 65}, "Follow between 1 and 64 hops"},
		{"negative probes", Params{Host: "8.8.8.8", Queries: -1}, "Send between 1 and 5 probes per hop"},
		{"too many probes", Params{Host: "8.8.8.8", Queries: 6}, "Send between 1 and 5 probes per hop"},
		{"wait too short", Params{Host: "8.8.8.8", TimeoutMS: 199}, "must be between 0.2 and 10 seconds"},
		{"wait too long", Params{Host: "8.8.8.8", TimeoutMS: 10001}, "must be between 0.2 and 10 seconds"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.params.normalize()
			if err == nil {
				t.Fatal("bad parameters were accepted")
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("message %q does not mention %q", err.Error(), c.want)
			}
		})
	}
}

func TestSummaryNote(t *testing.T) {
	cases := []struct {
		name     string
		hops     int
		answered int
		reached  bool
		want     string
	}{
		{
			name: "nothing on the path answered",
			want: "No router on the path answered. Some networks block traceroute completely, so this does not always mean the connection is broken.",
		},
		{
			name: "hops printed but every one silent",
			hops: 12,
			want: "No router on the path answered. Some networks block traceroute completely, so this does not always mean the connection is broken.",
		},
		{
			name:     "stopped short of the destination",
			hops:     12,
			answered: 8,
			want:     "The path stopped after 12 hops without reaching example.com. The last router that answered is the place to start looking.",
		},
		{
			name:     "destination reached",
			hops:     9,
			answered: 9,
			reached:  true,
			want:     "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := summaryNote("example.com", c.hops, c.answered, c.reached)
			if got != c.want {
				t.Errorf("summaryNote = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFinalHopDetection(t *testing.T) {
	cases := []struct {
		name        string
		hop         Hop
		destination string
		want        bool
	}{
		{"the destination itself", Hop{IP: "8.8.8.8"}, "8.8.8.8", true},
		{"a router on the way", Hop{IP: "10.0.0.1"}, "8.8.8.8", false},
		{"a hop nothing answered on", Hop{}, "8.8.8.8", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := finalHop(c.hop, c.destination); got != c.want {
				t.Errorf("finalHop = %v, want %v", got, c.want)
			}
		})
	}
}

func TestStartTraceRejectsBadInputWithoutStartingAJob(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartTrace(Params{Host: "my router"})
	if err == nil {
		t.Fatal("a host with a space in it was accepted")
	}
	if id != "" {
		t.Errorf("job id = %q, want empty", id)
	}
	if !strings.Contains(err.Error(), "is not a host name or an IP address") {
		t.Errorf("message %q does not explain the problem", err.Error())
	}
	if n := s.jobs.Running(); n != 0 {
		t.Errorf("%d jobs running, want 0", n)
	}
}

// TestTraceIsCancellable is the one that matters for the UI's Cancel button:
// the child process must die and the job must return a context error soon
// after Cancel, not when the trace would have finished on its own.
func TestTraceIsCancellable(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the system traceroute command")
	}
	if _, err := lookupTool(runtime.GOOS); err != nil {
		t.Skip(err.Error())
	}

	// TEST-NET-1, which never routes, so every hop times out and the trace
	// keeps running until it is stopped.
	set, err := Params{Host: "192.0.2.1", MaxHops: 30, TimeoutMS: 2000}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}

	m := core.NewJobManager()
	done := make(chan error, 1)
	id := m.Start(JobKind, set.MaxHops, func(jc *core.JobContext) error {
		err := trace(jc, set)
		done <- err
		return err
	})

	time.Sleep(300 * time.Millisecond)
	if err := m.Cancel(id); err != nil {
		t.Fatalf("Cancel returned %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled trace returned %v, want context.Canceled", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the trace kept running after it was cancelled")
	}
}
