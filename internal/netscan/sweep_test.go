package netscan

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"chit/internal/core"
)

type sweepResult struct {
	summary Summary
	err     error
}

// runSweep drives Sweep through a real JobManager, which is the only way to get
// a JobContext. With no Wails context set, the events go nowhere.
func runSweep(t *testing.T, opts Options) (string, *core.JobManager, chan sweepResult) {
	t.Helper()
	m := core.NewJobManager()
	done := make(chan sweepResult, 1)
	id := m.Start("ipscan", opts.Range.Count, func(jc *core.JobContext) error {
		s, err := Sweep(jc, opts)
		done <- sweepResult{s, err}
		return err
	})
	return id, m, done
}

func TestSweepRejectsBadOptions(t *testing.T) {
	_, _, done := runSweep(t, Options{Range: mustRange(t, "192.168.1.0/24"), Workers: -5})
	select {
	case r := <-done:
		if r.err == nil {
			t.Fatal("Sweep accepted invalid options")
		}
		if code := core.CodeOf(r.err); code != core.CodeInvalidInput {
			t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Sweep did not return")
	}
}

// TestSweepLoopback is the reality check: 127.0.0.1 always answers, either on
// ping or with a TCP reset.
func TestSweepLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the loopback network")
	}
	_, _, done := runSweep(t, Options{
		Range:         mustRange(t, "127.0.0.1"),
		Timeout:       2 * time.Second,
		SkipHostnames: true,
	})

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Sweep returned %v", r.err)
		}
		t.Logf("summary: %+v", r.summary)
		if r.summary.Total != 1 {
			t.Fatalf("total = %d, want 1", r.summary.Total)
		}
		if r.summary.Up != 1 {
			t.Fatalf("127.0.0.1 was reported down: %+v", r.summary)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Sweep did not finish")
	}
}

// TestProbeLoopbackHost checks the Host record itself, not just the tally.
func TestProbeLoopbackHost(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the loopback network")
	}
	o, err := Options{Range: mustRange(t, "127.0.0.1"), Timeout: 2 * time.Second}.normalize()
	if err != nil {
		t.Fatal(err)
	}
	sw := &sweeper{opts: o}
	h, keep := sw.probe(context.Background(), netip.MustParseAddr("127.0.0.1"))
	if !keep {
		t.Fatal("probe dropped the host")
	}
	t.Logf("host: %+v (icmp usable: %v)", h, sw.icmpOK.Load())
	if h.IP != "127.0.0.1" {
		t.Errorf("ip = %s, want 127.0.0.1", h.IP)
	}
	if h.Status != StatusUp {
		t.Fatalf("status = %s, want %s", h.Status, StatusUp)
	}
	if h.RespondedVia != ViaICMP && h.RespondedVia != ViaTCP {
		t.Errorf("respondedVia = %q, want icmp or tcp", h.RespondedVia)
	}
	if h.Vendor != "" {
		t.Errorf("vendor = %q, netscan must leave it to the tool layer", h.Vendor)
	}
}

// TestProbeWithoutICMP is the no-administrator promise: with ping switched off,
// a refused TCP connection still proves the host is there.
func TestProbeWithoutICMP(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the loopback network")
	}
	o, err := Options{
		Range:         mustRange(t, "127.0.0.1"),
		Timeout:       time.Second,
		Ports:         []int{9},
		SkipICMP:      true,
		SkipHostnames: true,
	}.normalize()
	if err != nil {
		t.Fatal(err)
	}
	sw := &sweeper{opts: o}
	h, _ := sw.probe(context.Background(), netip.MustParseAddr("127.0.0.1"))
	t.Logf("host: %+v", h)
	if h.Status != StatusUp || h.RespondedVia != ViaTCP {
		t.Fatalf("status = %s via %q, want up via tcp", h.Status, h.RespondedVia)
	}
	if h.OpenPorts == nil {
		t.Error("openPorts should be an empty list, not null")
	}
	if len(h.OpenPorts) != 0 {
		t.Errorf("openPorts = %v, a refused port is not an open one", h.OpenPorts)
	}
}

func TestProbeReportsAnOpenPort(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the loopback network")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	o, err := Options{
		Range:         mustRange(t, "127.0.0.1"),
		Timeout:       time.Second,
		Ports:         []int{port},
		SkipICMP:      true,
		SkipHostnames: true,
	}.normalize()
	if err != nil {
		t.Fatal(err)
	}
	sw := &sweeper{opts: o}
	h, _ := sw.probe(context.Background(), netip.MustParseAddr("127.0.0.1"))
	if h.Status != StatusUp || h.RespondedVia != ViaTCP {
		t.Fatalf("status = %s via %q, want up via tcp", h.Status, h.RespondedVia)
	}
	if len(h.OpenPorts) != 1 || h.OpenPorts[0] != port {
		t.Errorf("openPorts = %v, want [%d]", h.OpenPorts, port)
	}
}

// TestARPTableSmoke does not assume this machine has any neighbours, only that
// whatever the OS reports parses into clean entries.
func TestARPTableSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("reads the system ARP cache")
	}
	entries, err := arpTable(context.Background())
	if err != nil {
		t.Skipf("no ARP cache available here: %v", err)
	}
	t.Logf("%d ARP entries", len(entries))
	for _, e := range entries {
		if a, perr := netip.ParseAddr(e.IP); perr != nil || !a.Is4() {
			t.Errorf("entry %+v has a bad IP", e)
		}
		if normalizeMAC(e.MAC) != e.MAC || !usableMAC(e.MAC) {
			t.Errorf("entry %+v has an unnormalised or unusable MAC", e)
		}
	}
}

func TestSweepCancelsPromptly(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the loopback network")
	}
	// A big slice of loopback space: plenty of work, no packets on the LAN.
	id, m, done := runSweep(t, Options{
		Range:         mustRange(t, "127.0.0.0/16"),
		Workers:       8,
		Timeout:       time.Second,
		SkipHostnames: true,
		SkipARP:       true,
	})

	time.Sleep(200 * time.Millisecond)
	if err := m.Cancel(id); err != nil {
		t.Fatalf("Cancel returned %v", err)
	}

	select {
	case r := <-done:
		if !errors.Is(r.err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled so the job reports as cancelled", r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Sweep ignored cancellation")
	}
}

func TestSummaryMap(t *testing.T) {
	s := Summary{Total: 254, Up: 3, Down: 251, ViaICMP: 1, ViaTCP: 1, ViaARP: 1, WithMAC: 2}
	m := s.Map()
	if m["total"] != 254 || m["up"] != 3 || m["viaArp"] != 1 || m["withMac"] != 2 {
		t.Errorf("Map lost values: %v", m)
	}
	if _, ok := m["note"]; !ok {
		t.Error("Map should always carry a note key")
	}
}

func TestMilliseconds(t *testing.T) {
	cases := map[time.Duration]float64{
		0:                       0,
		1500 * time.Microsecond: 1.5,
		time.Second:             1000,
		123 * time.Microsecond:  0.12,
	}
	for in, want := range cases {
		if got := milliseconds(in); got != want {
			t.Errorf("milliseconds(%s) = %v, want %v", in, got, want)
		}
	}
}

func TestIsRefused(t *testing.T) {
	if IsRefused(nil) {
		t.Error("nil error is not a refusal")
	}
	if !IsRefused(errors.New("dial tcp 127.0.0.1:9: connect: connection refused")) {
		t.Error("connection refused text should count as a refusal")
	}
	if IsRefused(errors.New("dial tcp 10.0.0.1:80: i/o timeout")) {
		t.Error("a timeout is not a refusal")
	}
}
