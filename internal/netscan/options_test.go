package netscan

import (
	"testing"
	"time"

	"chit/internal/core"
)

func mustRange(t *testing.T, s string) Range {
	t.Helper()
	r, err := ParseRange(s)
	if err != nil {
		t.Fatalf("ParseRange(%q): %v", s, err)
	}
	return r
}

func TestOptionsDefaults(t *testing.T) {
	o, err := Options{Range: mustRange(t, "192.168.1.0/24")}.normalize()
	if err != nil {
		t.Fatalf("normalize returned %v", err)
	}
	if o.Workers != DefaultWorkers {
		t.Errorf("workers = %d, want %d", o.Workers, DefaultWorkers)
	}
	if o.Timeout != DefaultTimeout {
		t.Errorf("timeout = %s, want %s", o.Timeout, DefaultTimeout)
	}
	if o.DNSTimeout != DefaultDNSTimeout {
		t.Errorf("dns timeout = %s, want %s", o.DNSTimeout, DefaultDNSTimeout)
	}
	if len(o.Ports) != len(DefaultPorts) {
		t.Errorf("ports = %v, want %v", o.Ports, DefaultPorts)
	}
}

func TestOptionsWorkersNeverExceedTheRange(t *testing.T) {
	o, err := Options{Range: mustRange(t, "10.0.0.5")}.normalize()
	if err != nil {
		t.Fatalf("normalize returned %v", err)
	}
	if o.Workers != 1 {
		t.Errorf("workers = %d, want 1 for a single address", o.Workers)
	}
}

func TestOptionsKeepsExplicitValues(t *testing.T) {
	in := Options{
		Range:      mustRange(t, "192.168.1.0/24"),
		Workers:    16,
		Timeout:    2 * time.Second,
		DNSTimeout: 50 * time.Millisecond,
		Ports:      []int{22},
	}
	o, err := in.normalize()
	if err != nil {
		t.Fatalf("normalize returned %v", err)
	}
	if o.Workers != 16 || o.Timeout != 2*time.Second || o.DNSTimeout != 50*time.Millisecond {
		t.Errorf("normalize changed explicit values: %+v", o)
	}
	if len(o.Ports) != 1 || o.Ports[0] != 22 {
		t.Errorf("ports = %v, want [22]", o.Ports)
	}
}

func TestOptionsValidation(t *testing.T) {
	good := mustRange(t, "192.168.1.0/24")
	cases := []struct {
		name string
		opts Options
	}{
		{"no range", Options{}},
		{"range with no addresses", Options{Range: Range{Text: "nothing"}}},
		{"oversized range", Options{Range: Range{Start: good.Start, End: good.End, Count: MaxAddresses + 1}}},
		{"negative workers", Options{Range: good, Workers: -1}},
		{"too many workers", Options{Range: good, Workers: maxWorkers + 1}},
		{"negative timeout", Options{Range: good, Timeout: -time.Second}},
		{"huge timeout", Options{Range: good, Timeout: maxTimeout + time.Second}},
		{"negative dns timeout", Options{Range: good, DNSTimeout: -time.Second}},
		{"huge dns timeout", Options{Range: good, DNSTimeout: maxDNSTimeout + time.Second}},
		{"port zero", Options{Range: good, Ports: []int{0}}},
		{"port too high", Options{Range: good, Ports: []int{70000}}},
		{"negative port", Options{Range: good, Ports: []int{80, -1}}},
		{"everything switched off", Options{Range: good, SkipICMP: true, SkipTCP: true, SkipARP: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.opts.normalize(); err == nil {
				t.Fatal("normalize accepted options it should reject")
			} else if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
			}
		})
	}
}
