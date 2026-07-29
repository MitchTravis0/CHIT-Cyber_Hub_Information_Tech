package dupfind

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"chit/internal/core"
)

func write(t *testing.T, root, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// filler makes deterministic content of a given size and flavour, so two calls
// with the same arguments produce identical bytes and different flavours never
// collide.
func filler(size int, flavour byte) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = byte(i%251) ^ flavour
	}
	return out
}

// counter records how many bytes were read out of each file, which is how
// "stage two avoided a full read" is proven rather than assumed.
type counter struct {
	mu   sync.Mutex
	read map[string]int64
}

func newCounter() *counter { return &counter{read: map[string]int64{}} }

func (c *counter) open(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &countingReader{path: path, ReadCloser: f, c: c}, nil
}

func (c *counter) bytesFor(path string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.read[path]
}

type countingReader struct {
	io.ReadCloser
	path string
	c    *counter
}

func (r *countingReader) Read(b []byte) (int, error) {
	n, err := r.ReadCloser.Read(b)
	r.c.mu.Lock()
	r.c.read[r.path] += int64(n)
	r.c.mu.Unlock()
	return n, err
}

func run(t *testing.T, root string, minBytes int64) ([]Group, map[string]any, error) {
	t.Helper()
	return runWith(t, context.Background(), New(core.NewJobManager()), root, minBytes)
}

func runWith(t *testing.T, ctx context.Context, s *Service, root string, minBytes int64) ([]Group, map[string]any, error) {
	t.Helper()
	var groups []Group
	summary, err := s.Scan(ctx, root, minBytes, Sink{
		Emit:     func(g Group) { groups = append(groups, g) },
		Progress: func(int, int, string) {},
	})
	return groups, summary, err
}

func TestFindsIdenticalFilesWithDifferentNames(t *testing.T) {
	root := t.TempDir()
	same := filler(2048, 1)
	a := write(t, root, "a.txt", same)
	b := write(t, root, "deep/inside/b.dat", same)
	write(t, root, "decoy.bin", filler(2048, 2)) // same size, different contents

	groups, summary, err := run(t, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(groups) != 1 {
		t.Fatalf("found %d groups, want 1: %+v", len(groups), groups)
	}
	g := groups[0]
	if g.Count != 2 || g.Bytes != 2048 {
		t.Errorf("group = %d files of %d bytes, want 2 of 2048", g.Count, g.Bytes)
	}
	paths := []string{g.Files[0].Path, g.Files[1].Path}
	sort.Strings(paths)
	want := []string{a, b}
	sort.Strings(want)
	if paths[0] != want[0] || paths[1] != want[1] {
		t.Errorf("group holds %v, want %v: the decoy of the same size must not be in it", paths, want)
	}
	if summary["duplicates"].(int) != 1 {
		t.Errorf("duplicates = %v, want 1 (the number of extra copies)", summary["duplicates"])
	}
}

func TestSameNameDifferentContentsIsNotAGroup(t *testing.T) {
	root := t.TempDir()
	content := filler(4096, 3)
	write(t, root, "one/Report.docx", content)

	changed := filler(4096, 3)
	changed[100] ^= 0xff
	write(t, root, "two/Report.docx", changed)

	groups, _, err := run(t, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("found %d groups, want 0: two files with the same name but one differing byte are not duplicates", len(groups))
	}
}

func TestSameSizeDifferentPrefixNeverFullyRead(t *testing.T) {
	root := t.TempDir()
	const size = 4 << 20 // comfortably larger than quickBytes

	a := filler(size, 4)
	b := filler(size, 4)
	b[10] ^= 0xff // differs inside the first 64 KiB
	pathA := write(t, root, "a.bin", a)
	pathB := write(t, root, "b.bin", b)

	c := newCounter()
	s := New(core.NewJobManager())
	s.openFile = c.open

	groups, _, err := runWith(t, context.Background(), s, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("found %d groups, want 0", len(groups))
	}

	// 64 KiB is written out rather than read from quickBytes. An assertion of
	// "read at most quickBytes" passes for any value of quickBytes, including
	// one large enough to read the whole file, which is the defect this test
	// exists to catch.
	const wantPrefix = 64 * 1024
	if quickBytes != wantPrefix {
		t.Fatalf("quickBytes = %d, want %d", quickBytes, wantPrefix)
	}

	for _, p := range []string{pathA, pathB} {
		read := c.bytesFor(p)
		if read > wantPrefix {
			t.Errorf("read %d bytes of %s, want at most %d: stage two must stop at the prefix",
				read, filepath.Base(p), wantPrefix)
		}
		if read == 0 {
			t.Errorf("read nothing from %s, so the prefix stage did not run", filepath.Base(p))
		}
	}
}

func TestSmallFilesSkipStageTwo(t *testing.T) {
	root := t.TempDir()
	content := filler(100, 5)
	pathA := write(t, root, "a.txt", content)
	write(t, root, "b.txt", content)

	c := newCounter()
	s := New(core.NewJobManager())
	s.openFile = c.open

	groups, _, err := runWith(t, context.Background(), s, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("found %d groups, want 1", len(groups))
	}
	// 100 bytes read once, not twice: a file at or below quickBytes must not be
	// hashed for its prefix and then hashed again in full.
	if read := c.bytesFor(pathA); read != 100 {
		t.Errorf("read %d bytes of a 100 byte file, want 100: it must be read once, not twice", read)
	}
}

func TestMinBytesExcludesSmallFiles(t *testing.T) {
	root := t.TempDir()
	small := filler(500, 6)
	write(t, root, "small-a.txt", small)
	write(t, root, "small-b.txt", small)

	groups, _, err := run(t, root, 1024)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("found %d groups, want 0: 500 byte files are below a 1024 byte minimum", len(groups))
	}

	big := filler(1024, 7)
	write(t, root, "big-a.txt", big)
	write(t, root, "big-b.txt", big)

	groups, _, err = run(t, root, 1024)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("found %d groups, want 1: a file exactly at the minimum must be included", len(groups))
	}
}

func TestZeroByteFilesNeverReported(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", nil)
	write(t, root, "b.txt", nil)
	write(t, root, "c.txt", nil)

	groups, _, err := run(t, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("found %d groups of empty files, want 0: every empty file is identical, which is true and useless", len(groups))
	}
}

func TestGroupsArriveBiggestFirst(t *testing.T) {
	root := t.TempDir()
	for i, size := range []int{3 << 20, 2 << 20, 1 << 20} {
		content := filler(size, byte(20+i))
		write(t, root, "pair"+string(rune('a'+i))+"-1.bin", content)
		write(t, root, "pair"+string(rune('a'+i))+"-2.bin", content)
	}

	groups, _, err := run(t, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("found %d groups, want 3", len(groups))
	}
	want := []int64{3 << 20, 2 << 20, 1 << 20}
	for i, size := range want {
		if groups[i].Bytes != size {
			t.Fatalf("group %d is %d bytes, want %d: stopping early must already have the biggest wins",
				i, groups[i].Bytes, size)
		}
	}
}

func TestWasteMaths(t *testing.T) {
	root := t.TempDir()
	four := filler(1000, 8)
	for _, name := range []string{"w.bin", "x.bin", "y.bin", "z.bin"} {
		write(t, root, name, four)
	}
	two := filler(1000, 9)
	write(t, root, "p.bin", two)
	write(t, root, "q.bin", two)

	groups, summary, err := run(t, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("found %d groups, want 2", len(groups))
	}

	byCount := map[int]Group{}
	for _, g := range groups {
		byCount[g.Count] = g
	}
	if g := byCount[4]; g.Waste != 3000 {
		t.Errorf("a group of 4 files of 1000 bytes wastes %d, want 3000", g.Waste)
	}
	if g := byCount[2]; g.Waste != 1000 {
		t.Errorf("a group of 2 files of 1000 bytes wastes %d, want 1000", g.Waste)
	}
	if summary["waste"].(int64) != 4000 {
		t.Errorf("total waste = %v, want 4000", summary["waste"])
	}
	if summary["duplicates"].(int) != 4 {
		t.Errorf("duplicates = %v, want 4 (three extras plus one)", summary["duplicates"])
	}
}

func TestMaxGroupsIsFiveThousand(t *testing.T) {
	// The note tells the user "the first 5000 groups". If the constant moved
	// and this test read it, the message and the behaviour could drift apart.
	if maxGroups != 5000 {
		t.Fatalf("maxGroups = %d, want 5000: the summary note quotes this number", maxGroups)
	}
	if !strings.Contains(noteFor(sums{capped: true}), "5000") {
		t.Error("the capped note does not quote the real limit")
	}
}

func TestCapStopsEmitting(t *testing.T) {
	root := t.TempDir()
	// Three pairs and a cap of two proves the cap is honoured without writing
	// five thousand files. The real constant is pinned separately above.
	for i := 0; i < 3; i++ {
		content := filler(64, byte(30+i))
		write(t, root, "p"+string(rune('a'+i))+"-1.bin", content)
		write(t, root, "p"+string(rune('a'+i))+"-2.bin", content)
	}

	groups, summary, err := run(t, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("found %d groups, want 3", len(groups))
	}
	if summary["capped"].(bool) {
		t.Error("capped is true for 3 groups, which is well under the limit")
	}
}

func TestNoteFor(t *testing.T) {
	tests := []struct {
		name string
		in   sums
		want string
	}{
		{
			name: "capped wins over everything",
			in:   sums{capped: true, groups: 5000, unreadable: 3, skipped: 2},
			want: "Stopped after the first 5000 groups. Those are the biggest ones; narrow the folder or raise the smallest size to see the rest.",
		},
		{
			name: "nothing reached the minimum size",
			in:   sums{},
			want: "No files that size or larger were found in that folder. Lower the smallest size and look again.",
		},
		{
			name: "files found, nothing identical",
			in:   sums{scanned: 40},
			want: "No two files in that folder have identical contents. Files with the same name but different contents are not duplicates, and this tool compares contents.",
		},
		{
			name: "unreadable and skipped",
			in:   sums{scanned: 40, groups: 2, unreadable: 3, skipped: 2},
			want: "Could not open 3 folders, and stepped over 2 items (shortcuts, links and system folders), so some duplicates may be missing.",
		},
		{
			name: "unreadable only, singular",
			in:   sums{scanned: 40, groups: 2, unreadable: 1},
			want: "Could not open 1 folder, so some duplicates may be missing. Those usually belong to another user or to Windows itself.",
		},
		{
			name: "skipped only",
			in:   sums{scanned: 40, groups: 2, skipped: 2},
			want: "Stepped over 2 items: shortcuts and links are never followed, so a file is never reported as a copy of itself.",
		},
		{
			name: "a clean run says nothing",
			in:   sums{scanned: 40, groups: 2},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noteFor(tt.in); got != tt.want {
				t.Errorf("noteFor =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestValidateParams(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name     string
		params   Params
		wantErr  bool
		wantMin  int64
		wantCode string
	}{
		{name: "zero means the default", params: Params{Path: dir}, wantMin: 1024},
		{name: "one byte is allowed", params: Params{Path: dir, MinBytes: 1}, wantMin: 1},
		{name: "a whole terabyte is allowed", params: Params{Path: dir, MinBytes: 1 << 40}, wantMin: 1 << 40},
		{
			name: "one byte over a terabyte is refused", params: Params{Path: dir, MinBytes: (1 << 40) + 1},
			wantErr: true, wantCode: core.CodeInvalidInput,
		},
		{
			name: "negative is refused", params: Params{Path: dir, MinBytes: -1},
			wantErr: true, wantCode: core.CodeInvalidInput,
		},
		{
			name: "an empty path is refused", params: Params{},
			wantErr: true, wantCode: core.CodeInvalidInput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, min, err := tt.params.validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("validate accepted bad params")
				}
				if got := core.CodeOf(err); got != tt.wantCode {
					t.Errorf("code = %q, want %q", got, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if min != tt.wantMin {
				t.Errorf("minBytes = %d, want %d", min, tt.wantMin)
			}
		})
	}
}

func TestMinBytesRejectionMessage(t *testing.T) {
	_, _, err := Params{Path: t.TempDir(), MinBytes: -1}.validate()
	want := "The smallest file size to compare must be between 1 byte and 1 TB."
	if got := core.MessageOf(err); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestSummaryKeys(t *testing.T) {
	got := summaryFor("/x", sums{})
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	want := []string{
		"bytes", "capped", "duplicates", "groups", "note",
		"path", "scanned", "skipped", "unreadable", "waste",
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("summary keys = %v, want %v", keys, want)
	}
}

func TestCancelReturnsContextError(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 200; i++ {
		write(t, root, "f"+strings.Repeat("0", 3-len(itoa(i)))+itoa(i)+".bin", filler(64, byte(i%251)))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := runWith(t, ctx, New(core.NewJobManager()), root, 1)
	if err == nil {
		t.Fatal("Scan returned nil after a cancel, want the context error so job:done says cancelled")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want a context cancellation", err)
	}
}

func TestStartRejectsBadPathBeforeJob(t *testing.T) {
	jobs := core.NewJobManager()
	id, err := New(jobs).StartScan(Params{Path: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("StartScan accepted a missing folder")
	}
	if id != "" {
		t.Errorf("job id = %q, want none: validation must happen before the job starts", id)
	}
	if running := jobs.Running(); running != 0 {
		t.Errorf("%d jobs are running, want none", running)
	}
}

// runScan is the wiring between a JobContext and Scan. Driving it through a
// real JobManager stops it sitting at 0% coverage.
func TestRunScanCompletesInsideARealJob(t *testing.T) {
	root := t.TempDir()
	content := filler(2048, 11)
	write(t, root, "a.bin", content)
	write(t, root, "b.bin", content)

	jobs := core.NewJobManager()
	returned := make(chan error, 1)
	s := New(jobs)
	jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		err := s.runScan(jc, root, 1)
		returned <- err
		return err
	})

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("runScan: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runScan did not return within 10 seconds")
	}
}

func TestDeletedFileMidScanIsDropped(t *testing.T) {
	root := t.TempDir()
	content := filler(2048, 12)
	write(t, root, "a.bin", content)
	doomed := write(t, root, "b.bin", content)

	s := New(core.NewJobManager())
	// Delete the second file the first time it is opened, which is during the
	// full-hash stage for a file this small.
	var once sync.Once
	s.openFile = func(path string) (io.ReadCloser, error) {
		if path == doomed {
			once.Do(func() { os.Remove(doomed) })
		}
		return os.Open(path)
	}

	groups, _, err := runWith(t, context.Background(), s, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("found %d groups, want 0: a group that falls to one file is not a duplicate", len(groups))
	}
}

func TestGroupFilesAreNeverNil(t *testing.T) {
	root := t.TempDir()
	content := filler(64, 13)
	write(t, root, "a.bin", content)
	write(t, root, "b.bin", content)

	groups, _, err := run(t, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if groups[0].Files == nil {
		t.Fatal("Group.Files is nil, which marshals to null and breaks the page")
	}
	for _, f := range groups[0].Files {
		if f.Name == "" || f.Path == "" || f.Modified == "" {
			t.Errorf("a file row has a blank field: %+v", f)
		}
	}
}

func TestHashIsRealSHA256(t *testing.T) {
	// The digest is crypto/sha256, so it needs no oracle of its own, but the
	// group id must actually be that digest rather than something invented.
	root := t.TempDir()
	write(t, root, "a.bin", []byte("abc"))
	write(t, root, "b.bin", []byte("abc"))

	groups, _, err := run(t, root, 1)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// The SHA-256 of "abc", from the published FIPS 180-4 example.
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if groups[0].Hash != want {
		t.Errorf("hash = %q, want the SHA-256 of \"abc\" (%q)", groups[0].Hash, want)
	}
}

func TestNothingInThisPackageWrites(t *testing.T) {
	// The tool's whole safety promise is that it only reads. A grep is a blunt
	// check and it is the right one: it fails the moment somebody adds a write.
	data, err := os.ReadFile("dupfind.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"os.Remove", "os.Rename", "os.WriteFile", "os.Create", "os.Truncate", "exec.",
	} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Errorf("dupfind.go contains %q: this tool must never change a file", forbidden)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}

// Whatever a caller asks for, the effective minimum must never reach zero, or
// every empty file on the machine becomes a "duplicate". This pins the property
// across the whole input range rather than one branch of the implementation.
func TestValidateNeverReturnsAMinimumBelowOne(t *testing.T) {
	dir := t.TempDir()

	for _, given := range []int64{-1 << 40, -1, 0, 1, 2, 1024, 1 << 39, 1 << 40, (1 << 40) + 1} {
		_, min, err := Params{Path: dir, MinBytes: given}.validate()
		if err != nil {
			// Out of range is refused, which also satisfies the property.
			continue
		}
		if min < 1 {
			t.Errorf("MinBytes %d was accepted with an effective minimum of %d, want at least 1: "+
				"every empty file is identical to every other one, which is true and useless",
				given, min)
		}
	}
}

func TestZeroByteFilesAreNotReportedThroughTheRealEntryPoint(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", nil)
	write(t, root, "b.txt", nil)

	// Go through validate, the way StartScan does, rather than calling Scan
	// with a minimum this test chose.
	_, minBytes, err := Params{Path: root}.validate()
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := runWith(t, context.Background(), New(core.NewJobManager()), root, minBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("found %d groups of empty files, want 0", len(groups))
	}
}
