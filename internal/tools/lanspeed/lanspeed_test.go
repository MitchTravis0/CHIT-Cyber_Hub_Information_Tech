package lanspeed

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"chit/internal/tools/filedrop"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name             string
		in               Params
		wantPort, wantMB int
		wantErr          string
	}{
		// The literals below are written in on purpose. Reading them out of the
		// constants would prove only that a constant equals itself, and both
		// bounds appear in user-facing sentences.
		{"both zero take the defaults", Params{}, 8740, 200, ""},
		{"lowest allowed port", Params{Port: 1024, SizeMB: 200}, 1024, 200, ""},
		{"highest allowed port", Params{Port: 65535, SizeMB: 200}, 65535, 200, ""},
		{"smallest allowed size", Params{Port: 8740, SizeMB: 10}, 8740, 10, ""},
		{"largest allowed size", Params{Port: 8740, SizeMB: 4096}, 8740, 4096, ""},
		{
			"one below the port floor", Params{Port: 1023}, 0, 0,
			"The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.",
		},
		{
			"one above the port ceiling", Params{Port: 65536}, 0, 0,
			"The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.",
		},
		{
			"one below the smallest size", Params{SizeMB: 9}, 0, 0,
			"The test size must be between 10 MB and 4096 MB. 200 MB is enough to tell a gigabit link from a 100 Mbps one.",
		},
		{
			"one above the largest size", Params{SizeMB: 4097}, 0, 0,
			"The test size must be between 10 MB and 4096 MB. 200 MB is enough to tell a gigabit link from a 100 Mbps one.",
		},
		{
			"negative size", Params{SizeMB: -1}, 0, 0,
			"The test size must be between 10 MB and 4096 MB. 200 MB is enough to tell a gigabit link from a 100 Mbps one.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := tt.in.validate()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("wanted an error, got none")
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("message\n got %q\nwant %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts.port != tt.wantPort || opts.sizeMB != tt.wantMB {
				t.Errorf("got port %d size %d, want %d and %d", opts.port, opts.sizeMB, tt.wantPort, tt.wantMB)
			}
		})
	}
}

type goldenReading struct {
	Mbps    float64 `json:"mbps"`
	Reading string  `json:"reading"`
}

// TestReading reads the same golden file the TypeScript suite reads, so the two
// implementations of this ladder cannot drift apart.
func TestReading(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/lanspeed-readings.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Cases []goldenReading `json:"cases"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Cases) < 17 {
		t.Fatalf("the golden file has only %d cases; both sides of every threshold must be in it", len(golden.Cases))
	}
	for _, c := range golden.Cases {
		if got := Reading(c.Mbps); got != c.Reading {
			t.Errorf("Reading(%v)\n got %q\nwant %q", c.Mbps, got, c.Reading)
		}
	}
}

func TestMbpsArithmetic(t *testing.T) {
	// One mebibyte in exactly one second. Megabits are decimal, mebibytes are
	// binary: 1048576 * 8 / 1e6 = 8.388608. The literal is computed from that
	// rule, so swapping 1e6 for a shift, or bits for bytes, fails here.
	got := Mbps(1<<20, 1)
	if math.Abs(got-8.388608) > 1e-9 {
		t.Errorf("Mbps(1 MiB, 1 s) = %v, want 8.388608", got)
	}
	// 200 MiB in 2.2 seconds, the shape of a real gigabit pull.
	if got := Mbps(200<<20, 2.2); math.Abs(got-762.6) > 0.1 {
		t.Errorf("Mbps(200 MiB, 2.2 s) = %v, want about 762.6", got)
	}
	if Mbps(0, 1) != 0 {
		t.Error("no bytes must measure 0, not a division")
	}
	if Mbps(1<<20, 0) != 0 {
		t.Error("no time must measure 0, not an infinity")
	}
	if Mbps(1<<20, -1) != 0 {
		t.Error("negative time must measure 0")
	}
}

func TestSessionNote(t *testing.T) {
	none := sessionNote(0, 0)
	if !strings.Contains(none, "Nothing pulled the test file") {
		t.Errorf("empty-session note is wrong: %q", none)
	}
	some := sessionNote(2, 1)
	if !strings.Contains(some, "approximate") {
		t.Errorf("every finished session must call its numbers approximate: %q", some)
	}
	if !strings.Contains(some, "from this machine to the other one only") {
		t.Errorf("the note must say which direction was measured: %q", some)
	}

	// Found by running the tool: pulling the link on the machine serving it
	// reports tens of thousands of Mbps, which is memory speed and not a network
	// figure at all.
	self := sessionNote(2, 2)
	if !strings.Contains(self, "none of these numbers describes the network") {
		t.Errorf("a session where only this machine pulled must say so: %q", self)
	}
}

func TestSummaryFor(t *testing.T) {
	s := summaryFor("http://10.0.0.5:8740/t/abc", 8740, 200, 2, 0, 742.1, 400<<20)
	for _, key := range []string{"url", "port", "sizeMb", "pulls", "bestMbps", "bytesOut", "note"} {
		if _, ok := s[key]; !ok {
			t.Errorf("summary is missing key %q", key)
		}
	}
	if s["port"] != 8740 || s["sizeMb"] != 200 || s["pulls"] != 2 {
		t.Errorf("counts wrong: %v", s)
	}
	if s["bestMbps"] != 742.1 {
		t.Errorf("bestMbps = %v", s["bestMbps"])
	}
}

func TestSummaryBestIsTheMaximumNotTheLast(t *testing.T) {
	sess := newSession(options{port: 8740, sizeMB: 10}, "tok", "http://x", nil, Sink{})
	sess.record(Pull{Mbps: 742.1, Bytes: 10 << 20})
	sess.record(Pull{Mbps: 12.5, Bytes: 10 << 20})

	s := sess.summary()
	if s["bestMbps"] != 742.1 {
		t.Errorf("bestMbps = %v, want the maximum 742.1 and not the last 12.5", s["bestMbps"])
	}
	if s["bytesOut"] != int64(20<<20) {
		t.Errorf("bytesOut = %v, want the sum", s["bytesOut"])
	}
}

func TestNoSliceIsNil(t *testing.T) {
	data, err := json.Marshal(summaryFor("http://x", 8740, 200, 0, 0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Fatalf("summary marshals a null: %s", data)
	}
}

func TestURLFor(t *testing.T) {
	if got := URLFor("10.0.0.5", 8740, "abc123"); got != "http://10.0.0.5:8740/t/abc123" {
		t.Errorf("URLFor = %q", got)
	}
	if got := URLFor("fe80::1", 8740, "abc123"); got != "http://[fe80::1]:8740/t/abc123" {
		t.Errorf("an IPv6 literal must be bracketed, got %q", got)
	}
}

func TestPatternIsNotAllZeroes(t *testing.T) {
	// A run of zeroes is what a WAN accelerator or a proxy would collapse, and
	// the tool would then be measuring the middlebox.
	seen := map[byte]bool{}
	for _, b := range block {
		seen[b] = true
	}
	if len(seen) < 200 {
		t.Fatalf("the block has only %d distinct byte values, want at least 200", len(seen))
	}
	if len(block) != 1<<20 {
		t.Fatalf("block is %d bytes, want 1 MiB", len(block))
	}
}

func TestIndexHTMLIsInert(t *testing.T) {
	page := IndexHTML("a1b2c3d4e5f6a7b8", 200, "http://10.0.0.5:8740/t/a1b2c3d4e5f6a7b8")

	for _, banned := range []string{"<script", "<img", "<iframe", "<link", "javascript:", "onload="} {
		if strings.Contains(strings.ToLower(page), banned) {
			t.Errorf("the served page contains %q", banned)
		}
	}
	// The only absolute URL on the page is this session's own link, inside the
	// curl line. Nothing is fetched from anywhere.
	for _, line := range strings.Split(page, "\n") {
		at := strings.Index(line, "http")
		if at < 0 {
			continue
		}
		if !strings.Contains(line, "10.0.0.5:8740") {
			t.Errorf("the page reaches outside this session: %q", line)
		}
	}
	if err := xml.Unmarshal([]byte(strings.TrimPrefix(page, "<!doctype html>\n")), new(any)); err != nil {
		t.Errorf("the page is not well formed: %v", err)
	}
	if !strings.Contains(page, "Start the test (200 MB)") {
		t.Error("the page does not offer the download")
	}
	if !strings.Contains(page, "/t/a1b2c3d4e5f6a7b8/dl") {
		t.Error("the download link is wrong")
	}
}

func TestIndexHTMLEscapes(t *testing.T) {
	// The token is hex by construction, so this pins the escaping rather than a
	// reachable bug. A future change that builds a token differently must not
	// silently open an injection.
	page := IndexHTML(`" onerror="alert(1)`, 200, "http://10.0.0.5:8740/t/x")
	if strings.Contains(page, `onerror="alert(1)`) {
		t.Fatalf("the token broke out of its attribute:\n%s", page)
	}
	if !strings.Contains(page, "&#34;") && !strings.Contains(page, "&quot;") {
		t.Error("the quote was not escaped at all")
	}
}

func TestCurlFor(t *testing.T) {
	got := curlFor("http://10.0.0.5:8740/t/abc")
	if got != "curl -o /dev/null http://10.0.0.5:8740/t/abc/dl" {
		t.Errorf("curlFor = %q", got)
	}
}

// recorder collects what a session emitted. The handler runs on its own
// goroutine, so a plain slice read straight after a request passes about nine
// runs in ten. internal/tools/filedrop shipped exactly that flake once.
type recorder struct {
	mu    sync.Mutex
	pulls []Pull
}

func (r *recorder) sink() Sink {
	return Sink{
		Emit: func(p Pull) {
			r.mu.Lock()
			r.pulls = append(r.pulls, p)
			r.mu.Unlock()
		},
		Progress: func(int64, int64, string) {},
	}
}

func (r *recorder) wait(t *testing.T, want int) []Pull {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		n := len(r.pulls)
		r.mu.Unlock()
		if n >= want {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Pull, len(r.pulls))
	copy(out, r.pulls)
	return out
}

func startLoopback(t *testing.T, sizeMB int) (*session, *recorder, string, context.CancelFunc, chan error) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()

	rec := &recorder{}
	url := "http://" + address + "/t/testtoken"
	sess := newSession(options{port: 1024, sizeMB: sizeMB}, "testtoken", url, localSet(nil), rec.sink())

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- Serve(ctx, listener, sess) }()

	return sess, rec, "http://" + address, cancel, errc
}

func TestServeStreamsTheRightNumberOfBytes(t *testing.T) {
	_, _, base, cancel, errc := startLoopback(t, 10)
	defer func() { cancel(); <-errc }()

	resp, err := http.Get(base + "/t/testtoken/dl")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	want := int64(10) << 20
	if resp.ContentLength != want {
		t.Errorf("Content-Length = %d, want %d", resp.ContentLength, want)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Errorf("body was %d bytes, want exactly %d", n, want)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "chit-speedtest.bin") {
		t.Errorf("Content-Disposition = %q", got)
	}
}

func TestServeRecordsAPull(t *testing.T) {
	_, rec, base, cancel, errc := startLoopback(t, 10)
	defer func() { cancel(); <-errc }()

	resp, err := http.Get(base + "/t/testtoken/dl")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	pulls := rec.wait(t, 1)
	if len(pulls) != 1 {
		t.Fatalf("got %d pulls, want exactly 1", len(pulls))
	}
	got := pulls[0]
	if got.Status != "ok" {
		t.Errorf("status = %q, want ok", got.Status)
	}
	if got.Bytes != int64(10)<<20 {
		t.Errorf("bytes = %d", got.Bytes)
	}
	if got.Seconds <= 0 || got.Mbps <= 0 {
		t.Errorf("seconds = %v, mbps = %v, both must be positive", got.Seconds, got.Mbps)
	}
	// The pull came from 127.0.0.1, which localSet always holds, so the reading
	// must be the same-machine sentence rather than a speed verdict.
	if got.Reading != SameMachineReading {
		t.Errorf("reading = %q, want the same-machine sentence", got.Reading)
	}
	if got.Peer != "127.0.0.1" {
		t.Errorf("peer = %q", got.Peer)
	}
}

func TestServeRecordsAnAbandonedPull(t *testing.T) {
	_, rec, base, cancel, errc := startLoopback(t, 64)
	defer func() { cancel(); <-errc }()

	resp, err := http.Get(base + "/t/testtoken/dl")
	if err != nil {
		t.Fatal(err)
	}
	// Read one mebibyte then hang up, which is what closing the browser tab does.
	io.CopyN(io.Discard, resp.Body, 1<<20)
	resp.Body.Close()

	pulls := rec.wait(t, 1)
	if len(pulls) != 1 {
		t.Fatalf("got %d pulls, want exactly 1", len(pulls))
	}
	if pulls[0].Status != "stopped" {
		t.Errorf("status = %q, want stopped", pulls[0].Status)
	}
	if pulls[0].Bytes >= int64(64)<<20 {
		t.Errorf("bytes = %d, want fewer than the whole stream", pulls[0].Bytes)
	}
}

func TestServeRefusesAWrongToken(t *testing.T) {
	_, rec, base, cancel, errc := startLoopback(t, 10)
	defer func() { cancel(); <-errc }()

	for _, path := range []string{
		"/t/wrongtoken",
		"/t/wrongtoken/dl",
		"/t/testtoken/nope",
		"/",
		"/dl",
	} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s returned %d, want 404", path, resp.StatusCode)
		}
		if string(body) != "Nothing here.\n" {
			t.Errorf("%s body = %q, want the flat 404 that reveals nothing", path, body)
		}
	}

	// A traversal attempt that http.Client will not canonicalise away.
	req, _ := http.NewRequest(http.MethodGet, base+"/t/testtoken/../../etc/passwd", nil)
	req.URL.Opaque = "/t/testtoken/../../etc/passwd"
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("a traversal path returned %d, want 404", resp.StatusCode)
		}
	}

	if pulls := rec.wait(t, 0); len(pulls) != 0 {
		t.Errorf("a refused request emitted %d pulls, want none", len(pulls))
	}
}

func TestServeStopsOnCancelAndReleasesThePort(t *testing.T) {
	_, _, base, cancel, errc := startLoopback(t, 10)
	address := strings.TrimPrefix(base, "http://")

	cancel()
	select {
	case err := <-errc:
		if err != context.Canceled {
			t.Fatalf("Serve returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return within 5 seconds of cancel")
	}

	again, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("the port was still held after Stop: %v", err)
	}
	again.Close()
}

func TestServeCapsEmittedPulls(t *testing.T) {
	sess := newSession(options{port: 8740, sizeMB: 10}, "tok", "http://x", nil, Sink{})
	rec := &recorder{}
	sess.out = rec.sink()

	for i := 0; i < maxPulls+5; i++ {
		sess.record(Pull{Peer: "10.0.0.7", Bytes: 1 << 20, Mbps: 100})
	}

	// 100 is written in rather than read from maxPulls: a test that reads the cap
	// it is checking pins nothing.
	if got := len(rec.wait(t, 100)); got != 100 {
		t.Errorf("emitted %d rows, want exactly 100", got)
	}
	if s := sess.summary(); s["pulls"] != maxPulls+5 {
		t.Errorf("the tally stopped counting at the cap: %v", s["pulls"])
	}
}

func TestNoFileIsEverWritten(t *testing.T) {
	// The whole point of the tool is that nothing lands on either machine's disk.
	for _, name := range []string{"lanspeed.go", "serve.go", "page.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"os.Create", "os.WriteFile", "os.OpenFile", "os.MkdirAll"} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s calls %s; this tool must never write a file", name, banned)
			}
		}
	}
}

func TestLocalSet(t *testing.T) {
	local := localSet([]filedrop.Address{{IP: "10.0.0.5"}, {IP: "192.168.1.20"}})
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.5", "192.168.1.20"} {
		if !local[ip] {
			t.Errorf("%s should count as this same machine", ip)
		}
	}
	if local["10.0.0.9"] {
		t.Error("another machine's address must not count as this one")
	}
	if len(localSet(nil)) != 2 {
		t.Error("loopback must be in the set even with no adapters")
	}
}

// TestSameMachineReading covers both sides of the guard added after running the
// tool: a pull from this machine is labelled, a pull from anywhere else gets the
// real speed verdict. Loopback is the only peer a test can produce, so the two
// cases are made by varying the address set rather than the peer.
func TestSameMachineReading(t *testing.T) {
	t.Run("from this machine", func(t *testing.T) {
		_, rec, base, cancel, errc := startLoopback(t, 10)
		defer func() { cancel(); <-errc }()

		resp, err := http.Get(base + "/t/testtoken/dl")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		pulls := rec.wait(t, 1)
		if len(pulls) != 1 || pulls[0].Reading != SameMachineReading {
			t.Fatalf("reading = %q, want the same-machine sentence", pulls[0].Reading)
		}
	})

	t.Run("from another machine", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		rec := &recorder{}
		// An empty address set makes 127.0.0.1 look like a different machine,
		// which is the only way a test can reach the other branch.
		sess := newSession(options{port: 1024, sizeMB: 10}, "testtoken", "http://x",
			map[string]bool{}, rec.sink())
		ctx, cancel := context.WithCancel(context.Background())
		errc := make(chan error, 1)
		go func() { errc <- Serve(ctx, listener, sess) }()
		defer func() { cancel(); <-errc }()

		resp, err := http.Get("http://" + listener.Addr().String() + "/t/testtoken/dl")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		pulls := rec.wait(t, 1)
		if len(pulls) != 1 {
			t.Fatalf("got %d pulls", len(pulls))
		}
		if pulls[0].Reading == SameMachineReading {
			t.Fatal("a peer that is not this machine must get a speed verdict")
		}
		if pulls[0].Reading != Reading(pulls[0].Mbps) {
			t.Errorf("reading = %q, want %q", pulls[0].Reading, Reading(pulls[0].Mbps))
		}
	})
}
