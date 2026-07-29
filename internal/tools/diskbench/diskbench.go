// Package diskbench measures how fast a drive or a network share reads and
// writes, by writing one temporary file inside a folder the user chose and
// reading it back. It is the "this machine is slow" triage tool: a dying drive,
// a nearly full SSD or a USB 2.0 stick somebody is working off by mistake all
// show up in seconds.
//
// The temporary file is removed on every exit path, including cancel and error.
// Nothing else on the drive is touched, and nothing is read except the file this
// package just wrote.
package diskbench

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"chit/internal/core"
	"chit/internal/walk"
)

// JobKind is the job manager kind for a disk speed test.
const JobKind = "diskbench"

// KindSample is the result kind of every block group measured.
const KindSample = "sample"

// Phases.
const (
	PhaseWrite = "write"
	PhaseRead  = "read"
)

const (
	defaultSizeMB = 256
	minSizeMB     = 16
	maxSizeMB     = 4096

	// blockBytes is 4 MiB: a multiple of every sector size in use, so the
	// Windows unbuffered read can take it directly, and large enough that the
	// per-write syscall cost is noise.
	blockBytes = 4 << 20

	// samplesPerPhase caps the rows one phase emits. A 4096 MB run is 1024
	// blocks; one row each would bury the shape of the run in noise.
	samplesPerPhase = 32

	tempPrefix = "chit-diskspeed-"
)

// Params is what the UI sends. Every zero value means "use the default".
type Params struct {
	Path string `json:"path"`
	// SizeMB is the size of the temporary file. Zero means the default.
	SizeMB int `json:"sizeMb"`
}

// Sample is one measured block group.
type Sample struct {
	// Phase is "write" or "read".
	Phase string `json:"phase"`
	// Bytes is the running total for this phase.
	Bytes int64 `json:"bytes"`
	// Seconds is the elapsed time for this phase so far.
	Seconds float64 `json:"seconds"`
	// MBps is the rate for this block group alone, so a drive that slows down
	// part way through shows it.
	MBps float64 `json:"mbps"`
}

// Sink is where a run reports to. The job wires it to jc.Emit and jc.Progress; a
// test wires it to a slice, which is the only way to read what a run emitted
// without a Wails runtime.
type Sink struct {
	Emit     func(Sample)
	Progress func(doneMB int, message string)
}

// options is the validated form of Params, produced before anything is written.
type options struct {
	root   string
	sizeMB int
}

func (o options) totalBytes() int64 { return int64(o.sizeMB) << 20 }

// validate catches everything a user can get wrong, so bad input rejects the
// StartTest call instead of starting a job that immediately fails. The folder
// checks reuse walk.Root, which already has the wording for a missing folder, a
// file where a folder was expected, and a folder this user cannot open.
func (p Params) validate() (options, error) {
	root, err := walk.Root(p.Path)
	if err != nil {
		return options{}, err
	}
	info, err := os.Stat(root)
	if err == nil && !info.IsDir() {
		return options{}, core.Errorf(core.CodeInvalidInput,
			"%s is a file, not a folder. Pick the folder it is in.", root)
	}
	if !writable(root) {
		return options{}, core.Errorf(core.CodePermission,
			"This computer will not let CHIT write into %s. Pick a folder you own, such as your Downloads folder.", root)
	}

	size := p.SizeMB
	if size == 0 {
		size = defaultSizeMB
	}
	if size < minSizeMB || size > maxSizeMB {
		return options{}, core.Errorf(core.CodeInvalidInput,
			"The test size must be between %d MB and %d MB. %d MB is enough to tell a healthy drive from a failing one.",
			minSizeMB, maxSizeMB, defaultSizeMB)
	}

	return options{root: root, sizeMB: size}, nil
}

// writable proves the folder can be written to now, rather than finding out when
// the test is half done.
func writable(dir string) bool {
	probe, err := os.CreateTemp(dir, tempPrefix+"probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return true
}

// verdictFor is the plain reading of the result. It is keyed on the slower of
// the two figures, because a drive is only as good as its slower direction.
func verdictFor(readMBps, writeMBps float64) string {
	slower := readMBps
	if writeMBps < slower {
		slower = writeMBps
	}
	switch {
	case slower >= 400:
		return "This looks like an SSD or an NVMe drive."
	case slower >= 150:
		return "Fast enough for everyday work. An older SSD, or a fast network share."
	case slower >= 60:
		return "About what a healthy spinning hard disk does."
	case slower >= 20:
		return "Slow. A spinning disk that is busy or nearly full, a USB 2.0 stick, or a network share."
	case slower > 0:
		return "Very slow. Check the drive's health before blaming the machine."
	}
	return "Too little was transferred to measure."
}

// sessionNote labels the figures. When the operating system would not let CHIT
// bypass its cache, the read figure may be memory speed, and saying so is the
// difference between a useful number and a misleading one.
func sessionNote(cacheBypassed bool) string {
	if cacheBypassed {
		return "These figures are approximate. Anything else the machine is doing at the time affects them, so run it twice before drawing a conclusion."
	}
	return "These figures are approximate, and the read figure may be coming from memory rather than from the drive: this operating system would not let CHIT bypass its cache without administrator rights. Trust the write figure more than the read figure here."
}

// summaryFor is the job:done summary. Its keys are a contract with the page and
// are checked nowhere by the compiler, in either language, so they are built in
// one named place that a test can pin.
func summaryFor(root string, sizeMB int, write, read phaseResult, cacheBypassed bool) map[string]any {
	return map[string]any{
		"path":          root,
		"sizeMb":        sizeMB,
		"writeMbps":     write.mbps(),
		"readMbps":      read.mbps(),
		"writeSeconds":  write.seconds,
		"readSeconds":   read.seconds,
		"cacheBypassed": cacheBypassed,
		"verdict":       verdictFor(read.mbps(), write.mbps()),
		"note":          sessionNote(cacheBypassed),
	}
}

// phaseResult is what one pass measured.
type phaseResult struct {
	bytes   int64
	seconds float64
}

// mbps is megabytes per second, decimal, because that is the unit every drive
// datasheet and every other benchmark uses. The network tools report megabits;
// mixing the two is the mistake to avoid.
func (p phaseResult) mbps() float64 {
	if p.seconds <= 0 || p.bytes <= 0 {
		return 0
	}
	return float64(p.bytes) / 1e6 / p.seconds
}

// tempName is a name nothing else will pick, inside the folder the user chose
// and nowhere else.
func tempName(root string) string {
	return filepath.Join(root, tempPrefix+strconv.FormatUint(rand.Uint64(), 36)+".tmp")
}

// pattern is the block written over and over. It is a repeating byte sequence
// rather than zeroes because a filesystem that compresses or deduplicates (ZFS,
// btrfs, a network share) would otherwise store almost nothing and the write
// figure would be fiction.
var pattern = func() []byte {
	b := make([]byte, blockBytes)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}()

// groupSize is how many blocks share one emitted sample, so a long run does not
// emit a thousand rows.
func groupSize(totalBlocks int) int {
	if totalBlocks <= samplesPerPhase {
		return 1
	}
	size := totalBlocks / samplesPerPhase
	if totalBlocks%samplesPerPhase != 0 {
		size++
	}
	return size
}

// diskFull turns the operating system's out-of-space error into a sentence that
// names the folder and says what to do.
func diskFull(err error, root string) error {
	if errors.Is(err, syscall.ENOSPC) {
		return core.Errorf(core.CodeInternal,
			"The drive ran out of space while writing the test file into %s. Free some space, or choose a smaller test size.", root)
	}
	return core.Errorf(core.CodeInternal,
		"The test file could not be written into %s. Check that the drive is still connected and that you can write to that folder.", root)
}

// Run writes one temporary file, reads it back, and returns the job:done
// summary. The file is removed on every path out of this function, including a
// cancel part way through the write.
func Run(ctx context.Context, opts options, out Sink) (map[string]any, error) {
	path := tempName(opts.root)
	defer os.Remove(path)

	total := opts.totalBytes()
	sizeMB := opts.sizeMB

	write, err := writePass(ctx, path, total, sizeMB, out)
	if err != nil {
		return nil, err
	}

	read, bypassed, err := readPass(ctx, path, total, sizeMB, out)
	if err != nil {
		return nil, err
	}

	return summaryFor(opts.root, sizeMB, write, read, bypassed), nil
}

// writePass times the write including the flush to the device. Without the
// Sync it would be timing a copy into memory, and on a slow drive the number
// would be off by an order of magnitude.
func writePass(ctx context.Context, path string, total int64, sizeMB int, out Sink) (phaseResult, error) {
	file, err := os.Create(path)
	if err != nil {
		return phaseResult{}, diskFull(err, filepath.Dir(path))
	}
	defer file.Close()

	blocks := int((total + blockBytes - 1) / blockBytes)
	group := groupSize(blocks)

	started := time.Now()
	groupStart := started
	var written int64
	var groupBytes int64

	out.Progress(0, "Writing "+strconv.Itoa(sizeMB)+" MB to "+filepath.Dir(path))

	for block := 0; written < total; block++ {
		if ctx.Err() != nil {
			return phaseResult{}, ctx.Err()
		}
		chunk := pattern
		if remaining := total - written; remaining < int64(len(chunk)) {
			chunk = chunk[:remaining]
		}
		n, err := file.Write(chunk)
		written += int64(n)
		groupBytes += int64(n)
		if err != nil {
			return phaseResult{}, diskFull(err, filepath.Dir(path))
		}

		if (block+1)%group == 0 || written >= total {
			now := time.Now()
			out.Emit(Sample{
				Phase:   PhaseWrite,
				Bytes:   written,
				Seconds: now.Sub(started).Seconds(),
				MBps:    rate(groupBytes, now.Sub(groupStart).Seconds()),
			})
			out.Progress(int(written>>20), "Writing "+strconv.Itoa(sizeMB)+" MB to "+filepath.Dir(path))
			groupStart = now
			groupBytes = 0
		}
	}

	if err := file.Sync(); err != nil {
		return phaseResult{}, diskFull(err, filepath.Dir(path))
	}
	return phaseResult{bytes: written, seconds: time.Since(started).Seconds()}, nil
}

// readPass reads the file back with the operating system's cache bypassed where
// it allows it. The returned bool is false where it would not, and the page then
// says the read figure may be coming from memory.
func readPass(ctx context.Context, path string, total int64, sizeMB int, out Sink) (phaseResult, bool, error) {
	file, bypassed, err := openUncached(path, total)
	if err != nil {
		return phaseResult{}, false, core.Errorf(core.CodeInternal,
			"The test file was removed while the test was running. Something else on this machine may be cleaning that folder.")
	}
	defer file.Close()

	buf := readBuffer(blockBytes)
	blocks := int((total + blockBytes - 1) / blockBytes)
	group := groupSize(blocks)

	started := time.Now()
	groupStart := started
	var read int64
	var groupBytes int64

	out.Progress(sizeMB, "Reading it back")

	for block := 0; read < total; block++ {
		if ctx.Err() != nil {
			return phaseResult{}, bypassed, ctx.Err()
		}
		want := buf
		if remaining := total - read; remaining < int64(len(want)) {
			want = want[:remaining]
		}
		n, err := io.ReadFull(file, want)
		read += int64(n)
		groupBytes += int64(n)
		if err != nil {
			if read < total {
				return phaseResult{}, bypassed, core.Errorf(core.CodeInternal,
					"The test file could not be read back. Check that the drive is still connected.")
			}
			break
		}

		if (block+1)%group == 0 || read >= total {
			now := time.Now()
			out.Emit(Sample{
				Phase:   PhaseRead,
				Bytes:   read,
				Seconds: now.Sub(started).Seconds(),
				MBps:    rate(groupBytes, now.Sub(groupStart).Seconds()),
			})
			out.Progress(sizeMB+int(read>>20), "Reading it back")
			groupStart = now
			groupBytes = 0
		}
	}

	return phaseResult{bytes: read, seconds: time.Since(started).Seconds()}, bypassed, nil
}

// rate is megabytes per second for one block group.
func rate(bytes int64, seconds float64) float64 {
	if seconds <= 0 || bytes <= 0 {
		return 0
	}
	return float64(bytes) / 1e6 / seconds
}

// Service owns the test entry point. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartTest begins a test and returns the job id at once. Samples arrive as
// "sample" items on job:result, one per block group in each phase.
func (s *Service) StartTest(p Params) (string, error) {
	opts, err := p.validate()
	if err != nil {
		return "", err
	}
	// The bar covers both passes, so it does not jump back to zero half way.
	return s.jobs.Start(JobKind, opts.sizeMB*2, func(jc *core.JobContext) error {
		return runBench(jc, opts)
	}), nil
}

// runBench is the body of the job, named so it is not stranded at 0% coverage
// inside an anonymous closure.
func runBench(jc *core.JobContext, opts options) error {
	summary, err := Run(jc.Ctx(), opts, Sink{
		Emit:     func(s Sample) { jc.Emit(KindSample, s) },
		Progress: func(doneMB int, message string) { jc.Progress(doneMB, opts.sizeMB*2, message) },
	})
	if err != nil {
		return err
	}
	jc.SetSummary(summary)
	return nil
}
