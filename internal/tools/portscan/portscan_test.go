package portscan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

func TestNormalizeDefaults(t *testing.T) {
	set, err := Params{Host: "1.2.3.4", Ports: "80"}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	if set.host != "1.2.3.4" {
		t.Errorf("host = %q, want 1.2.3.4", set.host)
	}
	if set.timeout != defaultTimeoutMS*time.Millisecond {
		t.Errorf("timeout = %s, want %d ms", set.timeout, defaultTimeoutMS)
	}
	// One port, so the default of 64 workers is clamped down to it.
	if set.workers != 1 {
		t.Errorf("workers = %d, want 1", set.workers)
	}
	if set.banners {
		t.Error("banner grabbing is off unless the user asks for it")
	}
}

func TestNormalizeTrimsHostAndPassesParams(t *testing.T) {
	set, err := Params{
		Host:        "  printer-3  ",
		Ports:       "1-1024",
		TimeoutMS:   2000,
		Workers:     128,
		GrabBanners: true,
	}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	if set.host != "printer-3" {
		t.Errorf("host = %q, want printer-3", set.host)
	}
	if set.timeout != 2*time.Second {
		t.Errorf("timeout = %s, want 2s", set.timeout)
	}
	if set.workers != 128 {
		t.Errorf("workers = %d, want 128", set.workers)
	}
	if len(set.ports) != 1024 {
		t.Errorf("ports = %d, want 1024", len(set.ports))
	}
	if !set.banners {
		t.Error("GrabBanners was not passed through")
	}
}

func TestWorkersClampedToPortCount(t *testing.T) {
	set, err := Params{Host: "1.2.3.4", Ports: "80,443", Workers: 64}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}
	if set.workers != 2 {
		t.Errorf("workers = %d, want 2 for a two port scan", set.workers)
	}
}

func TestNormalizeRejects(t *testing.T) {
	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{"no host", Params{Ports: "80"}, "Enter a host name or IP address"},
		{"whitespace host", Params{Host: "   ", Ports: "80"}, "Enter a host name or IP address"},
		{"absurdly long host", Params{Host: strings.Repeat("a", 300), Ports: "80"}, "Enter a host name or IP address"},
		{"no ports", Params{Host: "1.2.3.4"}, "Enter at least one port to check"},
		{"bad ports", Params{Host: "1.2.3.4", Ports: "80,abc"}, "is not a port or a port range"},
		{"timeout too short", Params{Host: "1.2.3.4", Ports: "80", TimeoutMS: 50}, "wait per port"},
		{"timeout too long", Params{Host: "1.2.3.4", Ports: "80", TimeoutMS: 30001}, "wait per port"},
		{"negative timeout", Params{Host: "1.2.3.4", Ports: "80", TimeoutMS: -1}, "wait per port"},
		{"negative workers", Params{Host: "1.2.3.4", Ports: "80", Workers: -1}, "at a time"},
		{"too many workers", Params{Host: "1.2.3.4", Ports: "80", Workers: 513}, "at a time"},
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

func TestServiceName(t *testing.T) {
	cases := []struct {
		port int
		want string
	}{
		{22, "SSH"},
		{3389, "RDP remote desktop"},
		{9100, "Raw printing (JetDirect)"},
		{12345, ""},
	}
	for _, c := range cases {
		if got := serviceName(c.port); got != c.want {
			t.Errorf("serviceName(%d) = %q, want %q", c.port, got, c.want)
		}
	}
}

func TestSanitizeBanner(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"a line ending is trimmed", "SSH-2.0-OpenSSH_9.6\r\n", "SSH-2.0-OpenSSH_9.6"},
		{"control bytes collapse to one space", "220\x00\x01mail ready", "220 mail ready"},
		{"runs of spaces collapse", "220    mail     ready", "220 mail ready"},
		{"leading and trailing space goes", "  hello  ", "hello"},
		{"nothing but line endings", "\r\n\r\n", ""},
		{"empty stays empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeBanner(c.in); got != c.want {
				t.Errorf("sanitizeBanner(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}

	long := sanitizeBanner(strings.Repeat("a", 400))
	if len(long) != maxBanner+3 {
		t.Errorf("a 400 character banner became %d characters, want %d", len(long), maxBanner+3)
	}
	if !strings.HasSuffix(long, "...") {
		t.Errorf("a truncated banner should end in ..., got %q", long[len(long)-5:])
	}
}

func TestSummaryNote(t *testing.T) {
	cases := []struct {
		name                   string
		open, closed, filtered int
		want                   string
	}{
		{
			"nothing answered at all", 0, 0, 5,
			"Nothing answered on any of those ports. The host may be switched off, or a firewall is dropping the checks without replying.",
		},
		{
			"the host is up but nothing listens", 0, 3, 2,
			"The host is there and answering, but nothing is listening on the ports you checked. The service is probably stopped, or it is running on a different port.",
		},
		{
			"some open and some silent", 2, 0, 3,
			"3 ports gave no answer at all. That usually means a firewall is dropping the traffic rather than the port being closed.",
		},
		{"everything was decided", 2, 3, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := summaryNote(c.open, c.closed, c.filtered); got != c.want {
				t.Errorf("summaryNote(%d, %d, %d) = %q, want %q",
					c.open, c.closed, c.filtered, got, c.want)
			}
		})
	}
}

// loopback opens a listener on a free loopback port and reports the port.
func loopback(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen on the loopback interface: %v", err)
	}
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func TestProbeOpen(t *testing.T) {
	ln, port := loopback(t)
	defer ln.Close()

	got := probe(context.Background(), "127.0.0.1", port, time.Second, false)
	if got.State != StateOpen {
		t.Fatalf("state = %q, want %q", got.State, StateOpen)
	}
	if got.Port != port {
		t.Errorf("port = %d, want %d", got.Port, port)
	}
	if got.LatencyMS < 0 {
		t.Errorf("latency = %v, want a non-negative number", got.LatencyMS)
	}
	if got.Banner != "" {
		t.Errorf("banner = %q, want empty when banners are off", got.Banner)
	}
}

func TestProbeClosed(t *testing.T) {
	ln, port := loopback(t)
	if err := ln.Close(); err != nil {
		t.Fatalf("closing the listener returned %v", err)
	}

	got := probe(context.Background(), "127.0.0.1", port, time.Second, false)
	if got.State != StateClosed {
		t.Fatalf("state = %q, want %q: a refused connection is what proves the host is there",
			got.State, StateClosed)
	}
}

func TestProbeBanner(t *testing.T) {
	ln, port := loopback(t)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("SSH-2.0-test\r\n"))
			conn.Close()
		}
	}()

	with := probe(context.Background(), "127.0.0.1", port, time.Second, true)
	if with.Banner != "SSH-2.0-test" {
		t.Errorf("banner = %q, want %q", with.Banner, "SSH-2.0-test")
	}

	without := probe(context.Background(), "127.0.0.1", port, time.Second, false)
	if without.Banner != "" {
		t.Errorf("banner = %q, want empty when the option is off", without.Banner)
	}
}

func TestProbeHonoursCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	// 192.0.2.1 is TEST-NET-1 and never routes, so only the cancelled context
	// can end this quickly.
	got := probe(ctx, "192.0.2.1", 80, 4*time.Second, false)
	elapsed := time.Since(started)

	if got.State != StateFiltered {
		t.Errorf("state = %q, want %q", got.State, StateFiltered)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("a cancelled probe took %s, want it to give up at once", elapsed)
	}
}

func TestStartScanRejectsBadInputWithoutStartingAJob(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartScan(Params{Host: "192.0.2.1", Ports: "443-80"})
	if err == nil {
		t.Fatal("a backwards range was accepted")
	}
	if id != "" {
		t.Errorf("job id = %q, want empty", id)
	}
	if !strings.Contains(err.Error(), "runs backwards") {
		t.Errorf("message %q does not explain the problem", err.Error())
	}
	if n := s.jobs.Running(); n != 0 {
		t.Errorf("%d jobs running, want 0", n)
	}
}

func TestStartScanReturnsJobID(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartScan(Params{Host: "192.0.2.1", Ports: "80", TimeoutMS: 100})
	if err != nil {
		t.Fatalf("StartScan returned %v", err)
	}
	if id == "" {
		t.Fatal("StartScan returned an empty job id")
	}
	_ = s.jobs.Cancel(id)
}

// TestScanIsCancellable is the one that matters for the UI's Cancel button: a
// long scan must stop soon after the job is cancelled, not run to the end.
func TestScanIsCancellable(t *testing.T) {
	set, err := Params{Host: "192.0.2.1", Ports: "1-65535", TimeoutMS: 4000}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}

	m := core.NewJobManager()
	finished := make(chan error, 1)
	id := m.Start(JobKind, len(set.ports), func(jc *core.JobContext) error {
		_, err := scan(jc, set)
		finished <- err
		return err
	})

	// Long enough that the scan is genuinely in flight: 65535 ports at up to
	// four seconds each cannot have finished.
	time.Sleep(200 * time.Millisecond)
	if err := m.Cancel(id); err != nil {
		t.Fatalf("Cancel returned %v", err)
	}

	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled scan returned %v, want context.Canceled", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the scan kept running after it was cancelled")
	}
}

// TestScanLoopbackEndToEnd exercises the whole path: parameters, resolution,
// probing and the summary, against ports whose answers are known.
func TestScanLoopbackEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the loopback network")
	}

	open, openPort := loopback(t)
	defer open.Close()
	shut, shutPort := loopback(t)
	if err := shut.Close(); err != nil {
		t.Fatalf("closing the second listener returned %v", err)
	}

	set, err := Params{
		Host:      "127.0.0.1",
		Ports:     fmt.Sprintf("%d,%d", openPort, shutPort),
		TimeoutMS: 2000,
	}.normalize()
	if err != nil {
		t.Fatalf("normalize() returned %v", err)
	}

	type outcome struct {
		summary map[string]any
		err     error
	}
	finished := make(chan outcome, 1)
	core.NewJobManager().Start(JobKind, len(set.ports), func(jc *core.JobContext) error {
		summary, err := scan(jc, set)
		finished <- outcome{summary, err}
		return err
	})

	select {
	case got := <-finished:
		if got.err != nil {
			t.Fatalf("scan returned %v", got.err)
		}
		if got.summary["open"] != 1 {
			t.Errorf("open = %v, want 1", got.summary["open"])
		}
		if got.summary["closed"] != 1 {
			t.Errorf("closed = %v, want 1", got.summary["closed"])
		}
		if got.summary["ip"] != "127.0.0.1" {
			t.Errorf("ip = %v, want 127.0.0.1", got.summary["ip"])
		}
		if got.summary["note"] != "" {
			t.Errorf("note = %v, want empty when one port is open and one is closed", got.summary["note"])
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the scan did not finish")
	}
}
