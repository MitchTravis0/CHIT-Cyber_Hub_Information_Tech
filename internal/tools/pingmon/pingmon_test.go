package pingmon

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

func TestNormalizeDefaults(t *testing.T) {
	cfg, err := Params{Targets: []string{"1.2.3.4"}}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	if cfg.Interval != time.Second {
		t.Errorf("interval = %s, want 1s", cfg.Interval)
	}
	if cfg.Timeout != time.Second || cfg.TimeoutMS != 1000 {
		t.Errorf("timeout = %s (%d ms), want 1s", cfg.Timeout, cfg.TimeoutMS)
	}
	if cfg.Rounds != 0 {
		t.Errorf("rounds = %d, want 0 (run until stopped)", cfg.Rounds)
	}
	if cfg.TCPPort != 443 {
		t.Errorf("tcp port = %d, want 443", cfg.TCPPort)
	}
	if cfg.SkipTCP {
		t.Error("the TCP fallback is what keeps the tool useful where ping is blocked, it must stay on")
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0] != "1.2.3.4" {
		t.Errorf("targets = %v, want [1.2.3.4]", cfg.Targets)
	}
}

func TestNormalizeTargets(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"trimmed and de-duplicated", []string{" 1.2.3.4 ", "1.2.3.4"}, []string{"1.2.3.4"}},
		{"case insensitive duplicates", []string{"A.com", "a.com"}, []string{"A.com"}},
		{"blanks dropped", []string{"1.2.3.4", "  ", ""}, []string{"1.2.3.4"}},
		{"order kept", []string{"b.com", "a.com"}, []string{"b.com", "a.com"}},
		{"the maximum is accepted", []string{"a.com", "b.com", "c.com", "d.com"},
			[]string{"a.com", "b.com", "c.com", "d.com"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := Params{Targets: c.in}.normalize()
			if err != nil {
				t.Fatalf("normalize() returned %v", err)
			}
			if strings.Join(cfg.Targets, ",") != strings.Join(c.want, ",") {
				t.Errorf("targets = %v, want %v", cfg.Targets, c.want)
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
		{"no targets", Params{}, "Enter at least one host to ping"},
		{"only blanks", Params{Targets: []string{"", "  "}}, "Enter at least one host to ping"},
		{"five targets", Params{Targets: []string{"a.com", "b.com", "c.com", "d.com", "e.com"}},
			"Watch at most 4 hosts at a time. You gave 5."},
		{"space in a target", Params{Targets: []string{"my printer"}},
			`"my printer" is not a host name or an IP address.`},
		{"target too long", Params{Targets: []string{strings.Repeat("a", 300)}},
			"is not a host name or an IP address"},
		{"empty label", Params{Targets: []string{"a..b"}}, "is not a host name or an IP address"},
		{"label starts with a dash", Params{Targets: []string{"-bad.example.com"}},
			"is not a host name or an IP address"},
		{"label ends with a dash", Params{Targets: []string{"bad-.example.com"}},
			"is not a host name or an IP address"},
		{"interval too short", Params{Targets: []string{"1.2.3.4"}, IntervalMS: 199},
			"Ping every 0.2 to 60 seconds. 199 ms is outside that."},
		{"interval too long", Params{Targets: []string{"1.2.3.4"}, IntervalMS: 60001},
			"Ping every 0.2 to 60 seconds. 60001 ms is outside that."},
		{"timeout too short", Params{Targets: []string{"1.2.3.4"}, TimeoutMS: 99},
			"The wait for a reply must be between 0.1 and 10 seconds. 99 ms is outside that."},
		{"timeout too long", Params{Targets: []string{"1.2.3.4"}, TimeoutMS: 10001},
			"The wait for a reply must be between 0.1 and 10 seconds. 10001 ms is outside that."},
		{"negative rounds", Params{Targets: []string{"1.2.3.4"}, Rounds: -1},
			"Stop after at most 43200 pings, which is twelve hours at one a second."},
		{"too many rounds", Params{Targets: []string{"1.2.3.4"}, Rounds: 43201},
			"Stop after at most 43200 pings, which is twelve hours at one a second."},
		{"negative port", Params{Targets: []string{"1.2.3.4"}, TCPPort: -1},
			"-1 is not a port number. Ports run from 1 to 65535."},
		{"port too high", Params{Targets: []string{"1.2.3.4"}, TCPPort: 65536},
			"65536 is not a port number. Ports run from 1 to 65535."},
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
				t.Errorf("message %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestNormalizeAcceptsAddresses(t *testing.T) {
	for _, addr := range []string{"192.168.1.1", "::1", "2001:db8::1", "a", "example-1.co.uk"} {
		t.Run(addr, func(t *testing.T) {
			cfg, err := Params{Targets: []string{addr}}.normalize()
			if err != nil {
				t.Fatalf("normalize(%q) returned %v", addr, err)
			}
			if cfg.Targets[0] != addr {
				t.Errorf("target = %q, want %q", cfg.Targets[0], addr)
			}
		})
	}
}

func TestNormalizeBoundaries(t *testing.T) {
	cfg, err := Params{
		Targets:    []string{"1.2.3.4"},
		IntervalMS: minIntervalMS,
		TimeoutMS:  minTimeoutMS,
		Rounds:     MaxRounds,
		TCPPort:    65535,
	}.normalize()
	if err != nil {
		t.Fatalf("the lowest and highest allowed values were rejected: %v", err)
	}
	if cfg.Interval != 200*time.Millisecond || cfg.Timeout != 100*time.Millisecond {
		t.Errorf("interval = %s, timeout = %s", cfg.Interval, cfg.Timeout)
	}
	if cfg.Rounds != MaxRounds || cfg.TCPPort != 65535 {
		t.Errorf("rounds = %d, port = %d", cfg.Rounds, cfg.TCPPort)
	}

	cfg, err = Params{Targets: []string{"1.2.3.4"}, IntervalMS: maxIntervalMS, TimeoutMS: maxTimeoutMS}.normalize()
	if err != nil {
		t.Fatalf("the highest interval and timeout were rejected: %v", err)
	}
	if cfg.Interval != 60*time.Second || cfg.Timeout != 10*time.Second {
		t.Errorf("interval = %s, timeout = %s", cfg.Interval, cfg.Timeout)
	}
}

func TestSummaryNote(t *testing.T) {
	cases := []struct {
		name           string
		icmpBlocked    bool
		skipTCP        bool
		stoppedAtLimit bool
		want           string
	}{
		{
			name:        "ping blocked, TCP fallback on",
			icmpBlocked: true,
			want:        "This computer is not allowed to send ping (ICMP) packets, so the times shown are how long a TCP connection to port 443 took instead. They read a little higher than a real ping.",
		},
		{
			name:        "ping blocked, TCP fallback off",
			icmpBlocked: true,
			skipTCP:     true,
			want:        "This computer is not allowed to send ping (ICMP) packets and the TCP fallback is switched off, so nothing could be measured. Turn the TCP fallback back on in the options.",
		},
		{
			name:           "stopped at the twelve hour limit",
			stoppedAtLimit: true,
			want:           "The monitor stopped on its own after 43200 pings, which is the twelve hour limit. Start it again to keep watching.",
		},
		{
			name:           "both, joined with one space",
			icmpBlocked:    true,
			stoppedAtLimit: true,
			want:           "This computer is not allowed to send ping (ICMP) packets, so the times shown are how long a TCP connection to port 443 took instead. They read a little higher than a real ping. The monitor stopped on its own after 43200 pings, which is the twelve hour limit. Start it again to keep watching.",
		},
		{name: "healthy run says nothing", want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := noteFor(c.icmpBlocked, c.skipTCP, c.stoppedAtLimit, 443)
			if got != c.want {
				t.Errorf("note = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSummaryReportsTheRun(t *testing.T) {
	cfg, err := Params{Targets: []string{"1.2.3.4", "5.6.7.8"}}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	m := newMonitor(cfg)
	m.record(Sample{Round: 1, Target: "1.2.3.4", OK: true})
	m.record(Sample{Round: 2, Target: "1.2.3.4"})
	m.record(Sample{Round: 2, Target: "5.6.7.8", OK: true})

	sum := m.summary()
	if sum["rounds"] != 2 {
		t.Errorf("rounds = %v, want 2", sum["rounds"])
	}
	if sum["targets"] != 2 {
		t.Errorf("targets = %v, want 2", sum["targets"])
	}
	if sum["icmpAvailable"] != true {
		t.Errorf("icmpAvailable = %v, want true when nothing failed to send", sum["icmpAvailable"])
	}
	if sum["stoppedAtLimit"] != false {
		t.Errorf("stoppedAtLimit = %v, want false", sum["stoppedAtLimit"])
	}
	if sum["note"] != "" {
		t.Errorf("note = %q, want empty for a healthy run", sum["note"])
	}
}

func TestICMPBudget(t *testing.T) {
	m := newMonitor(settings{})
	for i := 0; i < icmpFailureBudget-1; i++ {
		m.icmpFails.Add(1)
		if !m.icmpUsable() {
			t.Fatalf("ping was given up after %d failures, the budget is %d", i+1, icmpFailureBudget)
		}
		if m.icmpBlocked() {
			t.Fatalf("ping was called blocked after %d failures", i+1)
		}
	}
	m.icmpFails.Add(1)
	if m.icmpUsable() {
		t.Errorf("ping kept being tried after %d outright failures", icmpFailureBudget)
	}
	if !m.icmpBlocked() {
		t.Error("icmpBlocked() is false after the budget ran out")
	}

	// One reply proves this machine may send echo requests, so later failures
	// are the network's problem, not a permissions problem.
	ok := newMonitor(settings{})
	ok.icmpOK.Store(true)
	for i := 0; i < icmpFailureBudget*3; i++ {
		ok.icmpFails.Add(1)
	}
	if !ok.icmpUsable() {
		t.Error("ping was given up even though an echo had already succeeded")
	}
	if ok.icmpBlocked() {
		t.Error("ping was called blocked even though an echo had already succeeded")
	}
}

func TestSampleReasons(t *testing.T) {
	cases := []struct {
		name        string
		resolved    bool
		icmpBlocked bool
		skipTCP     bool
		want        string
	}{
		{
			name: "the name never resolved",
			want: "This name could not be looked up, so it could not be pinged.",
		},
		{
			name:     "nothing answered in time",
			resolved: true,
			want:     "No reply within 1000 ms.",
		},
		{
			name:        "ping blocked and the TCP fallback is off",
			resolved:    true,
			icmpBlocked: true,
			skipTCP:     true,
			want:        "This computer cannot send ping packets and the TCP fallback is off.",
		},
		{
			name:        "ping blocked but the TCP fallback tried and failed",
			resolved:    true,
			icmpBlocked: true,
			want:        "No reply within 1000 ms.",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := missReason(c.resolved, c.icmpBlocked, c.skipTCP, 1000)
			if got != c.want {
				t.Errorf("reason = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTCPPingSucceeds(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen on the loopback address: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	rtt, ok := tcpPing(context.Background(), "127.0.0.1", port, time.Second)
	if !ok {
		t.Fatal("a listener that accepted the connection was reported as no reply")
	}
	if rtt <= 0 {
		t.Errorf("round trip = %s, want a positive duration", rtt)
	}
}

func TestTCPPingCountsRefusedAsAReply(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen on the loopback address: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	rtt, ok := tcpPing(context.Background(), "127.0.0.1", port, time.Second)
	if !ok {
		t.Fatal("a refused connection is still an answer from the host, it must count as a reply")
	}
	if rtt <= 0 {
		t.Errorf("round trip = %s, want a positive duration", rtt)
	}
}

func TestStartMonitorRejectsBadInputWithoutStartingAJob(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartMonitor(Params{Targets: []string{"a.com", "b.com", "c.com", "d.com", "e.com"}})
	if err == nil {
		t.Fatal("five targets were accepted")
	}
	if id != "" {
		t.Errorf("job id = %q, want empty", id)
	}
	if !strings.Contains(err.Error(), "Watch at most 4 hosts at a time. You gave 5.") {
		t.Errorf("message %q does not explain the limit", err.Error())
	}
	if n := s.jobs.Running(); n != 0 {
		t.Errorf("%d jobs running, want 0", n)
	}
}

func TestStartMonitorReturnsJobID(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartMonitor(Params{Targets: []string{"192.0.2.1"}, TimeoutMS: 100, IntervalMS: 60000})
	if err != nil {
		t.Fatalf("StartMonitor returned %v", err)
	}
	if id == "" {
		t.Fatal("StartMonitor returned an empty job id")
	}
	_ = s.jobs.Cancel(id)
}

// TestMonitorStopsAtRoundLimit checks the bounded run: "stop after 2 pings"
// really does stop, and the summary reports what happened.
func TestMonitorStopsAtRoundLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the loopback network")
	}
	cfg, err := Params{Targets: []string{"127.0.0.1"}, Rounds: 2, IntervalMS: 200, TimeoutMS: 1000}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}

	m := newMonitor(cfg)
	done := make(chan error, 1)
	core.NewJobManager().Start(JobKind, cfg.Rounds, func(jc *core.JobContext) error {
		err := m.run(jc)
		done <- err
		return err
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a two round run did not finish")
	}

	sum := m.summary()
	if sum["rounds"] != 2 {
		t.Errorf("rounds = %v, want 2", sum["rounds"])
	}
	if sum["stoppedAtLimit"] != false {
		t.Errorf("stoppedAtLimit = %v, want false: the run reached the round the user asked for", sum["stoppedAtLimit"])
	}
}

// TestMonitorIsCancellable is the one that matters for the UI's Stop button: an
// open ended run must stop soon after the job is cancelled.
func TestMonitorIsCancellable(t *testing.T) {
	// 192.0.2.1 is TEST-NET-1, which never routes, so nothing here depends on
	// this machine's network being up.
	cfg, err := Params{Targets: []string{"192.0.2.1"}, TimeoutMS: 4000}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}

	mgr := core.NewJobManager()
	done := make(chan error, 1)
	id := mgr.Start(JobKind, cfg.Rounds, func(jc *core.JobContext) error {
		err := monitorRun(jc, cfg)
		done <- err
		return err
	})

	time.Sleep(300 * time.Millisecond)
	if err := mgr.Cancel(id); err != nil {
		t.Fatalf("Cancel returned %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled run returned %v, want context.Canceled", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the monitor kept running after it was cancelled")
	}
}

func TestResolveTargetsRejectsWhenNothingResolves(t *testing.T) {
	cfg, err := Params{Targets: []string{"no-such-host.invalid"}}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	m := newMonitor(cfg)

	// .invalid never resolves anywhere (RFC 6761), but the lookup still asks the
	// resolver, so this is skipped in short mode with the other network tests.
	if testing.Short() {
		t.Skip("asks the system resolver")
	}
	err = m.resolveTargets(context.Background())
	if err == nil {
		t.Fatal("a host that cannot be looked up was accepted")
	}
	if code := core.CodeOf(err); code != core.CodeNotFound {
		t.Errorf("code = %s, want %s", code, core.CodeNotFound)
	}
	if err.Error() != "None of those hosts could be found. Check the spelling, or enter IP addresses instead." {
		t.Errorf("message = %q", err.Error())
	}
}

func TestResolveTargetsKeepsAddressLiterals(t *testing.T) {
	cfg, err := Params{Targets: []string{"192.0.2.1", "::1"}}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	m := newMonitor(cfg)
	if err := m.resolveTargets(context.Background()); err != nil {
		t.Fatalf("resolveTargets returned %v", err)
	}
	if m.targets[0].ip != "192.0.2.1" || m.targets[1].ip != "::1" {
		t.Errorf("addresses = %q and %q, want them used as typed", m.targets[0].ip, m.targets[1].ip)
	}
}

func TestPreferIPv4(t *testing.T) {
	cases := []struct {
		name string
		in   []net.IP
		want string
	}{
		{"v4 only", []net.IP{net.ParseIP("192.0.2.1")}, "192.0.2.1"},
		{"v6 first", []net.IP{net.ParseIP("2001:db8::1"), net.ParseIP("192.0.2.1")}, "192.0.2.1"},
		{"v6 only", []net.IP{net.ParseIP("2001:db8::1")}, "2001:db8::1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := preferIPv4(c.in); got != c.want {
				t.Errorf("preferIPv4 = %q, want %q", got, c.want)
			}
		})
	}
}

func TestMilliseconds(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want float64
	}{
		{0, 0},
		{1500 * time.Microsecond, 1.5},
		{time.Millisecond, 1},
		{1234 * time.Microsecond, 1.23},
		{2 * time.Second, 2000},
	}
	for _, c := range cases {
		if got := milliseconds(c.in); got != c.want {
			t.Errorf("milliseconds(%s) = %v, want %v", c.in, got, c.want)
		}
	}
}
