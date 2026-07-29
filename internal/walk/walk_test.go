package walk

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"chit/internal/core"
)

// tree builds a fixture directory and returns its root.
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

func collect(t *testing.T, root string, opts Options) ([]File, Result) {
	t.Helper()
	opts.Root = root
	var got []File
	res, err := Walk(context.Background(), opts, func(f File) error {
		got = append(got, f)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Path < got[j].Path })
	return got, res
}

func TestWalkCountsFilesAndBytes(t *testing.T) {
	root := tree(t, map[string]int{
		"a.txt":       10,
		"one/b.txt":   20,
		"one/two/c.b": 30,
	})

	files, res := collect(t, root, Options{})

	if len(files) != 3 {
		t.Fatalf("visited %d files, want 3", len(files))
	}
	if res.Files != 3 {
		t.Errorf("Files = %d, want 3", res.Files)
	}
	// 10 + 20 + 30, written out rather than summed from the fixture.
	if res.Bytes != 60 {
		t.Errorf("Bytes = %d, want 60", res.Bytes)
	}
	// The root, one, and one/two.
	if res.Dirs != 3 {
		t.Errorf("Dirs = %d, want 3", res.Dirs)
	}
	if res.Skipped != 0 || res.Unreadable != 0 {
		t.Errorf("Skipped = %d, Unreadable = %d, want 0 and 0", res.Skipped, res.Unreadable)
	}
}

func TestWalkEmptyRoot(t *testing.T) {
	files, res := collect(t, t.TempDir(), Options{})
	if len(files) != 0 {
		t.Fatalf("visited %d files, want 0", len(files))
	}
	if res.Dirs != 1 {
		t.Errorf("Dirs = %d, want 1 (the root itself)", res.Dirs)
	}
}

func TestWalkNeverFollowsSymlinks(t *testing.T) {
	root := tree(t, map[string]int{"real/a.txt": 10})

	// A link pointing at its own ancestor is the loop that hangs a naive walk.
	if err := os.Symlink(root, filepath.Join(root, "real", "loop")); err != nil {
		t.Skipf("this machine will not create symlinks (%v); on Windows that needs Developer Mode", err)
	}
	if err := os.Symlink(filepath.Join(root, "real", "a.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}

	files, res := collect(t, root, Options{})

	if len(files) != 1 {
		t.Fatalf("visited %d files, want 1: a symlinked file must not be visited", len(files))
	}
	if !strings.HasSuffix(files[0].Path, filepath.Join("real", "a.txt")) {
		t.Errorf("visited %q, want the real file", files[0].Path)
	}
	// One directory link plus one file link.
	if res.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", res.Skipped)
	}
	if res.Bytes != 10 {
		t.Errorf("Bytes = %d, want 10: the link must not be counted twice", res.Bytes)
	}
}

func TestWalkCountsUnreadableDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 does not deny listing on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, which can read a 0000 directory")
	}

	root := tree(t, map[string]int{
		"aaa/locked.txt": 10,
		"zzz/after.txt":  20,
	})
	locked := filepath.Join(root, "aaa")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	files, res := collect(t, root, Options{})

	if res.Unreadable != 1 {
		t.Errorf("Unreadable = %d, want 1", res.Unreadable)
	}
	// "zzz" sorts after "aaa", so this proves the walk carried on.
	if len(files) != 1 || !strings.HasSuffix(files[0].Path, "after.txt") {
		t.Fatalf("visited %v, want only zzz/after.txt: one locked folder must not abandon the walk", files)
	}
}

func TestWalkHonoursContext(t *testing.T) {
	files := map[string]int{}
	for i := 0; i < 200; i++ {
		files[filepath.ToSlash(filepath.Join("d", string(rune('a'+i%26))+string(rune('a'+i/26))+".txt"))] = 1
	}
	root := tree(t, files)

	ctx, cancel := context.WithCancel(context.Background())
	seen := 0
	_, err := Walk(ctx, Options{Root: root}, func(File) error {
		seen++
		if seen == 5 {
			cancel()
		}
		return nil
	})

	if err == nil {
		t.Fatal("Walk returned nil, want the context error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want a context cancellation", err)
	}
	if seen >= 200 {
		t.Errorf("visited %d files after a cancel, want far fewer than the whole tree", seen)
	}
}

func TestWalkVisitErrorStops(t *testing.T) {
	root := tree(t, map[string]int{"a.txt": 1, "b.txt": 1, "c.txt": 1})

	want := core.Errorf(core.CodeInternal, "stop here")
	seen := 0
	_, err := Walk(context.Background(), Options{Root: root}, func(File) error {
		seen++
		return want
	})

	if err != want {
		t.Fatalf("err = %v, want the exact error visit returned", err)
	}
	if seen != 1 {
		t.Errorf("visited %d files, want 1: the walk must stop on the first error", seen)
	}
}

func TestWalkMinSize(t *testing.T) {
	root := tree(t, map[string]int{"small.txt": 10, "equal.txt": 20, "big.txt": 30})

	tests := []struct {
		name    string
		minSize int64
		want    int
		bytes   int64
	}{
		{"zero keeps everything", 0, 3, 60},
		{"at the boundary keeps equal and above", 20, 2, 50},
		{"above everything keeps nothing", 31, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, res := collect(t, root, Options{MinSize: tt.minSize})
			if len(files) != tt.want {
				t.Errorf("visited %d files, want %d", len(files), tt.want)
			}
			if res.Bytes != tt.bytes {
				t.Errorf("Bytes = %d, want %d", res.Bytes, tt.bytes)
			}
		})
	}
}

func TestSkipPathExactMatchOnly(t *testing.T) {
	// Written with forward slashes and converted, because skipPath runs the
	// input through filepath.Clean: on Windows "/proc" cleans to "\proc" and
	// would never equal a root spelled with a forward slash. Converting both
	// sides keeps one table meaningful on all three platforms rather than
	// making this a Unix-only test.
	roots := []string{"/proc", "/sys", "/dev", "/run"}
	for i, root := range roots {
		roots[i] = filepath.FromSlash(root)
	}

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{"the pseudo filesystem itself", "/proc", true},
		{"with a trailing slash", "/proc/", true},
		{"an uncleaned path", "/proc/../proc", true},
		{"a folder that starts with the same letters", "/devices", false},
		{"a user folder with the same name deeper down", "/home/me/dev", false},
		{"a child of a skipped root", "/proc/1", false},
		{"an unrelated path", "/home/me/projects", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.FromSlash(tt.dir)
			if got := skipPath(dir, roots); got != tt.want {
				t.Errorf("skipPath(%q) = %v, want %v", dir, got, tt.want)
			}
		})
	}
}

func TestSkipPathEmptyListSkipsNothing(t *testing.T) {
	if skipPath("/proc", nil) {
		t.Error("an empty skip list must skip nothing")
	}
}

func TestRootMessages(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope")

	tests := []struct {
		name     string
		in       string
		wantCode string
		wantMsg  string
	}{
		{"empty", "", core.CodeInvalidInput,
			"Choose a folder to scan, or type the full path to it."},
		{"whitespace only", "   ", core.CodeInvalidInput,
			"Choose a folder to scan, or type the full path to it."},
		{"missing", missing, core.CodeNotFound,
			"There is no folder at " + missing + ". Check the path, or press Choose folder and pick it."},
		{"a file", file, core.CodeInvalidInput,
			file + " is a file, not a folder. Pick the folder it is in."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Root(tt.in)
			if err == nil {
				t.Fatal("Root returned nil, want an error")
			}
			if got := core.CodeOf(err); got != tt.wantCode {
				t.Errorf("code = %q, want %q", got, tt.wantCode)
			}
			if got := core.MessageOf(err); got != tt.wantMsg {
				t.Errorf("message =\n%q\nwant\n%q", got, tt.wantMsg)
			}
		})
	}
}

func TestRootAcceptsAFolder(t *testing.T) {
	dir := t.TempDir()
	got, err := Root(dir + string(filepath.Separator) + ".")
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if got != filepath.Clean(dir) {
		t.Errorf("Root = %q, want the cleaned absolute path %q", got, filepath.Clean(dir))
	}
}

func TestRootRejectsUnreadableFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 does not deny listing on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, which can read a 0000 directory")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	_, err := Root(dir)
	if err == nil {
		t.Fatal("Root returned nil for an unreadable folder")
	}
	if got := core.CodeOf(err); got != core.CodePermission {
		t.Errorf("code = %q, want %q", got, core.CodePermission)
	}
}

func TestResultAdd(t *testing.T) {
	a := Result{Files: 1, Dirs: 2, Bytes: 3, Unreadable: 4, Skipped: 5}
	a.Add(Result{Files: 10, Dirs: 20, Bytes: 30, Unreadable: 40, Skipped: 50})
	want := Result{Files: 11, Dirs: 22, Bytes: 33, Unreadable: 44, Skipped: 55}
	if a != want {
		t.Errorf("Add = %+v, want %+v", a, want)
	}
}
