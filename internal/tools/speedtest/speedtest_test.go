package speedtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"chit/internal/core"
)

func TestMbps(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		d     time.Duration
		want  float64
	}{
		{"a megabyte a second", 1_000_000, time.Second, 8},
		{"no time", 1_000_000, 0, 0},
		{"negative time", 1_000_000, -time.Second, 0},
		{"no bytes", 0, time.Second, 0},
		{"half a second", 500_000, 500 * time.Millisecond, 8},
		{"a quarter second", 250_000, 250 * time.Millisecond, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mbps(tt.bytes, tt.d)
			if !closeTo(got, tt.want) {
				t.Errorf("mbps(%d, %s) = %v, want %v", tt.bytes, tt.d, got, tt.want)
			}
		})
	}
}

func TestJitter(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want float64
	}{
		{"empty", nil, 0},
		{"one probe", []float64{12}, 0},
		{"flat", []float64{10, 10, 10, 10}, 0},
		{"alternating", []float64{10, 20, 10, 20}, 10},
		{"mixed", []float64{10, 13, 11}, 2.5},
		{"descending", []float64{30, 20, 10}, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jitter(tt.in); !closeTo(got, tt.want) {
				t.Errorf("jitter(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMeanMin(t *testing.T) {
	tests := []struct {
		name     string
		in       []float64
		wantMean float64
		wantMin  float64
	}{
		{"empty", nil, 0, 0},
		{"one", []float64{7.5}, 7.5, 7.5},
		{"several", []float64{1, 2, 3}, 2, 1},
		{"smallest last", []float64{9, 4, 0.5}, 4.5, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := meanF(tt.in); !closeTo(got, tt.wantMean) {
				t.Errorf("meanF(%v) = %v, want %v", tt.in, got, tt.wantMean)
			}
			if got := minF(tt.in); !closeTo(got, tt.wantMin) {
				t.Errorf("minF(%v) = %v, want %v", tt.in, got, tt.wantMin)
			}
		})
	}
}

func TestParamsValidation(t *testing.T) {
	durationMsg := func(d int) string {
		return fmt.Sprintf("Each part of the test can run for 3 to 30 seconds. %d is outside that.", d)
	}
	streamsMsg := func(s int) string {
		return fmt.Sprintf("Use between 1 and 8 connections at a time. %d is outside that.", s)
	}

	tests := []struct {
		name    string
		params  Params
		wantErr string
	}{
		{"duration 0 means default", Params{DurationSec: 0}, ""},
		{"duration 2 too short", Params{DurationSec: 2}, durationMsg(2)},
		{"duration 3 is the minimum", Params{DurationSec: 3}, ""},
		{"duration 8", Params{DurationSec: 8}, ""},
		{"duration 30 is the maximum", Params{DurationSec: 30}, ""},
		{"duration 31 too long", Params{DurationSec: 31}, durationMsg(31)},
		{"duration -1", Params{DurationSec: -1}, durationMsg(-1)},
		{"streams 0 means default", Params{Streams: 0}, ""},
		{"streams 1", Params{Streams: 1}, ""},
		{"streams 4", Params{Streams: 4}, ""},
		{"streams 8 is the maximum", Params{Streams: 8}, ""},
		{"streams 9 too many", Params{Streams: 9}, streamsMsg(9)},
		{"streams -1", Params{Streams: -1}, streamsMsg(-1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.params.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() = %v, want no error", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate() accepted %+v", tt.params)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("message = %q, want %q", err.Error(), tt.wantErr)
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %s, want %s", code, core.CodeInvalidInput)
			}
		})
	}
}

func TestParamsDefaults(t *testing.T) {
	got, err := Params{}.validate()
	if err != nil {
		t.Fatalf("validate() = %v", err)
	}
	if got.DurationSec != 8 {
		t.Errorf("DurationSec = %d, want 8", got.DurationSec)
	}
	if got.Streams != 4 {
		t.Errorf("Streams = %d, want 4", got.Streams)
	}
	if got.SkipUpload {
		t.Error("SkipUpload = true, want false")
	}

	kept, err := Params{DurationSec: 3, Streams: 1, SkipUpload: true}.validate()
	if err != nil {
		t.Fatalf("validate() = %v", err)
	}
	if kept.DurationSec != 3 || kept.Streams != 1 || !kept.SkipUpload {
		t.Errorf("validate() changed a filled-in Params: %+v", kept)
	}
}

func TestDownloadPhaseCountsBytes(t *testing.T) {
	body := make([]byte, 1<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	_, _, out := startPhase(transferOpts{
		phase:    phaseDownload,
		url:      srv.URL + "/__down?bytes=1048576",
		message:  "Measuring download speed",
		client:   srv.Client(),
		duration: 300 * time.Millisecond,
		streams:  1,
		totalSec: 1,
	})

	got := waitPhase(t, out, 10*time.Second)
	if got.err != nil {
		t.Fatalf("download phase returned %v", got.err)
	}
	if got.res.Bytes <= 0 {
		t.Fatalf("counted %d bytes, want more than 0", got.res.Bytes)
	}
	if got.res.Mbps <= 0 {
		t.Fatalf("headline = %v Mbps, want more than 0", got.res.Mbps)
	}
	if got.res.Samples < 1 {
		t.Errorf("samples = %d, want at least the final one", got.res.Samples)
	}
}

func TestUploadPhaseSendsBytes(t *testing.T) {
	var received int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		atomic.AddInt64(&received, n)
	}))
	t.Cleanup(srv.Close)

	_, _, out := startPhase(transferOpts{
		phase:    phaseUpload,
		url:      srv.URL + "/__up",
		message:  "Measuring upload speed",
		client:   srv.Client(),
		duration: 300 * time.Millisecond,
		streams:  1,
		totalSec: 1,
		upload:   true,
	})

	got := waitPhase(t, out, 10*time.Second)
	if got.err != nil {
		t.Fatalf("upload phase returned %v", got.err)
	}
	// Close waits for the handler, so the counter is settled before it is read.
	srv.Close()
	if atomic.LoadInt64(&received) <= 0 {
		t.Fatalf("the server received %d bytes, want more than 0", received)
	}
	if got.res.Bytes <= 0 {
		t.Fatalf("counted %d bytes sent, want more than 0", got.res.Bytes)
	}
}

func TestPhaseHonoursCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 4096)
		for r.Context().Err() == nil {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)

	m, id, out := startPhase(transferOpts{
		phase:    phaseDownload,
		url:      srv.URL + "/__down?bytes=1048576",
		message:  "Measuring download speed",
		client:   srv.Client(),
		duration: 5 * time.Second,
		streams:  1,
		totalSec: 5,
	})

	time.Sleep(100 * time.Millisecond)
	if err := m.Cancel(id); err != nil {
		t.Fatalf("Cancel returned %v", err)
	}

	started := time.Now()
	got := waitPhase(t, out, time.Second)
	if took := time.Since(started); took > time.Second {
		t.Errorf("the phase took %s to stop, want under 1 s", took)
	}
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", got.err)
	}
}

func TestRampCorrection(t *testing.T) {
	// A synthetic tick series: 1 Mbps for the first second, then 10 Mbps.
	type tick struct {
		bytes   int64
		elapsed time.Duration
	}
	ticks := []tick{}
	var total int64
	for ms := 200; ms <= 1000; ms += 200 {
		total += 25_000 // 1 Mbps over each 200 ms window
		ticks = append(ticks, tick{total, time.Duration(ms) * time.Millisecond})
	}
	rampTotal := total
	for ms := 1200; ms <= 3000; ms += 200 {
		total += 250_000 // 10 Mbps over each 200 ms window
		ticks = append(ticks, tick{total, time.Duration(ms) * time.Millisecond})
	}

	var r ramp
	for _, tk := range ticks {
		r.mark(tk.bytes, tk.elapsed)
	}
	if !r.set {
		t.Fatal("the ramp point was never marked")
	}
	if r.bytes != rampTotal {
		t.Errorf("ramp bytes = %d, want %d", r.bytes, rampTotal)
	}

	last := ticks[len(ticks)-1]
	if got := r.headline(last.bytes, last.elapsed); !closeTo(got, 10) {
		t.Errorf("headline = %v Mbps, want 10 (the speed after the ramp)", got)
	}
	if whole := mbps(last.bytes, last.elapsed); closeTo(whole, 10) {
		t.Fatalf("the whole-phase rate is also %v Mbps, so the test proves nothing", whole)
	}

	// A phase that ended before the ramp point falls back to the whole phase.
	var short ramp
	for ms := 200; ms <= 800; ms += 200 {
		short.mark(100_000, time.Duration(ms)*time.Millisecond)
	}
	if short.set {
		t.Fatal("the ramp point was marked before the first second")
	}
	if got := short.headline(100_000, 800*time.Millisecond); !closeTo(got, 1) {
		t.Errorf("fallback headline = %v Mbps, want 1", got)
	}
	if got := short.headline(0, 0); got != 0 {
		t.Errorf("headline of an empty phase = %v, want 0", got)
	}
}

func TestParseMeta(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"a data centre", `{"colo":"LHR"}`, "Cloudflare LHR"},
		{"other fields ignored", `{"clientIp":"203.0.113.9","colo":"AMS","asn":64500}`, "Cloudflare AMS"},
		{"no colo", `{"clientIp":"203.0.113.9"}`, ""},
		{"empty colo", `{"colo":""}`, ""},
		{"bad json", `{"colo":`, ""},
		{"empty body", "", ""},
		{"html error page", "<html>blocked</html>", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseMeta([]byte(tt.body)); got != tt.want {
				t.Errorf("parseMeta(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

type phaseOutcome struct {
	res phaseResult
	err error
}

// startPhase drives a transfer phase through a real JobManager, which is the
// only way to get a JobContext. No Wails context is set, so the emitted events
// go nowhere and the phase's return value is what the test asserts on.
func startPhase(o transferOpts) (*core.JobManager, string, chan phaseOutcome) {
	m := core.NewJobManager()
	out := make(chan phaseOutcome, 1)
	id := m.Start(JobKind, o.totalSec, func(jc *core.JobContext) error {
		res, err := runTransfer(jc, o)
		out <- phaseOutcome{res, err}
		return err
	})
	return m, id, out
}

func waitPhase(t *testing.T, out chan phaseOutcome, within time.Duration) phaseOutcome {
	t.Helper()
	select {
	case got := <-out:
		return got
	case <-time.After(within):
		t.Fatal("the phase did not finish")
		return phaseOutcome{}
	}
}

func closeTo(got, want float64) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d < 1e-6
}
