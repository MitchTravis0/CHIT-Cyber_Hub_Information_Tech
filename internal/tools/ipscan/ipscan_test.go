package ipscan

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
	"chit/internal/netscan"
)

func TestOptionsDefaults(t *testing.T) {
	opts, err := ScanParams{Range: "192.168.1.0/24"}.options()
	if err != nil {
		t.Fatalf("options() returned %v", err)
	}
	if opts.Range.Count != 254 {
		t.Errorf("count = %d, want 254 usable addresses", opts.Range.Count)
	}
	if opts.Workers != 0 || opts.Timeout != 0 || opts.Ports != nil {
		t.Errorf("unset parameters should stay zero so netscan applies its defaults: %+v", opts)
	}
	if opts.SkipARP {
		t.Error("the ARP readback is what finds silent devices, it must stay on")
	}
	if opts.VendorLookup == nil {
		t.Error("VendorLookup was not wired up, so no MAC would ever be named")
	}
}

func TestOptionsPassesParams(t *testing.T) {
	opts, err := ScanParams{
		Range:         "192.168.1.10-20",
		TimeoutMS:     500,
		Workers:       16,
		Ports:         []int{22, 443},
		SkipTCP:       true,
		SkipHostnames: true,
	}.options()
	if err != nil {
		t.Fatalf("options() returned %v", err)
	}
	if opts.Timeout != 500*time.Millisecond {
		t.Errorf("timeout = %s, want 500ms", opts.Timeout)
	}
	if opts.Workers != 16 || !opts.SkipTCP || !opts.SkipHostnames {
		t.Errorf("parameters were not passed through: %+v", opts)
	}
	if len(opts.Ports) != 2 || opts.Ports[0] != 22 {
		t.Errorf("ports = %v, want [22 443]", opts.Ports)
	}
	if opts.Range.Count != 11 {
		t.Errorf("count = %d, want 11", opts.Range.Count)
	}
}

func TestOptionsRejects(t *testing.T) {
	cases := []struct {
		name   string
		params ScanParams
		want   string
	}{
		{"empty range", ScanParams{}, "192.168.1.0/24"},
		{"nonsense range", ScanParams{Range: "not an address"}, "not an address"},
		{"backwards range", ScanParams{Range: "192.168.1.50-10"}, "backwards"},
		{"ipv6", ScanParams{Range: "2001:db8::1"}, "IPv4"},
		{"absurd range", ScanParams{Range: "10.0.0.0/8"}, "at most 65536"},
		{"timeout too long", ScanParams{Range: "192.168.1.0/24", TimeoutMS: 60000}, "wait per device"},
		{"negative timeout", ScanParams{Range: "192.168.1.0/24", TimeoutMS: -1}, "wait per device"},
		{"too many workers", ScanParams{Range: "192.168.1.0/24", Workers: 5000}, "at a time"},
		{"bad port", ScanParams{Range: "192.168.1.0/24", Ports: []int{0}}, "not a port number"},
		{"port too high", ScanParams{Range: "192.168.1.0/24", Ports: []int{70000}}, "not a port number"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.params.options()
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

func TestStartScanRejectsBadRangeWithoutStartingAJob(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartScan(ScanParams{Range: "10.0.0.0/8"})
	if err == nil {
		t.Fatal("a /8 was accepted")
	}
	if id != "" {
		t.Errorf("job id = %q, want empty", id)
	}
	if !strings.Contains(err.Error(), "at most 65536") {
		t.Errorf("message %q does not explain the limit", err.Error())
	}
	if n := s.jobs.Running(); n != 0 {
		t.Errorf("%d jobs running, want 0", n)
	}
}

func TestStartScanReturnsJobID(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartScan(ScanParams{Range: "192.0.2.1", SkipHostnames: true})
	if err != nil {
		t.Fatalf("StartScan returned %v", err)
	}
	if id == "" {
		t.Fatal("StartScan returned an empty job id")
	}
	_ = s.jobs.Cancel(id)
}

// TestScanIsCancellable is the one that matters for the UI's Cancel button: a
// large range must stop soon after the job is cancelled, not run to the end.
func TestScanIsCancellable(t *testing.T) {
	opts, err := ScanParams{Range: "10.0.0.0/16", TimeoutMS: 1000, SkipHostnames: true}.options()
	if err != nil {
		t.Fatalf("options() returned %v", err)
	}

	m := core.NewJobManager()
	done := make(chan error, 1)
	id := m.Start(JobKind, opts.Range.Count, func(jc *core.JobContext) error {
		_, err := sweep(jc, opts)
		done <- err
		return err
	})

	// Long enough that the sweep is genuinely in flight: 65534 addresses at a
	// second each cannot have finished.
	time.Sleep(200 * time.Millisecond)
	if err := m.Cancel(id); err != nil {
		t.Fatalf("Cancel returned %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled sweep returned %v, want context.Canceled", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the sweep kept running after it was cancelled")
	}
}

func TestVendorName(t *testing.T) {
	if got := vendorName("B8:27:EB:00:11:22"); !strings.Contains(got, "Raspberry Pi") {
		t.Errorf("vendorName of a Raspberry Pi address = %q", got)
	}
	if got := vendorName("not a mac"); got != "" {
		t.Errorf("vendorName(%q) = %q, want empty", "not a mac", got)
	}
}

// TestScanLoopback exercises the whole path (parameters, sweep, summary)
// against an address that always answers.
func TestScanLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the loopback network")
	}
	opts, err := ScanParams{Range: "127.0.0.1", TimeoutMS: 2000, SkipHostnames: true}.options()
	if err != nil {
		t.Fatalf("options() returned %v", err)
	}

	m := core.NewJobManager()
	type result struct {
		sum netscan.Summary
		err error
	}
	done := make(chan result, 1)
	m.Start(JobKind, opts.Range.Count, func(jc *core.JobContext) error {
		sum, err := sweep(jc, opts)
		done <- result{sum, err}
		return err
	})

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("sweep returned %v", r.err)
		}
		t.Logf("summary: %+v", r.sum)
		if r.sum.Total != 1 || r.sum.Up != 1 {
			t.Fatalf("127.0.0.1 was not reported up: %+v", r.sum)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the sweep did not finish")
	}
}
