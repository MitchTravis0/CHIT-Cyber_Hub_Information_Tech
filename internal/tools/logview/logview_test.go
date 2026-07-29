package logview

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"chit/internal/core"
)

func fixture(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// tenLines is "line 1\n" ... "line 10\n".
func tenLines() string {
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		b.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	return b.String()
}

func texts(lines []Line) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.Text)
	}
	return out
}

func TestLevel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"an obvious error", "ERROR: disk full", LevelError},
		{"lowercase", "error opening file", LevelError},
		{"bracketed", "2026-07-27 [error] something", LevelError},
		{"fatal", "FATAL exiting", LevelError},
		{"failed", "The operation failed", LevelError},
		{"exception", "Unhandled exception at 0x00", LevelError},
		{"terror is not an error", "the terror of it all", LevelNone},
		{"errors as part of a longer word", "noerrors here", LevelNone},
		{"a hyphen is not a letter, so this does match", "no-error", LevelError},
		{"a warning", "warning: low disk", LevelWarn},
		{"deprecated", "This call is deprecated", LevelWarn},
		{"info", "INFO ready", LevelInfo},
		{"debug", "debug: starting", LevelInfo},
		{"nothing special", "Started the service at 10:02", LevelNone},
		{"empty", "", LevelNone},
		{"error beyond the scan window is ignored", strings.Repeat("x", 300) + " error", LevelNone},
		{"error inside the scan window is found", strings.Repeat("x", 100) + " error", LevelError},
		{"error wins over warning on the same line", "warning: an error occurred", LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Level(tt.in); got != tt.want {
				t.Errorf("Level(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadHead(t *testing.T) {
	path := fixture(t, "a.log", tenLines())

	chunk, err := Read(ReadParams{Path: path, Where: WhereHead, Lines: 3})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := texts(chunk.Lines); strings.Join(got, "|") != "line 1|line 2|line 3" {
		t.Errorf("lines = %v, want the first three", got)
	}
	for i, line := range chunk.Lines {
		if line.Number != i+1 {
			t.Errorf("line %d has Number %d, want %d", i, line.Number, i+1)
		}
	}
	if !chunk.AtStart || chunk.AtEnd {
		t.Errorf("AtStart/AtEnd = %v/%v, want true/false", chunk.AtStart, chunk.AtEnd)
	}

	whole, err := Read(ReadParams{Path: path, Where: WhereHead, Lines: 100})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(whole.Lines) != 10 || !whole.AtEnd {
		t.Errorf("read %d lines, AtEnd %v; want 10 and true", len(whole.Lines), whole.AtEnd)
	}
}

func TestReadTail(t *testing.T) {
	path := fixture(t, "a.log", tenLines())

	chunk, err := Read(ReadParams{Path: path, Where: WhereTail, Lines: 3})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := texts(chunk.Lines); strings.Join(got, "|") != "line 8|line 9|line 10" {
		t.Errorf("lines = %v, want the last three", got)
	}
	for _, line := range chunk.Lines {
		if line.Number != 0 {
			t.Errorf("a tail line has Number %d, want 0: a tail window cannot know it", line.Number)
		}
	}
	if chunk.AtStart || !chunk.AtEnd {
		t.Errorf("AtStart/AtEnd = %v/%v, want false/true", chunk.AtStart, chunk.AtEnd)
	}

	// A tail that reaches byte 0 does know the numbers.
	whole, err := Read(ReadParams{Path: path, Where: WhereTail, Lines: 100})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(whole.Lines) != 10 {
		t.Fatalf("read %d lines, want 10", len(whole.Lines))
	}
	if !whole.AtStart {
		t.Error("AtStart is false for a tail that reached the start of the file")
	}
	if whole.Lines[0].Number != 1 || whole.Lines[9].Number != 10 {
		t.Errorf("numbers = %d..%d, want 1..10 once the window reaches byte 0",
			whole.Lines[0].Number, whole.Lines[9].Number)
	}
}

func TestReadTailAcrossBlockBoundary(t *testing.T) {
	// 2000 lines of 100 bytes crosses many 64 KiB blocks, which is the part of
	// the backwards reader that a small fixture never exercises.
	var b strings.Builder
	for i := 1; i <= 2000; i++ {
		line := "line " + strconv.Itoa(i)
		b.WriteString(line + strings.Repeat("-", 99-len(line)) + "\n")
	}
	path := fixture(t, "big.log", b.String())

	chunk, err := Read(ReadParams{Path: path, Where: WhereTail, Lines: 500})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(chunk.Lines) != 500 {
		t.Fatalf("read %d lines, want 500", len(chunk.Lines))
	}
	if !strings.HasPrefix(chunk.Lines[0].Text, "line 1501-") {
		t.Errorf("first line is %q, want line 1501", chunk.Lines[0].Text)
	}
	if !strings.HasPrefix(chunk.Lines[499].Text, "line 2000-") {
		t.Errorf("last line is %q, want line 2000", chunk.Lines[499].Text)
	}
}

func TestReadNoTrailingNewline(t *testing.T) {
	path := fixture(t, "a.log", "one\ntwo\nthree")

	head, err := Read(ReadParams{Path: path, Where: WhereHead, Lines: 100})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := texts(head.Lines); strings.Join(got, "|") != "one|two|three" {
		t.Errorf("head = %v, want all three: a last line with no newline is still a line", got)
	}

	tail, err := Read(ReadParams{Path: path, Where: WhereTail, Lines: 1})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(tail.Lines) != 1 || tail.Lines[0].Text != "three" {
		t.Errorf("tail = %v, want the unterminated last line", texts(tail.Lines))
	}
}

func TestReadCRLF(t *testing.T) {
	path := fixture(t, "win.log", "alpha\r\nbeta\r\n")

	info, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !info.CRLF {
		t.Error("CRLF is false for a file with Windows line endings")
	}

	chunk, err := Read(ReadParams{Path: path, Where: WhereHead, Lines: 10})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := texts(chunk.Lines); strings.Join(got, "|") != "alpha|beta" {
		t.Errorf("lines = %q, want no carriage returns", got)
	}
}

func TestReadStripsByteOrderMark(t *testing.T) {
	// The escape, never the character: a literal byte order mark in a Go source
	// file is a compile error.
	path := fixture(t, "bom.log", "\uFEFFfirst\nsecond\n")

	chunk, err := Read(ReadParams{Path: path, Where: WhereHead, Lines: 10})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if chunk.Lines[0].Text != "first" {
		t.Errorf("first line = %q, want %q with no mark", chunk.Lines[0].Text, "first")
	}
}

func TestReadBeforeAndAtPageBothWays(t *testing.T) {
	path := fixture(t, "a.log", tenLines())

	end, err := Read(ReadParams{Path: path, Where: WhereTail, Lines: 3})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if got := strings.Join(texts(end.Lines), "|"); got != "line 8|line 9|line 10" {
		t.Fatalf("tail = %s", got)
	}

	older, err := Read(ReadParams{Path: path, Where: WhereBefore, Offset: end.Start, Lines: 3})
	if err != nil {
		t.Fatalf("before: %v", err)
	}
	if got := strings.Join(texts(older.Lines), "|"); got != "line 5|line 6|line 7" {
		t.Errorf("older = %s, want lines 5 to 7: a before read must not run back over the window it came from", got)
	}

	newer, err := Read(ReadParams{Path: path, Where: WhereAt, Offset: older.End, Lines: 3})
	if err != nil {
		t.Fatalf("at: %v", err)
	}
	if got := strings.Join(texts(newer.Lines), "|"); got != "line 8|line 9|line 10" {
		t.Errorf("newer = %s, want back where we started", got)
	}
}

func TestReadAtStartsOnALineBoundary(t *testing.T) {
	path := fixture(t, "a.log", tenLines())

	// Byte 3 is inside "line 1", so the window must begin at byte 0.
	chunk, err := Read(ReadParams{Path: path, Where: WhereAt, Offset: 3, Lines: 1})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if chunk.Start != 0 || chunk.Lines[0].Text != "line 1" {
		t.Errorf("start %d line %q, want 0 and \"line 1\"", chunk.Start, chunk.Lines[0].Text)
	}
}

func TestReadClampsPastEnd(t *testing.T) {
	path := fixture(t, "a.log", tenLines())

	chunk, err := Read(ReadParams{Path: path, Where: WhereAt, Offset: 1 << 30, Lines: 5})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !chunk.Shrank {
		t.Error("Shrank is false for an offset past the end of the file")
	}
	if !chunk.AtEnd {
		t.Error("AtEnd is false after clamping to the end")
	}
}

func TestLongLineTruncated(t *testing.T) {
	// 4000 is the number in the "line cut at 4000 characters" marker the page
	// shows, so it is written here rather than read from the constant.
	const want = 4000
	if maxLineBytes != want {
		t.Fatalf("maxLineBytes = %d, want %d: the page quotes this number", maxLineBytes, want)
	}

	path := fixture(t, "long.log", strings.Repeat("a", 10000)+"\nshort\n")

	chunk, err := Read(ReadParams{Path: path, Where: WhereHead, Lines: 10})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(chunk.Lines[0].Text) != want {
		t.Errorf("line length = %d, want %d", len(chunk.Lines[0].Text), want)
	}
	if !chunk.Lines[0].Truncated {
		t.Error("Truncated is false for a cut line")
	}
	if chunk.Lines[1].Text != "short" {
		t.Errorf("the line after a cut one is %q, want \"short\"", chunk.Lines[1].Text)
	}
	if chunk.Lines[1].Truncated {
		t.Error("Truncated is true for a short line")
	}
}

func TestWindowByteCap(t *testing.T) {
	// One 20 MiB line with no newline until the very end. The window must stop
	// at maxWindowBytes rather than pulling all of it in.
	path := fixture(t, "blob.log", strings.Repeat("a", 20<<20)+"\n")

	chunk, err := Read(ReadParams{Path: path, Where: WhereHead, Lines: 500})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if chunk.End > maxWindowBytes {
		t.Errorf("window ended at byte %d, want at most %d", chunk.End, maxWindowBytes)
	}
	if chunk.AtEnd {
		t.Error("AtEnd is true although the file is far longer than the window")
	}
}

func TestOpenRefusesBinary(t *testing.T) {
	path := fixture(t, "prog.log", "MZ\x00\x00this is a program")

	_, err := Open(path)
	if err == nil {
		t.Fatal("Open accepted a file with a NUL byte in it")
	}
	want := path + " does not look like a text file, so there is nothing readable to show. Log files are plain text; this one is a program, a database or a compressed archive."
	if got := core.MessageOf(err); got != want {
		t.Errorf("message =\n%q\nwant\n%q", got, want)
	}
}

func TestOpenAcceptsANulBeyondTheSniffWindow(t *testing.T) {
	// Only the first 8 KiB is sniffed, and that is the intended behaviour: a
	// 40 GB log must not be read end to end just to decide whether to show it.
	path := fixture(t, "late.log", strings.Repeat("a", 9000)+"\x00\n")

	if _, err := Open(path); err != nil {
		t.Fatalf("Open refused a file whose only NUL is past the sniff window: %v", err)
	}
}

func TestOpenRefusesByExtension(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"System.evtx", "%s is a Windows event log, which is a database rather than a text file. Open it with Event Viewer, or export it to .txt from there and open that here."},
		{"System.EVTX", "%s is a Windows event log, which is a database rather than a text file. Open it with Event Viewer, or export it to .txt from there and open that here."},
		{"old.gz", "%s is a compressed archive. Extract it first, then open the log file inside it."},
		{"old.ZIP", "%s is a compressed archive. Extract it first, then open the log file inside it."},
		{"old.7z", "%s is a compressed archive. Extract it first, then open the log file inside it."},
		{"old.bz2", "%s is a compressed archive. Extract it first, then open the log file inside it."},
		{"old.xz", "%s is a compressed archive. Extract it first, then open the log file inside it."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, tt.name, "plain text really")
			_, err := Open(path)
			if err == nil {
				t.Fatalf("Open accepted %s", tt.name)
			}
			want := strings.Replace(tt.want, "%s", path, 1)
			if got := core.MessageOf(err); got != want {
				t.Errorf("message =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestOpenRefusesAFolderAndAMissingFile(t *testing.T) {
	dir := t.TempDir()

	_, err := Open(dir)
	if core.CodeOf(err) != core.CodeInvalidInput {
		t.Errorf("a folder gave code %q, want %q", core.CodeOf(err), core.CodeInvalidInput)
	}

	missing := filepath.Join(dir, "nope.log")
	_, err = Open(missing)
	if core.CodeOf(err) != core.CodeNotFound {
		t.Errorf("a missing file gave code %q, want %q", core.CodeOf(err), core.CodeNotFound)
	}
	want := "There is no file at " + missing + ". Check the path, or press Choose file and pick it."
	if got := core.MessageOf(err); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestOpenEmptyFileIsFine(t *testing.T) {
	path := fixture(t, "empty.log", "")

	info, err := Open(path)
	if err != nil {
		t.Fatalf("Open refused an empty file: %v", err)
	}
	if info.Bytes != 0 {
		t.Errorf("Bytes = %d, want 0", info.Bytes)
	}

	chunk, err := Read(ReadParams{Path: path, Where: WhereTail, Lines: 10})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(chunk.Lines) != 0 {
		t.Errorf("read %d lines from an empty file, want 0", len(chunk.Lines))
	}
}

func TestValidateReadParams(t *testing.T) {
	tests := []struct {
		name      string
		in        ReadParams
		wantErr   bool
		wantLines int
		wantWhere string
	}{
		{name: "zero lines means the default", in: ReadParams{Path: "x"}, wantLines: 500, wantWhere: WhereTail},
		{name: "one line", in: ReadParams{Path: "x", Lines: 1}, wantLines: 1, wantWhere: WhereTail},
		{name: "the maximum", in: ReadParams{Path: "x", Lines: 5000}, wantLines: 5000, wantWhere: WhereTail},
		{name: "one past the maximum", in: ReadParams{Path: "x", Lines: 5001}, wantErr: true},
		{name: "negative lines", in: ReadParams{Path: "x", Lines: -1}, wantErr: true},
		{name: "an unknown where", in: ReadParams{Path: "x", Where: "sideways"}, wantErr: true},
		{name: "an empty path", in: ReadParams{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("validate accepted bad params")
				}
				if core.CodeOf(err) != core.CodeInvalidInput {
					t.Errorf("code = %q, want %q", core.CodeOf(err), core.CodeInvalidInput)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if got.Lines != tt.wantLines {
				t.Errorf("Lines = %d, want %d", got.Lines, tt.wantLines)
			}
			if got.Where != tt.wantWhere {
				t.Errorf("Where = %q, want %q", got.Where, tt.wantWhere)
			}
		})
	}
}

func TestMaxLinesRejectionMessage(t *testing.T) {
	_, err := ReadParams{Path: "x", Lines: 5001}.validate()
	// 5000 is quoted to the user, so the message is checked as a whole string.
	if got := core.MessageOf(err); got != "Show between 1 and 5000 lines at a time." {
		t.Errorf("message = %q", got)
	}
}

// TestTailMatchesSystemTail is the independent oracle. Without it the backwards
// reader would be checked only against the forward reader written by the same
// hand, which SPECS/CONVENTIONS.md 8.3 names as insufficient.
func TestTailMatchesSystemTail(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the system tail command")
	}
	tail, err := exec.LookPath("tail")
	if err != nil {
		t.Skip("this machine has no tail command, so there is nothing to compare against")
	}

	var b strings.Builder
	for i := 1; i <= 5000; i++ {
		b.WriteString("entry " + strconv.Itoa(i) + " of the log\n")
	}
	path := fixture(t, "oracle.log", b.String())

	out, err := exec.Command(tail, "-n", "500", path).Output()
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	want := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")

	chunk, err := Read(ReadParams{Path: path, Where: WhereTail, Lines: 500})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	got := texts(chunk.Lines)

	if len(got) != len(want) {
		t.Fatalf("CHIT returned %d lines, tail returned %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d differs: CHIT %q, tail %q", i, got[i], want[i])
		}
	}
}
