package diskscan

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
	"chit/internal/walk"
)

func tree(t *testing.T, files map[string]int) string {
	t.Helper()
	root := t.TempDir()
	for name, size := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// runWithContext drives Scan against a recording Sink. A JobContext's emitted
// items go out through the Wails runtime and cannot be read back, which is
// exactly why Scan takes a Sink instead of a JobContext.
func runWithContext(ctx context.Context, root string) ([]Entry, map[string]any, error) {
	entries, _, summary, err := runRecording(ctx, root)
	return entries, summary, err
}

func runRecording(ctx context.Context, root string) ([]Entry, []string, map[string]any, error) {
	var entries []Entry
	var progress []string
	summary, err := Scan(ctx, root, Sink{
		Emit:     func(e Entry) { entries = append(entries, e) },
		Progress: func(_ int, message string) { progress = append(progress, message) },
	})
	return entries, progress, summary, err
}

func TestScanEmitsOneEntryPerChild(t *testing.T) {
	root := tree(t, map[string]int{
		"loose.txt":       10,
		"alpha/a.txt":     20,
		"alpha/sub/b.txt": 30,
		"beta/c.txt":      40,
	})

	entries, summary, err := runWithContext(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("emitted %d entries, want 3 (one loose file and two folders): %+v", len(entries), entries)
	}

	byName := map[string]Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	if got := byName["loose.txt"]; got.Dir || got.Bytes != 10 || got.Files != 1 {
		t.Errorf("loose.txt = %+v, want a file of 10 bytes counting 1", got)
	}
	// 20 + 30, written out rather than summed from the fixture.
	if got := byName["alpha"]; !got.Dir || got.Bytes != 50 || got.Files != 2 {
		t.Errorf("alpha = %+v, want a folder of 50 bytes counting 2 files", got)
	}
	if got := byName["beta"]; !got.Dir || got.Bytes != 40 || got.Files != 1 {
		t.Errorf("beta = %+v, want a folder of 40 bytes counting 1 file", got)
	}

	if summary["bytes"].(int64) != 100 {
		t.Errorf("summary bytes = %v, want 100", summary["bytes"])
	}
	if summary["files"].(int) != 4 {
		t.Errorf("summary files = %v, want 4", summary["files"])
	}
}

func TestScanEmptyFolder(t *testing.T) {
	entries, summary, err := runWithContext(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("emitted %d entries for an empty folder, want 0", len(entries))
	}
	if summary["note"] != "That folder is empty." {
		t.Errorf("note = %q, want the empty-folder sentence", summary["note"])
	}
}

func TestLargestKeepsTopN(t *testing.T) {
	// 25 is the number the help text and the page heading both rely on, so it
	// is written here as a literal rather than read from the constant.
	const want = 25
	if maxLargest != want {
		t.Fatalf("maxLargest = %d, want %d: the page says it shows the biggest %d files", maxLargest, want, want)
	}

	l := &largestSet{}
	for i := 1; i <= 30; i++ {
		l.add(walk.File{Path: filepath.Join("d", "f"+pad(i)), Size: int64(i * 100)})
	}

	if len(l.items) != want {
		t.Fatalf("kept %d files, want %d", len(l.items), want)
	}
	if l.items[0].Bytes != 3000 {
		t.Errorf("biggest = %d bytes, want 3000", l.items[0].Bytes)
	}
	if l.items[want-1].Bytes != 600 {
		t.Errorf("smallest kept = %d bytes, want 600: the five smallest must be dropped", l.items[want-1].Bytes)
	}
	for i := 1; i < len(l.items); i++ {
		if l.items[i-1].Bytes < l.items[i].Bytes {
			t.Fatalf("list is not biggest first at %d: %d then %d", i, l.items[i-1].Bytes, l.items[i].Bytes)
		}
	}
}

func TestLargestTieOrder(t *testing.T) {
	l := &largestSet{}
	l.add(walk.File{Path: "first", Size: 100})
	l.add(walk.File{Path: "second", Size: 100})
	l.add(walk.File{Path: "third", Size: 100})

	got := []string{l.items[0].Path, l.items[1].Path, l.items[2].Path}
	want := []string{"first", "second", "third"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v: equal sizes must keep insertion order so a rerun matches", got, want)
		}
	}
}

func TestLargestNamesTheFile(t *testing.T) {
	l := &largestSet{}
	l.add(walk.File{Path: filepath.Join("a", "b", "big.iso"), Size: 1})
	if l.items[0].Name != "big.iso" {
		t.Errorf("Name = %q, want big.iso", l.items[0].Name)
	}
}

func TestNoteFor(t *testing.T) {
	tests := []struct {
		name string
		res  walk.Result
		want string
	}{
		{
			name: "both, plural",
			res:  walk.Result{Files: 5, Unreadable: 3, Skipped: 2},
			want: "Could not open 3 folders, and stepped over 2 items (shortcuts, links and system folders), so the total is lower than the real figure.",
		},
		{
			name: "both, singular",
			res:  walk.Result{Files: 5, Unreadable: 1, Skipped: 1},
			want: "Could not open 1 folder, and stepped over 1 item (shortcuts, links and system folders), so the total is lower than the real figure.",
		},
		{
			name: "unreadable only",
			res:  walk.Result{Files: 5, Unreadable: 3},
			want: "Could not open 3 folders, so the total is lower than the real figure. Those usually belong to another user or to Windows itself.",
		},
		{
			name: "skipped only",
			res:  walk.Result{Files: 5, Skipped: 2},
			want: "Stepped over 2 items: shortcuts, links and system folders are never followed, so nothing is counted twice.",
		},
		{
			name: "empty folder",
			res:  walk.Result{},
			want: "That folder is empty.",
		},
		{
			name: "nothing to say",
			res:  walk.Result{Files: 5, Bytes: 100},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noteFor(tt.res); got != tt.want {
				t.Errorf("noteFor =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{{0, "0 folders"}, {1, "1 folder"}, {2, "2 folders"}, {100, "100 folders"}}
	for _, tt := range tests {
		if got := count(tt.n, "folder", "folders"); got != tt.want {
			t.Errorf("count(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestSummaryKeys(t *testing.T) {
	// These keys are a contract with the page and nothing in either language
	// checks them, so they are pinned here.
	got := summaryFor("/x", walk.Result{}, &largestSet{})

	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	want := []string{"bytes", "dirs", "files", "largest", "note", "path", "skipped", "unreadable"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("summary keys = %v, want %v", keys, want)
	}
}

func TestSummaryLargestIsNeverNil(t *testing.T) {
	got := summaryFor("/x", walk.Result{}, &largestSet{})
	list, ok := got["largest"].([]map[string]any)
	if !ok {
		t.Fatalf("largest is %T, want []map[string]any", got["largest"])
	}
	if list == nil {
		t.Error("largest is nil, which marshals to null and breaks the page")
	}
}

func TestScanCancelReturnsContextError(t *testing.T) {
	files := map[string]int{}
	for i := 0; i < 400; i++ {
		files[filepath.ToSlash(filepath.Join("d", "f"+pad(i)+".txt"))] = 1
	}
	root := tree(t, files)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := runWithContext(ctx, root)
	if err == nil {
		t.Fatal("scan returned nil after a cancel, want the context error so job:done says cancelled")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want a context cancellation", err)
	}
}

func TestStartRejectsBadPathBeforeJob(t *testing.T) {
	s := New(core.NewJobManager())

	id, err := s.StartScan(Params{Path: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("StartScan accepted a missing folder")
	}
	if id != "" {
		t.Errorf("StartScan returned job id %q, want none: validation must happen before the job starts", id)
	}
	if core.CodeOf(err) != core.CodeNotFound {
		t.Errorf("code = %q, want %q", core.CodeOf(err), core.CodeNotFound)
	}
}

func TestStartAcceptsAGoodFolder(t *testing.T) {
	s := New(core.NewJobManager())
	id, err := s.StartScan(Params{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if id == "" {
		t.Error("StartScan returned no job id")
	}
}

func TestScanSkipsSymlinkedChildren(t *testing.T) {
	root := tree(t, map[string]int{"real/a.txt": 10})
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Skipf("this machine will not create symlinks: %v", err)
	}

	entries, summary, err := runWithContext(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("emitted %d entries, want 1: a symlinked folder must not be measured twice", len(entries))
	}
	if summary["bytes"].(int64) != 10 {
		t.Errorf("bytes = %v, want 10", summary["bytes"])
	}
	if summary["skipped"].(int) != 1 {
		t.Errorf("skipped = %v, want 1", summary["skipped"])
	}
}

func TestScanReportsProgressNamingTheFolder(t *testing.T) {
	root := tree(t, map[string]int{"alpha/a.txt": 10, "beta/b.txt": 20})

	_, progress, _, err := runRecording(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(progress) == 0 {
		t.Fatal("no progress was reported at all")
	}
	joined := strings.Join(progress, "|")
	for _, want := range []string{"Reading alpha", "Reading beta"} {
		if !strings.Contains(joined, want) {
			t.Errorf("progress %q does not contain %q", joined, want)
		}
	}
}

// runScan is the wiring between a JobContext and Scan. Driving it through a
// real JobManager is what stops it sitting at 0% coverage.
func TestRunScanCompletesInsideARealJob(t *testing.T) {
	root := tree(t, map[string]int{"a/b.txt": 10})

	jobs := core.NewJobManager()
	returned := make(chan error, 1)
	jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		err := runScan(jc, root)
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

func pad(i int) string {
	s := ""
	for _, c := range []int{i / 100 % 10, i / 10 % 10, i % 10} {
		s += string(rune('0' + c))
	}
	return s
}

// A cancel that lands after the last child has been measured must still end the
// job as cancelled. The loop's own check cannot see it, so the check after the
// loop is what catches it, and nothing reached that line before this test.
func TestScanCancelAfterTheLastChildStillReportsCancelled(t *testing.T) {
	root := tree(t, map[string]int{"only/a.txt": 10})

	ctx, cancel := context.WithCancel(context.Background())
	_, err := Scan(ctx, root, Sink{
		// Cancelling from the emit callback fires once the last child is done,
		// which is exactly the window the loop check cannot cover.
		Emit:     func(Entry) { cancel() },
		Progress: func(int, string) {},
	})

	if err == nil {
		t.Fatal("Scan returned nil, want the context error so job:done says cancelled")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want a context cancellation", err)
	}
}
