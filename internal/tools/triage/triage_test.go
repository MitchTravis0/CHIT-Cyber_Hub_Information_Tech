package triage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"chit/internal/netinfo"
)

// recorder is the Sink a test wires up. A JobContext's emitted items go out
// through the Wails runtime and cannot be read back, which is exactly why Run
// takes a Sink instead of a JobContext.
type recorder struct {
	mu       sync.Mutex
	rungs    []Rung
	progress []string
}

func (r *recorder) sink() Sink {
	return Sink{
		Emit: func(rung Rung) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.rungs = append(r.rungs, rung)
		},
		Progress: func(_ int, message string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.progress = append(r.progress, message)
		},
	}
}

func (r *recorder) byID(id string) Rung {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rung := range r.rungs {
		if rung.ID == id {
			return rung
		}
	}
	return Rung{}
}

// counted wraps a Probes so a test can assert that a probe was never called,
// which is how "the ladder stopped" is proved without reading the output.
type counted struct {
	Probes
	mu    sync.Mutex
	calls map[string]int
}

func count(p Probes) *counted {
	c := &counted{calls: map[string]int{}}
	c.Adapters = func() (netinfo.Report, error) { c.bump("adapters"); return p.Adapters() }
	c.Ping = func(ctx context.Context, ip string, d time.Duration) (float64, bool) {
		c.bump("ping")
		return p.Ping(ctx, ip, d)
	}
	c.Dial = func(ctx context.Context, addr string, d time.Duration) (float64, bool) {
		c.bump("dial")
		return p.Dial(ctx, addr, d)
	}
	c.Resolve = func(ctx context.Context, server, name string, d time.Duration) ([]string, error) {
		c.bump("resolve")
		return p.Resolve(ctx, server, name, d)
	}
	c.Get = func(ctx context.Context, url string, d time.Duration) (int, string, string, error) {
		c.bump("get:" + url)
		return p.Get(ctx, url, d)
	}
	return c
}

func (c *counted) bump(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[name]++
}

func (c *counted) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[name]
}

// healthy is a machine where everything works.
func healthy() Probes {
	return Probes{
		Adapters: func() (netinfo.Report, error) {
			return netinfo.Report{Adapters: []netinfo.Adapter{{
				Name: "eth0", Up: true, Primary: true, Gateway: "192.168.1.1",
				IPv4: []netinfo.IPv4{{IP: "192.168.1.42"}},
			}}}, nil
		},
		Ping: func(context.Context, string, time.Duration) (float64, bool) { return 3, true },
		Dial: func(context.Context, string, time.Duration) (float64, bool) { return 14, true },
		Resolve: func(_ context.Context, _, _ string, _ time.Duration) ([]string, error) {
			return []string{"93.184.216.34"}, nil
		},
		Get: func(_ context.Context, url string, _ time.Duration) (int, string, string, error) {
			if strings.HasPrefix(url, "https://") {
				return 204, "", "", nil
			}
			return 200, "", PortalTestBody + "\n", nil
		},
	}
}

func run(t *testing.T, p Probes) *recorder {
	t.Helper()
	rec := &recorder{}
	if _, err := Run(context.Background(), time.Second, p, rec.sink()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}
	return rec
}

func runSummary(t *testing.T, p Probes) (*recorder, map[string]any) {
	t.Helper()
	rec := &recorder{}
	summary, err := Run(context.Background(), time.Second, p, rec.sink())
	if err != nil {
		t.Fatalf("Run errored: %v", err)
	}
	return rec, summary
}

func TestNormalizeParams(t *testing.T) {
	tests := []struct {
		in      int
		wantMS  int
		wantErr bool
	}{
		{0, 4000, false},
		{499, 0, true},
		{500, 500, false},
		{20000, 20000, false},
		{20001, 0, true},
		{-1, 0, true},
	}
	for _, tt := range tests {
		got, err := Params{TimeoutMS: tt.in}.normalize()
		if tt.wantErr {
			if err == nil {
				t.Errorf("timeout %d was accepted", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("timeout %d errored: %v", tt.in, err)
			continue
		}
		if got != time.Duration(tt.wantMS)*time.Millisecond {
			t.Errorf("timeout %d became %v, want %d ms", tt.in, got, tt.wantMS)
		}
	}
}

// TestStepsMatchesTheLadder pins the progress denominator to the real number of
// rungs. Steps is passed to jobs.Start and to every Progress call, so if it
// drifts the bar reads "5 of 6" and nothing else notices.
func TestStepsMatchesTheLadder(t *testing.T) {
	if Steps != 6 {
		t.Errorf("Steps = %d, want 6", Steps)
	}
	if got := len(ladder()); got != Steps {
		t.Errorf("the ladder has %d rungs but Steps is %d", got, Steps)
	}
}

func TestAdapterRung(t *testing.T) {
	adapters := func(list ...netinfo.Adapter) func() (netinfo.Report, error) {
		return func() (netinfo.Report, error) { return netinfo.Report{Adapters: list}, nil }
	}

	tests := []struct {
		name       string
		adapters   func() (netinfo.Report, error)
		wantStatus string
		wantDetail string
		wantAdvice string
	}{
		{
			"a real address",
			adapters(netinfo.Adapter{Name: "eth0", Up: true, IPv4: []netinfo.IPv4{{IP: "192.168.1.42"}}}),
			StatusOK, "192.168.1.42 on eth0", "has an address on the network",
		},
		{
			"a self-assigned address only",
			adapters(netinfo.Adapter{Name: "eth0", Up: true, IPv4: []netinfo.IPv4{{IP: "169.254.8.12"}}}),
			StatusFail, "169.254.8.12 on eth0, which is a self-assigned address", "No DHCP server answered",
		},
		{
			"a real address alongside a self-assigned one wins",
			adapters(
				netinfo.Adapter{Name: "eth1", Up: true, IPv4: []netinfo.IPv4{{IP: "169.254.8.12"}}},
				netinfo.Adapter{Name: "eth0", Up: true, IPv4: []netinfo.IPv4{{IP: "10.0.0.5"}}},
			),
			StatusOK, "10.0.0.5 on eth0", "has an address on the network",
		},
		{
			"nothing up",
			adapters(netinfo.Adapter{Name: "eth0", Up: false, IPv4: []netinfo.IPv4{{IP: "10.0.0.5"}}}),
			StatusFail, "no adapter has an address", "Plug in the cable",
		},
		{
			"loopback only",
			adapters(netinfo.Adapter{Name: "lo", Up: true, Loopback: true, IPv4: []netinfo.IPv4{{IP: "127.0.0.1"}}}),
			StatusFail, "no adapter has an address", "Plug in the cable",
		},
		{
			"no adapters at all",
			adapters(),
			StatusFail, "no adapter has an address", "Plug in the cable",
		},
		{
			"IPv6 only",
			adapters(netinfo.Adapter{Name: "eth0", Up: true, IPv6: []netinfo.IPv6{{IP: "2001:db8::1"}}}),
			StatusFail, "eth0 has an IPv6 address but no IPv4 address", "CHIT checks IPv4 only",
		},
		{
			"adapters could not be read",
			func() (netinfo.Report, error) { return netinfo.Report{}, errors.New("boom") },
			StatusFail, "this computer would not list its adapters", "network service is running",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapterRung(context.Background(), Probes{Adapters: tt.adapters}, time.Second)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (detail %q)", got.Status, tt.wantStatus, got.Detail)
			}
			if got.Detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", got.Detail, tt.wantDetail)
			}
			if !strings.Contains(got.Advice, tt.wantAdvice) {
				t.Errorf("advice %q does not contain %q", got.Advice, tt.wantAdvice)
			}
		})
	}
}

func TestGatewayRung(t *testing.T) {
	withGateway := func(gw string, unsup ...string) func() (netinfo.Report, error) {
		return func() (netinfo.Report, error) {
			return netinfo.Report{
				Adapters:    []netinfo.Adapter{{Name: "eth0", Up: true, Primary: true, Gateway: gw}},
				Unsupported: unsup,
			}, nil
		}
	}
	never := func(context.Context, string, time.Duration) (float64, bool) { return 0, false }

	t.Run("answers ping", func(t *testing.T) {
		got := gatewayRung(context.Background(), Probes{
			Adapters: withGateway("192.168.1.1"),
			Ping:     func(context.Context, string, time.Duration) (float64, bool) { return 3, true },
			Dial:     never,
		}, time.Second)
		if got.Status != StatusOK || got.Detail != "192.168.1.1 answered ping in 3 ms" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("ignores ping but answers on port 80", func(t *testing.T) {
		got := gatewayRung(context.Background(), Probes{
			Adapters: withGateway("192.168.1.1"),
			Ping:     never,
			Dial: func(_ context.Context, addr string, _ time.Duration) (float64, bool) {
				return 4, addr == "192.168.1.1:80"
			},
		}, time.Second)
		if got.Status != StatusOK {
			t.Fatalf("status = %q, want ok (%s)", got.Status, got.Detail)
		}
		if got.Detail != "192.168.1.1 ignored ping but accepted a connection on port 80 in 4 ms" {
			t.Errorf("detail = %q", got.Detail)
		}
	})

	// Proves the port list is walked in order rather than only the first being
	// tried: 80 is refused and 443 answers.
	t.Run("falls through to port 443", func(t *testing.T) {
		got := gatewayRung(context.Background(), Probes{
			Adapters: withGateway("192.168.1.1"),
			Ping:     never,
			Dial: func(_ context.Context, addr string, _ time.Duration) (float64, bool) {
				return 9, addr == "192.168.1.1:443"
			},
		}, time.Second)
		if !strings.Contains(got.Detail, "on port 443") {
			t.Errorf("detail = %q", got.Detail)
		}
	})

	// A gateway that ignores everything is common, so the rung must not burn
	// four whole timeouts before the ladder moves on.
	t.Run("the three ports share one rung's budget", func(t *testing.T) {
		var seen []time.Duration
		gatewayRung(context.Background(), Probes{
			Adapters: withGateway("192.168.1.1"),
			Ping:     never,
			Dial: func(_ context.Context, _ string, d time.Duration) (float64, bool) {
				seen = append(seen, d)
				return 0, false
			},
		}, 3*time.Second)

		if len(seen) != 3 {
			t.Fatalf("dialled %d ports, want 3", len(seen))
		}
		var total time.Duration
		for _, d := range seen {
			total += d
		}
		if total > 3*time.Second {
			t.Errorf("the three ports were given %v in total, want at most the 3s rung budget", total)
		}
		for _, d := range seen {
			if d < 250*time.Millisecond {
				t.Errorf("a port was given only %v, which is too short to be worth trying", d)
			}
		}
	})

	// A short rung budget must not divide down to nothing.
	t.Run("a short budget still leaves each port a usable slice", func(t *testing.T) {
		var seen []time.Duration
		gatewayRung(context.Background(), Probes{
			Adapters: withGateway("192.168.1.1"),
			Ping:     never,
			Dial: func(_ context.Context, _ string, d time.Duration) (float64, bool) {
				seen = append(seen, d)
				return 0, false
			},
		}, 300*time.Millisecond)

		for _, d := range seen {
			if d != 250*time.Millisecond {
				t.Errorf("port budget = %v, want the 250ms floor", d)
			}
		}
	})

	t.Run("nothing answers is a warn, not a fail", func(t *testing.T) {
		got := gatewayRung(context.Background(), Probes{
			Adapters: withGateway("192.168.1.1"), Ping: never, Dial: never,
		}, time.Second)
		if got.Status != StatusWarn {
			t.Fatalf("status = %q, want warn: a silent gateway must not stop the ladder", got.Status)
		}
		if got.Detail != "192.168.1.1 did not answer ping or a connection on ports 80, 443 or 53" {
			t.Errorf("detail = %q", got.Detail)
		}
	})

	t.Run("no gateway configured", func(t *testing.T) {
		got := gatewayRung(context.Background(), Probes{
			Adapters: withGateway(""), Ping: never, Dial: never,
		}, time.Second)
		if got.Status != StatusFail || got.Detail != "this computer has no default gateway" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("this OS will not report a gateway", func(t *testing.T) {
		got := gatewayRung(context.Background(), Probes{
			Adapters: withGateway("", netinfo.FieldGateway), Ping: never, Dial: never,
		}, time.Second)
		if got.Status != StatusWarn {
			t.Fatalf("status = %q, want warn", got.Status)
		}
		if got.Detail != "this computer will not say what its gateway is" {
			t.Errorf("detail = %q", got.Detail)
		}
	})
}

func TestDNSRung(t *testing.T) {
	tests := []struct {
		name       string
		resolve    func(context.Context, string, string, time.Duration) ([]string, error)
		wantStatus string
		wantIn     string
		wantAdvice string
	}{
		{
			"system resolver answers",
			func(context.Context, string, string, time.Duration) ([]string, error) {
				return []string{"93.184.216.34"}, nil
			},
			StatusOK, "example.com resolved to 93.184.216.34", "Name lookups are working.",
		},
		{
			"system fails, the public resolver works",
			func(_ context.Context, server, _ string, _ time.Duration) ([]string, error) {
				if server == "" {
					return nil, errors.New("no answer")
				}
				return []string{"93.184.216.34"}, nil
			},
			StatusFail, "but 1.1.1.1 resolved it in", "The internet itself is reachable",
		},
		{
			"both fail",
			func(context.Context, string, string, time.Duration) ([]string, error) {
				return nil, errors.New("no answer")
			},
			StatusFail, "neither this computer's DNS server nor 1.1.1.1", "Nothing is getting out to port 53",
		},
		{
			"answers with no addresses",
			func(context.Context, string, string, time.Duration) ([]string, error) {
				return nil, nil
			},
			StatusFail, "answered about example.com with no addresses", "giving empty replies",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dnsRung(context.Background(), Probes{Resolve: tt.resolve}, time.Second)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (%s)", got.Status, tt.wantStatus, got.Detail)
			}
			if !strings.Contains(got.Detail, tt.wantIn) {
				t.Errorf("detail %q does not contain %q", got.Detail, tt.wantIn)
			}
			if !strings.Contains(got.Advice, tt.wantAdvice) {
				t.Errorf("advice %q does not contain %q", got.Advice, tt.wantAdvice)
			}
		})
	}
}

func TestInternetRung(t *testing.T) {
	t.Run("the first target answers", func(t *testing.T) {
		got := internetRung(context.Background(), Probes{
			Dial: func(_ context.Context, addr string, _ time.Duration) (float64, bool) {
				return 14, addr == InternetTargets[0]
			},
		}, time.Second)
		if got.Status != StatusOK || !strings.Contains(got.Detail, InternetTargets[0]) {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("the first fails and the second answers", func(t *testing.T) {
		got := internetRung(context.Background(), Probes{
			Dial: func(_ context.Context, addr string, _ time.Duration) (float64, bool) {
				return 20, addr == InternetTargets[1]
			},
		}, time.Second)
		if got.Status != StatusOK {
			t.Fatalf("status = %q, want ok: one operator being down is not no internet", got.Status)
		}
		if !strings.Contains(got.Detail, InternetTargets[1]) {
			t.Errorf("detail = %q", got.Detail)
		}
	})

	t.Run("nothing answers", func(t *testing.T) {
		got := internetRung(context.Background(), Probes{
			Dial: func(context.Context, string, time.Duration) (float64, bool) { return 0, false },
		}, time.Second)
		if got.Status != StatusFail {
			t.Fatalf("status = %q, want fail", got.Status)
		}
		if got.Detail != "neither 1.1.1.1:443 nor 8.8.8.8:443 answered" {
			t.Errorf("detail = %q", got.Detail)
		}
	})
}

func TestHTTPSRung(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		err        error
		wantStatus string
		wantDetail string
	}{
		{"204 with an empty body", 204, "", nil, StatusOK, "answered 204 in"},
		{"200 instead", 200, "hello", nil, StatusFail, "answered 200 instead of 204"},
		{"a redirect", 302, "", nil, StatusFail, "answered 302 instead of 204"},
		{"a transport error", 0, "", errors.New("boom"), StatusFail, "the connection failed before an answer came back"},
		{"204 but with a body", 204, "unexpected", nil, StatusFail, "answered 204 instead of 204"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := httpsRung(context.Background(), Probes{
				Get: func(context.Context, string, time.Duration) (int, string, string, error) {
					return tt.status, "", tt.body, tt.err
				},
			}, time.Second)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (%s)", got.Status, tt.wantStatus, got.Detail)
			}
			if !strings.Contains(got.Detail, tt.wantDetail) {
				t.Errorf("detail %q does not contain %q", got.Detail, tt.wantDetail)
			}
		})
	}
}

func TestPortalRung(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		location   string
		body       string
		err        error
		wantStatus string
		wantDetail string
		wantAdvice string
	}{
		{
			"the expected text", 200, "", "success\n", nil,
			StatusOK, "answered 200 with the expected text", "not asking you to sign in",
		},
		{
			"a redirect", 302, "https://wifi.example.com/login", "", nil,
			StatusFail, "answered 302 and sent us to https://wifi.example.com/login", "A login page is in the way",
		},
		{
			"a redirect with no Location", 307, "", "", nil,
			StatusFail, "answered 307 and sent us to somewhere else", "A login page is in the way",
		},
		{
			"200 with different text", 200, "", "<html>sign in</html>", nil,
			StatusFail, "answered 200 but with different text", "rewriting plain web pages",
		},
		{
			"a transport error", 0, "", "", errors.New("boom"),
			StatusWarn, "the check could not be made", "cannot be ruled out",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := portalRung(context.Background(), Probes{
				Get: func(context.Context, string, time.Duration) (int, string, string, error) {
					return tt.status, tt.location, tt.body, tt.err
				},
			}, time.Second)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (%s)", got.Status, tt.wantStatus, got.Detail)
			}
			if !strings.Contains(got.Detail, tt.wantDetail) {
				t.Errorf("detail %q does not contain %q", got.Detail, tt.wantDetail)
			}
			if !strings.Contains(got.Advice, tt.wantAdvice) {
				t.Errorf("advice %q does not contain %q", got.Advice, tt.wantAdvice)
			}
		})
	}
}

func TestPortalBodyComparison(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"success", true},
		{"success\n", true},
		{"success\r\n", true},
		{"success   ", true},
		{"success!", false},
		{"<html>success</html>", false},
		{"", false},
		{"Success", false},
	}
	for _, tt := range tests {
		if got := bodyMatches(tt.body, PortalTestBody); got != tt.want {
			t.Errorf("bodyMatches(%q) = %v, want %v", tt.body, got, tt.want)
		}
	}
}

// TestLadderStopsAtFirstFail proves the stop by counting probe calls, not by
// reading the output: a rung that ran and was then overwritten would look
// identical in the emitted list.
func TestLadderStopsAtFirstFail(t *testing.T) {
	p := healthy()
	p.Resolve = func(context.Context, string, string, time.Duration) ([]string, error) {
		return nil, errors.New("no answer")
	}
	c := count(p)

	rec := &recorder{}
	if _, err := Run(context.Background(), time.Second, c.Probes, rec.sink()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	if len(rec.rungs) != 6 {
		t.Fatalf("got %d rungs, want 6", len(rec.rungs))
	}
	for _, id := range []string{RungInternet, RungHTTPS, RungPortal} {
		rung := rec.byID(id)
		if rung.Status != StatusSkipped {
			t.Errorf("%s status = %q, want skipped", id, rung.Status)
		}
		if rung.MS != 0 {
			t.Errorf("%s took %v ms, want 0: it never ran", id, rung.MS)
		}
		if rung.Detail != "not checked" {
			t.Errorf("%s detail = %q", id, rung.Detail)
		}
	}
	// The internet rung dials, and the two web rungs fetch. None of them may
	// have been reached.
	if c.count("dial") != 0 {
		t.Errorf("dial was called %d times after the ladder stopped", c.count("dial"))
	}
	if c.count("get:"+HTTPSTestURL) != 0 || c.count("get:"+PortalTestURL) != 0 {
		t.Error("a web check ran after the ladder stopped")
	}
}

// TestWarnDoesNotStopTheLadder is the other half: a silent gateway is common and
// must not hide the answer.
func TestWarnDoesNotStopTheLadder(t *testing.T) {
	p := healthy()
	p.Ping = func(context.Context, string, time.Duration) (float64, bool) { return 0, false }
	p.Dial = func(_ context.Context, addr string, _ time.Duration) (float64, bool) {
		// The gateway ignores everything; the internet targets answer.
		return 14, strings.HasPrefix(addr, "1.1.1.1") || strings.HasPrefix(addr, "8.8.8.8")
	}

	rec := run(t, p)
	if len(rec.rungs) != 6 {
		t.Fatalf("got %d rungs, want 6", len(rec.rungs))
	}
	if got := rec.byID(RungGateway).Status; got != StatusWarn {
		t.Errorf("gateway status = %q, want warn", got)
	}
	for _, rung := range rec.rungs {
		if rung.Status == StatusSkipped {
			t.Errorf("%s was skipped after a warn, which must not stop the ladder", rung.ID)
		}
	}
}

// TestAllSixEmitted checks the ladder is always complete, in order, whatever
// happened. The literal 6 is written in.
func TestAllSixEmitted(t *testing.T) {
	broken := healthy()
	broken.Adapters = func() (netinfo.Report, error) { return netinfo.Report{}, errors.New("boom") }

	silent := healthy()
	silent.Get = func(context.Context, string, time.Duration) (int, string, string, error) {
		return 0, "", "", errors.New("boom")
	}

	wantIDs := []string{RungAdapter, RungGateway, RungDNS, RungInternet, RungHTTPS, RungPortal}
	wantNames := []string{"Network adapter", "Gateway", "DNS", "The internet", "HTTPS", "Captive portal"}

	for name, p := range map[string]Probes{
		"healthy": healthy(), "no adapters": broken, "no web": silent,
	} {
		rec := run(t, p)
		if len(rec.rungs) != 6 {
			t.Fatalf("%s: got %d rungs, want 6", name, len(rec.rungs))
		}
		seen := map[string]bool{}
		for i, rung := range rec.rungs {
			if rung.ID != wantIDs[i] {
				t.Errorf("%s: rung %d is %q, want %q", name, i, rung.ID, wantIDs[i])
			}
			if rung.Name != wantNames[i] {
				t.Errorf("%s: rung %d is named %q, want %q", name, i, rung.Name, wantNames[i])
			}
			if rung.Step != i+1 {
				t.Errorf("%s: rung %d has step %d, want %d", name, i, rung.Step, i+1)
			}
			if seen[rung.ID] {
				t.Errorf("%s: rung %q appears twice", name, rung.ID)
			}
			seen[rung.ID] = true
		}
	}
}

// TestEveryRungHasAdvice is the "never ship an ok row with nothing in it" rule.
func TestEveryRungHasAdvice(t *testing.T) {
	variants := map[string]func(*Probes){
		"healthy": func(*Probes) {},
		"no adapters": func(p *Probes) {
			p.Adapters = func() (netinfo.Report, error) { return netinfo.Report{}, errors.New("boom") }
		},
		"silent gateway": func(p *Probes) {
			p.Ping = func(context.Context, string, time.Duration) (float64, bool) { return 0, false }
			p.Dial = func(_ context.Context, addr string, _ time.Duration) (float64, bool) {
				return 14, !strings.HasPrefix(addr, "192.168.")
			}
		},
		"broken dns": func(p *Probes) {
			p.Resolve = func(context.Context, string, string, time.Duration) ([]string, error) {
				return nil, errors.New("no")
			}
		},
		"captive portal": func(p *Probes) {
			p.Get = func(_ context.Context, url string, _ time.Duration) (int, string, string, error) {
				if strings.HasPrefix(url, "https://") {
					return 204, "", "", nil
				}
				return 302, "https://wifi.example.com/login", "", nil
			}
		},
	}
	for name, tune := range variants {
		p := healthy()
		tune(&p)
		rec := run(t, p)
		for _, rung := range rec.rungs {
			if strings.TrimSpace(rung.Name) == "" {
				t.Errorf("%s: a rung has no name", name)
			}
			if strings.TrimSpace(rung.Detail) == "" {
				t.Errorf("%s: %s has no detail", name, rung.ID)
			}
			if strings.TrimSpace(rung.Advice) == "" {
				t.Errorf("%s: %s has no advice", name, rung.ID)
			}
		}
	}
}

func TestSummary(t *testing.T) {
	t.Run("everything passed", func(t *testing.T) {
		_, summary := runSummary(t, healthy())
		if summary["firstFailure"] != "" {
			t.Errorf("firstFailure = %v, want empty", summary["firstFailure"])
		}
		if summary["headline"] != "Everything passed. This computer can reach the internet." {
			t.Errorf("headline = %v", summary["headline"])
		}
		if summary["ok"] != 6 {
			t.Errorf("ok = %v, want 6", summary["ok"])
		}
		if summary["failed"] != 0 || summary["skipped"] != 0 || summary["warn"] != 0 {
			t.Errorf("tally = %v", summary)
		}
	})

	t.Run("a captive portal", func(t *testing.T) {
		p := healthy()
		p.Get = func(_ context.Context, url string, _ time.Duration) (int, string, string, error) {
			if strings.HasPrefix(url, "https://") {
				return 204, "", "", nil
			}
			return 302, "https://wifi.example.com/login", "", nil
		}
		_, summary := runSummary(t, p)
		if summary["firstFailure"] != RungPortal {
			t.Errorf("firstFailure = %v, want %q", summary["firstFailure"], RungPortal)
		}
		if summary["headline"] != "Captive portal: A login page is in the way." {
			t.Errorf("headline = %v", summary["headline"])
		}
		if summary["failed"] != 1 {
			t.Errorf("failed = %v, want 1", summary["failed"])
		}
	})

	t.Run("broken dns counts the skipped rungs", func(t *testing.T) {
		p := healthy()
		p.Resolve = func(context.Context, string, string, time.Duration) ([]string, error) {
			return nil, errors.New("no")
		}
		_, summary := runSummary(t, p)
		if summary["firstFailure"] != RungDNS {
			t.Errorf("firstFailure = %v, want %q", summary["firstFailure"], RungDNS)
		}
		if summary["skipped"] != 3 {
			t.Errorf("skipped = %v, want 3", summary["skipped"])
		}
		for _, key := range []string{"ok", "warn", "failed", "skipped", "firstFailure", "headline"} {
			if _, ok := summary[key]; !ok {
				t.Errorf("summary has no %q", key)
			}
		}
	})
}

func TestFirstSentence(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"A login page is in the way. Open a browser.", "A login page is in the way."},
		{"One sentence only.", "One sentence only."},
		{"", ""},
		{"No full stop at all", "No full stop at all"},
	}
	for _, tt := range tests {
		if got := firstSentence(tt.in); got != tt.want {
			t.Errorf("firstSentence(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestProgressMessages(t *testing.T) {
	rec := run(t, healthy())
	want := []string{
		"Checking network adapter", "Checking gateway", "Checking DNS",
		"Checking the internet", "Checking HTTPS", "Checking captive portal",
	}
	for _, message := range want {
		found := false
		for _, got := range rec.progress {
			if got == message {
				found = true
			}
		}
		if !found {
			t.Errorf("no progress message %q, got %v", message, rec.progress)
		}
	}
}

func TestCancelledMidLadder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := healthy()
	c := count(p)
	// Cancel while the DNS rung is being asked for, so the ladder must stop
	// before the three that follow it.
	c.Resolve = func(_ context.Context, _, _ string, _ time.Duration) ([]string, error) {
		c.bump("resolve")
		cancel()
		return []string{"93.184.216.34"}, nil
	}

	rec := &recorder{}
	_, err := Run(ctx, time.Second, c.Probes, rec.sink())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(rec.rungs) >= 6 {
		t.Errorf("got %d rungs after a cancel, want fewer than 6", len(rec.rungs))
	}
	if c.count("get:"+PortalTestURL) != 0 {
		t.Error("the portal check ran after the cancel")
	}
}

// TestDefaultProbesAreWired is the guard that stops a fake-only suite from
// passing while the shipped tool does nothing at all.
func TestDefaultProbesAreWired(t *testing.T) {
	p := DefaultProbes()
	if p.Adapters == nil {
		t.Error("Adapters is nil")
	}
	if p.Ping == nil {
		t.Error("Ping is nil")
	}
	if p.Dial == nil {
		t.Error("Dial is nil")
	}
	if p.Resolve == nil {
		t.Error("Resolve is nil")
	}
	if p.Get == nil {
		t.Error("Get is nil")
	}
}

// TestRealGetAgainstLoopback exercises the default Get probe rather than a fake,
// so its redirect handling and body cap are covered too.
func TestRealGetAgainstLoopback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "success\n")
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://wifi.example.com/login", http.StatusFound)
	})
	mux.HandleFunc("/nocontent", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	get := DefaultProbes().Get

	t.Run("a plain answer", func(t *testing.T) {
		status, _, body, err := get(context.Background(), srv.URL+"/ok", 3*time.Second)
		if err != nil {
			t.Fatalf("get errored: %v", err)
		}
		if status != 200 || !bodyMatches(body, PortalTestBody) {
			t.Errorf("status = %d, body = %q", status, body)
		}
	})

	t.Run("a redirect is reported, not followed", func(t *testing.T) {
		status, location, _, err := get(context.Background(), srv.URL+"/redirect", 3*time.Second)
		if err != nil {
			t.Fatalf("get errored: %v", err)
		}
		if status != 302 {
			t.Errorf("status = %d, want 302: following the redirect would hide the portal", status)
		}
		if location != "https://wifi.example.com/login" {
			t.Errorf("location = %q", location)
		}
	})

	t.Run("204 with an empty body", func(t *testing.T) {
		status, _, body, err := get(context.Background(), srv.URL+"/nocontent", 3*time.Second)
		if err != nil {
			t.Fatalf("get errored: %v", err)
		}
		if status != 204 || body != "" {
			t.Errorf("status = %d, body = %q", status, body)
		}
	})

	t.Run("nothing listening", func(t *testing.T) {
		srv.Close()
		if _, _, _, err := get(context.Background(), srv.URL+"/ok", time.Second); err == nil {
			t.Error("a dead server produced no error")
		}
	})
}

func TestHostOf(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://www.google.com/generate_204", "www.google.com"},
		{"http://detectportal.firefox.com/success.txt", "detectportal.firefox.com"},
		{"http://example.com", "example.com"},
		{"http://example.com?q=1", "example.com"},
		{"example.com", "example.com"},
	}
	for _, tt := range tests {
		if got := hostOf(tt.in); got != tt.want {
			t.Errorf("hostOf(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLowerFirst(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Network adapter", "network adapter"},
		{"Gateway", "gateway"},
		{"The internet", "the internet"},
		{"Captive portal", "captive portal"},
		// All-caps names stay as they are: "checking dNS" reads as a typo.
		{"DNS", "DNS"},
		{"HTTPS", "HTTPS"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := lowerFirst(tt.in); got != tt.want {
			t.Errorf("lowerFirst(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
