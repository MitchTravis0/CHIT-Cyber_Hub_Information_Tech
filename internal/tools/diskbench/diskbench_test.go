package diskbench

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestValidate(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		in      Params
		wantMB  int
		wantErr string
	}{
		// 256, 16 and 4096 are written in on purpose. Reading them out of the
		// constants would prove only that a constant equals itself, and all three
		// appear in a user-facing sentence.
		{"zero size takes the default", Params{Path: dir}, 256, ""},
		{"smallest allowed", Params{Path: dir, SizeMB: 16}, 16, ""},
		{"largest allowed", Params{Path: dir, SizeMB: 4096}, 4096, ""},
		{
			"one below the smallest", Params{Path: dir, SizeMB: 15}, 0,
			"The test size must be between 16 MB and 4096 MB. 256 MB is enough to tell a healthy drive from a failing one.",
		},
		{
			"one above the largest", Params{Path: dir, SizeMB: 4097}, 0,
			"The test size must be between 16 MB and 4096 MB. 256 MB is enough to tell a healthy drive from a failing one.",
		},
		{
			"negative", Params{Path: dir, SizeMB: -1}, 0,
			"The test size must be between 16 MB and 4096 MB. 256 MB is enough to tell a healthy drive from a failing one.",
		},
		{
			"empty path", Params{}, 0,
			"Choose a folder to scan, or type the full path to it.",
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
			if opts.sizeMB != tt.wantMB {
				t.Errorf("size = %d, want %d", opts.sizeMB, tt.wantMB)
			}
		})
	}
}

func TestValidateRejectsAFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Params{Path: file}).validate()
	if err == nil {
		t.Fatal("a file must be refused, not written into")
	}
	if !strings.Contains(err.Error(), "is a file, not a folder") {
		t.Errorf("message = %q", err.Error())
	}
}

func TestVerdictFor(t *testing.T) {
	tests := []struct {
		name        string
		read, write float64
		want        string
	}{
		{"nvme", 2000, 1500, "This looks like an SSD or an NVMe drive."},
		{"at the ssd boundary", 400, 400, "This looks like an SSD or an NVMe drive."},
		{"just below it", 399.9, 500, "Fast enough for everyday work. An older SSD, or a fast network share."},
		{"at the everyday boundary", 150, 150, "Fast enough for everyday work. An older SSD, or a fast network share."},
		{"just below it", 149.9, 200, "About what a healthy spinning hard disk does."},
		{"at the spinning boundary", 60, 60, "About what a healthy spinning hard disk does."},
		{"just below it", 59.9, 100, "Slow. A spinning disk that is busy or nearly full, a USB 2.0 stick, or a network share."},
		{"at the slow boundary", 20, 20, "Slow. A spinning disk that is busy or nearly full, a USB 2.0 stick, or a network share."},
		{"just below it", 19.9, 50, "Very slow. Check the drive's health before blaming the machine."},
		{"nothing measured", 0, 0, "Too little was transferred to measure."},
		{"a negative", -1, -1, "Too little was transferred to measure."},
		// The one that matters: a drive that reads like an NVMe and writes like a
		// dying stick is a dying stick, and the verdict must say so.
		{"the slower figure decides", 2000, 30, "Slow. A spinning disk that is busy or nearly full, a USB 2.0 stick, or a network share."},
		{"and in the other order", 30, 2000, "Slow. A spinning disk that is busy or nearly full, a USB 2.0 stick, or a network share."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := verdictFor(tt.read, tt.write); got != tt.want {
				t.Errorf("verdictFor(%v,%v)\n got %q\nwant %q", tt.read, tt.write, got, tt.want)
			}
		})
	}
}

func TestSessionNote(t *testing.T) {
	yes := sessionNote(true)
	if !strings.Contains(yes, "approximate") {
		t.Errorf("every note must call the figures approximate: %q", yes)
	}
	if strings.Contains(yes, "from memory") {
		t.Errorf("a bypassed cache must not carry the memory caveat: %q", yes)
	}

	no := sessionNote(false)
	if !strings.Contains(no, "coming from memory rather than from the drive") {
		t.Errorf("without a bypass the note must say the read may be memory: %q", no)
	}
	if !strings.Contains(no, "Trust the write figure more") {
		t.Errorf("the note must say which figure to believe: %q", no)
	}
}

func TestMBpsArithmetic(t *testing.T) {
	// 4 MiB in exactly one second. Megabytes are decimal here (every drive
	// datasheet uses them), mebibytes are binary: 4194304 / 1e6 = 4.194304. The
	// literal comes from that rule, so a MiB/MB slip fails.
	if got := rate(4<<20, 1); math.Abs(got-4.194304) > 1e-9 {
		t.Errorf("rate(4 MiB, 1 s) = %v, want 4.194304", got)
	}
	if got := (phaseResult{bytes: 256 << 20, seconds: 0.5}).mbps(); math.Abs(got-536.870912) > 1e-6 {
		t.Errorf("256 MiB in half a second = %v, want 536.870912", got)
	}
	if rate(0, 1) != 0 || rate(1<<20, 0) != 0 || rate(1<<20, -1) != 0 {
		t.Error("no bytes, no time and negative time must all measure 0")
	}
}

func TestGroupSize(t *testing.T) {
	tests := map[int]int{
		1: 1, 4: 1, 32: 1, // a short run emits one sample per block
		33:   2,  // 33 blocks over at most 32 samples
		64:   2,  // a 256 MB run
		1024: 32, // a 4096 MB run
	}
	for blocks, want := range tests {
		if got := groupSize(blocks); got != want {
			t.Errorf("groupSize(%d) = %d, want %d", blocks, got, want)
		}
	}
}

func TestPatternIsNotAllZeroes(t *testing.T) {
	// A run of zeroes is what a compressing or deduplicating filesystem would
	// store as almost nothing, and the write figure would be fiction.
	seen := map[byte]bool{}
	for _, b := range pattern {
		seen[b] = true
	}
	if len(seen) < 200 {
		t.Fatalf("the block has only %d distinct byte values, want at least 200", len(seen))
	}
	if len(pattern) != 4<<20 {
		t.Fatalf("the block is %d bytes, want 4 MiB", len(pattern))
	}
}

func TestReadBufferLength(t *testing.T) {
	// On Windows this buffer is a slice of a larger, aligned allocation, so its
	// length is the thing that could silently come back wrong.
	for _, n := range []int{4 << 20, 1 << 20, 4096} {
		if got := len(readBuffer(n)); got != n {
			t.Errorf("len(readBuffer(%d)) = %d", n, got)
		}
	}
}

func TestOpenUncachedRoundTrips(t *testing.T) {
	// The only thing that proves the Windows alignment path is not silently
	// truncating, once somebody runs the suite on Windows.
	dir := t.TempDir()
	path := filepath.Join(dir, "round-trip.bin")
	if err := os.WriteFile(path, pattern, 0o600); err != nil {
		t.Fatal(err)
	}

	file, bypassed, err := openUncached(path, int64(len(pattern)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	t.Logf("this platform bypassed the cache: %v", bypassed)

	buf := readBuffer(len(pattern))
	n, err := file.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if buf[i] != pattern[i] {
			t.Fatalf("byte %d came back as %d, want %d", i, buf[i], pattern[i])
		}
	}
	if n == 0 {
		t.Fatal("nothing was read")
	}
}

// recorder collects what a run emitted.
type recorder struct {
	mu      sync.Mutex
	samples []Sample
}

func (r *recorder) sink() Sink {
	return Sink{
		Emit: func(s Sample) {
			r.mu.Lock()
			r.samples = append(r.samples, s)
			r.mu.Unlock()
		},
		Progress: func(int, string) {},
	}
}

func (r *recorder) all() []Sample {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Sample, len(r.samples))
	copy(out, r.samples)
	return out
}

func phaseOf(samples []Sample, phase string) []Sample {
	var out []Sample
	for _, s := range samples {
		if s.Phase == phase {
			out = append(out, s)
		}
	}
	return out
}

// leftovers is what the folder holds afterwards. The tool's headline promise is
// that this is always empty.
func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestRunWritesReadsAndRemoves(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}

	summary, err := Run(context.Background(), options{root: dir, sizeMB: 16}, rec.sink())
	if err != nil {
		t.Fatal(err)
	}

	if summary["writeMbps"].(float64) <= 0 || summary["readMbps"].(float64) <= 0 {
		t.Errorf("both figures must be positive: %v", summary)
	}
	if summary["sizeMb"] != 16 || summary["path"] != dir {
		t.Errorf("summary = %v", summary)
	}
	if summary["verdict"] == "" || summary["note"] == "" {
		t.Error("the verdict and the note are never allowed to be empty")
	}
	if _, ok := summary["cacheBypassed"].(bool); !ok {
		t.Error("cacheBypassed must be a bool the page can read")
	}

	samples := rec.all()
	if len(phaseOf(samples, PhaseWrite)) == 0 || len(phaseOf(samples, PhaseRead)) == 0 {
		t.Fatalf("both phases must emit samples, got %d", len(samples))
	}
	if names := leftovers(t, dir); len(names) != 0 {
		t.Fatalf("the folder still holds %v", names)
	}
}

func TestRunRemovesTheFileOnCancel(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel as soon as the first block is on its way out, so the cancel lands
	// inside the write rather than after it.
	sink := Sink{
		Emit:     func(Sample) { cancel() },
		Progress: func(int, string) {},
	}

	_, err := Run(ctx, options{root: dir, sizeMB: 512}, sink)
	if err != context.Canceled {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	if names := leftovers(t, dir); len(names) != 0 {
		t.Fatalf("a cancelled run left %v behind", names)
	}
}

func TestRunRemovesTheFileOnError(t *testing.T) {
	dir := t.TempDir()
	// A context that is already dead makes the very first block fail, which is
	// the earliest error path there is.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Run(ctx, options{root: dir, sizeMB: 16}, Sink{Emit: func(Sample) {}, Progress: func(int, string) {}}); err == nil {
		t.Fatal("wanted an error")
	}
	if names := leftovers(t, dir); len(names) != 0 {
		t.Fatalf("a failed run left %v behind", names)
	}
}

func TestSampleCount(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	// 256 MB is 64 blocks of 4 MiB, which must collapse into at most 32 samples.
	// The literal 32 is written in rather than read from samplesPerPhase.
	if _, err := Run(context.Background(), options{root: dir, sizeMB: 256}, rec.sink()); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{PhaseWrite, PhaseRead} {
		if got := len(phaseOf(rec.all(), phase)); got > 32 {
			t.Errorf("%s emitted %d samples, want at most 32", phase, got)
		}
	}

	short := &recorder{}
	if _, err := Run(context.Background(), options{root: t.TempDir(), sizeMB: 16}, short.sink()); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{PhaseWrite, PhaseRead} {
		if got := len(phaseOf(short.all(), phase)); got < 2 {
			t.Errorf("%s emitted %d samples on a short run, want at least 2 so it is not one point", phase, got)
		}
	}
}

func TestSampleBytesAreMonotonic(t *testing.T) {
	// Asserting only the final total would pass with every intermediate figure
	// wrong, which is the Phase 5 treemap failure. This pins the per-item
	// property instead.
	dir := t.TempDir()
	rec := &recorder{}
	if _, err := Run(context.Background(), options{root: dir, sizeMB: 64}, rec.sink()); err != nil {
		t.Fatal(err)
	}

	for _, phase := range []string{PhaseWrite, PhaseRead} {
		samples := phaseOf(rec.all(), phase)
		if len(samples) == 0 {
			t.Fatalf("%s emitted nothing", phase)
		}
		var previous int64
		var previousSeconds float64
		for i, s := range samples {
			if s.Bytes <= previous {
				t.Errorf("%s sample %d has %d bytes, not more than the %d before it", phase, i, s.Bytes, previous)
			}
			if s.Seconds < previousSeconds {
				t.Errorf("%s sample %d went backwards in time", phase, i)
			}
			if s.MBps <= 0 {
				t.Errorf("%s sample %d has no rate", phase, i)
			}
			previous = s.Bytes
			previousSeconds = s.Seconds
		}
		if want := int64(64) << 20; previous != want {
			t.Errorf("%s finished at %d bytes, want exactly %d", phase, previous, want)
		}
	}
}

func TestSummaryForKeys(t *testing.T) {
	s := summaryFor("/tmp/x", 256,
		phaseResult{bytes: 256 << 20, seconds: 0.5},
		phaseResult{bytes: 256 << 20, seconds: 0.25},
		true)
	for _, key := range []string{
		"path", "sizeMb", "writeMbps", "readMbps", "writeSeconds", "readSeconds",
		"cacheBypassed", "verdict", "note",
	} {
		if _, ok := s[key]; !ok {
			t.Errorf("summary is missing key %q", key)
		}
	}
	if s["readMbps"].(float64) <= s["writeMbps"].(float64) {
		t.Error("the read figure should be the faster one in this fixture")
	}
}

func TestNoSliceIsNil(t *testing.T) {
	data, err := json.Marshal(summaryFor("/tmp/x", 16, phaseResult{}, phaseResult{}, false))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Fatalf("summary marshals a null: %s", data)
	}
}

func TestTempNameStaysInsideTheChosenFolder(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 50; i++ {
		name := tempName(dir)
		if filepath.Dir(name) != dir {
			t.Fatalf("%q is not inside %q", name, dir)
		}
		if !strings.HasPrefix(filepath.Base(name), tempPrefix) {
			t.Fatalf("%q does not carry the prefix that makes it recognisable", name)
		}
	}
	// Two calls must not collide, or a second run could overwrite the first.
	if tempName(dir) == tempName(dir) {
		t.Fatal("two temporary names came out identical")
	}
}

func TestNothingIsWrittenOutsideTheChosenFolder(t *testing.T) {
	src, err := os.ReadFile("diskbench.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"os.TempDir", "os.UserHomeDir", "os.UserCacheDir"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("diskbench.go calls %s; the test file goes in the folder the user chose and nowhere else", banned)
		}
	}
}

func TestStartTestRejectsBeforeWriting(t *testing.T) {
	svc := New(nil)
	dir := t.TempDir()
	if _, err := svc.StartTest(Params{Path: dir, SizeMB: 4097}); err == nil {
		t.Fatal("an oversized run should be refused before anything is written")
	}
	if names := leftovers(t, dir); len(names) != 0 {
		t.Fatalf("a refused run left %v behind", names)
	}
	if _, err := svc.StartTest(Params{Path: filepath.Join(dir, "nope")}); err == nil {
		t.Fatal("a missing folder should be refused")
	}
}

func TestWritableProbeLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	if !writable(dir) {
		t.Fatal("a temp dir should be writable")
	}
	if names := leftovers(t, dir); len(names) != 0 {
		t.Fatalf("the probe left %v behind", names)
	}

	missing := filepath.Join(dir, "not-there")
	if writable(missing) {
		t.Error("a folder that does not exist is not writable")
	}
}

func TestRunIsNotFlaky(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 5 x 16 MB")
	}
	for i := 0; i < 5; i++ {
		dir := t.TempDir()
		rec := &recorder{}
		if _, err := Run(context.Background(), options{root: dir, sizeMB: 16}, rec.sink()); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if names := leftovers(t, dir); len(names) != 0 {
			t.Fatalf("run %d left %v", i, names)
		}
	}
}

// TestRunStopsWritingOnCancel pins the cancel check inside the write loop.
// TestRunRemovesTheFileOnCancel passes with that check deleted, because the read
// phase then notices the cancel and Run still returns context.Canceled with the
// file gone: the whole 4 GB would just be written first. Found by mutation.
func TestRunStopsWritingOnCancel(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel on the first sample, then keep recording so the count is visible.
	sink := Sink{
		Emit: func(s Sample) {
			rec.mu.Lock()
			rec.samples = append(rec.samples, s)
			rec.mu.Unlock()
			cancel()
		},
		Progress: func(int, string) {},
	}

	if _, err := Run(ctx, options{root: dir, sizeMB: 512}, sink); err != context.Canceled {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}

	// 512 MB is 128 blocks over at most 32 samples, so a run that went to
	// completion emits 32 write samples. A run that stopped emits a handful.
	if got := len(phaseOf(rec.all(), PhaseWrite)); got >= 32 {
		t.Errorf("%d write samples: the write ran to completion after the cancel", got)
	}
	if got := len(phaseOf(rec.all(), PhaseRead)); got != 0 {
		t.Errorf("%d read samples after a cancel during the write", got)
	}
	if names := leftovers(t, dir); len(names) != 0 {
		t.Fatalf("a cancelled run left %v behind", names)
	}
}

// TestValidateRejectsAFolderItCannotWriteTo pins the writable probe. Nothing else
// in the suite reaches it, because every other rejection happens earlier in
// walk.Root. Found by mutation.
func TestValidateRejectsAFolderItCannotWriteTo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not stop a write on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores the mode bits this test relies on")
	}

	dir := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	// Put it back so t.TempDir's cleanup can remove it.
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	_, err := (Params{Path: dir}).validate()
	if err == nil {
		t.Fatal("a folder CHIT cannot write into must be refused before the test starts")
	}
	if !strings.Contains(err.Error(), "will not let CHIT write into") ||
		!strings.Contains(err.Error(), "Downloads folder") {
		t.Errorf("message = %q", err.Error())
	}
}

// TestWriteIsFlushedAndTheCacheHintIsRight is a source check, and it is here
// because neither property can be observed from a test on this machine: /tmp is
// tmpfs, where Sync is a no-op and FADV_DONTNEED has nothing to evict, so both
// mutations survived a behavioural suite. Both were instead proven live against
// the real filesystem (the cache hint cut a 512 MB read from 24,756 MB/s to
// 5,831 MB/s); this pins them so they cannot be quietly dropped afterwards.
func TestWriteIsFlushedAndTheCacheHintIsRight(t *testing.T) {
	src, err := os.ReadFile("diskbench.go")
	if err != nil {
		t.Fatal(err)
	}
	// Without the flush the write figure is the speed of a copy into memory, and
	// on Linux the pages stay dirty so FADV_DONTNEED cannot drop them either.
	if !strings.Contains(string(src), "file.Sync()") {
		t.Error("writePass no longer flushes to the device, so the write figure is memory speed")
	}

	linux, err := os.ReadFile("nocache_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(linux), "unix.FADV_DONTNEED") {
		t.Error("the Linux read no longer asks the kernel to drop the file's pages")
	}
	darwin, err := os.ReadFile("nocache_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(darwin), "unix.F_NOCACHE") {
		t.Error("the macOS read no longer turns the cache off")
	}
	windows, err := os.ReadFile("nocache_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(windows), "fileFlagNoBuffering") {
		t.Error("the Windows read no longer opens the handle unbuffered")
	}
}

// TestGroupRate drives the branch a nanosecond clock can never reach. Windows'
// monotonic clock is coarse, commonly around 15 ms, so a block group on a fast
// drive finishes inside one tick and measures as zero elapsed time. CI found
// this by failing: two write samples came back with no rate at all, which the
// UI would draw as the drive stalling to 0 MB/s when it had done the opposite.
func TestGroupRate(t *testing.T) {
	const mib = 1 << 20
	tests := []struct {
		name       string
		groupBytes int64
		elapsed    float64
		doneBytes  int64
		sinceStart float64
		last       bool
		wantMBps   float64
		wantOK     bool
	}{
		{"a measured group reports its own rate", 100 * mib, 0.5, 200 * mib, 1, false, 209.7152, true},
		{"the last group reports its own rate too", 100 * mib, 0.5, 200 * mib, 1, true, 209.7152, true},
		// The case CI hit: too fast for the clock, and not the last group.
		{"an unmeasurable group is held open", 4 * mib, 0, 64 * mib, 0.25, false, 0, false},
		// The last group must still be emitted, because its sample carries the
		// final byte count, so it falls back to the rate for the phase so far.
		{"an unmeasurable last group falls back to the phase rate", 4 * mib, 0, 64 * mib, 0.25, true, 268.435456, true},
		{"a negative elapsed is treated as unmeasurable", 4 * mib, -0.001, 64 * mib, 0.25, false, 0, false},
		{"a group with no bytes is held open", 0, 0.5, 64 * mib, 0.25, false, 0, false},
		// Nothing measurable anywhere. Reporting 0 is honest here: there is no
		// rate to be had, and holding the last group open would lose the sample.
		{"an unmeasurable phase on the last group", 4 * mib, 0, 64 * mib, 0, true, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mbps, ok := groupRate(tt.groupBytes, tt.elapsed, tt.doneBytes, tt.sinceStart, tt.last)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if math.Abs(mbps-tt.wantMBps) > 1e-6 {
				t.Errorf("mbps = %v, want %v", mbps, tt.wantMBps)
			}
		})
	}
}

// A group that is held open must never be reported, and one that is reported
// must never carry a zero rate unless nothing at all could be timed.
func TestGroupRateNeverReportsAZeroRateItCouldAvoid(t *testing.T) {
	for _, elapsed := range []float64{0, -1, 1e-9, 0.001, 1} {
		for _, last := range []bool{false, true} {
			mbps, ok := groupRate(1<<20, elapsed, 64<<20, 0.25, last)
			if ok && mbps <= 0 {
				t.Errorf("elapsed %v last %v: reported %v MB/s, which the chart draws as a stall", elapsed, last, mbps)
			}
			if !ok && last {
				t.Errorf("elapsed %v: the last group was held open, so its byte count is lost", elapsed)
			}
		}
	}
}
