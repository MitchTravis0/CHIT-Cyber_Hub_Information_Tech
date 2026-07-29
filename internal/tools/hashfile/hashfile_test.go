package hashfile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

// The three published constants for the input "abc", used by every comparison
// test so no test depends on a file this machine happens to have.
const (
	abcMD5    = "900150983cd24fb0d6963f7d28e17f72"
	abcSHA1   = "a9993e364706816aba3e25717850c26c9cd0d89d"
	abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
)

func abcDigests() Digests {
	return Digests{Path: "/tmp/abc.txt", Name: "abc.txt", Bytes: 3, MD5: abcMD5, SHA1: abcSHA1, SHA256: abcSHA256}
}

func TestNormalizeExpected(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare hex", "abc123", "abc123"},
		{"surrounding spaces", "  abc123  ", "abc123"},
		{"uppercase", "ABC123", "abc123"},
		{"trailing newline", "abc123\n", "abc123"},
		{"trailing crlf", "abc123\r\n", "abc123"},
		{"tab then file name", "abc123\tsetup.exe", "abc123"},
		{"sha256sum line", "abc123  setup.exe", "abc123"},
		{"sha256sum binary line", "abc123 *setup.exe", "abc123"},
		{"leading backslash", `\abc123  odd\name.iso`, "abc123"},
		{"openssl dgst", "SHA256(setup.exe)= abc123", "abc123"},
		{"openssl dgst spaced", "SHA256 (setup.exe) = abc123", "abc123"},
		{"only after the last equals", "MD5(a)= b= abc123", "abc123"},
		{"openssl dgst with no space after the equals", "SHA256(setup.exe)=abc123", "abc123"},
		// A file name may contain "=". Splitting on the last "=" regardless of
		// where it sits threw the hash away and reported a correct value as "that
		// does not look like a hash".
		{"equals in the file name", "abc123  my=file.iso", "abc123"},
		{"equals in a binary-mode file name", "abc123 *a=b.iso", "abc123"},
		{"equals in a spaced file name", "abc123  report = final.txt", "abc123"},
		// Found by mutation: each of these is the only input that tells the
		// first-field test apart from a weaker version of it.
		{"backslash prefix and equals in the file name", `\abc123  my=file.iso`, "abc123"},
		{"uppercase hash and equals in the file name", "ABC123  my=file.iso", "abc123"},
		{"uppercase openssl dgst", "SHA256(setup.exe)= ABC123", "abc123"},
		{"a lone backslash is not a hash", `\  name=value`, "value"},
		{"empty", "", ""},
		{"spaces only", "   ", ""},
		{"newlines only", "\n\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeExpected(tt.in); got != tt.want {
				t.Errorf("NormalizeExpected(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDetectAlgorithmByLength(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		wantState     string
		wantAlgorithm string
	}{
		{"32 is md5", strings.Repeat("a", 32), "mismatch", "MD5"},
		{"40 is sha1", strings.Repeat("a", 40), "mismatch", "SHA-1"},
		{"64 is sha256", strings.Repeat("a", 64), "mismatch", "SHA-256"},
		{"31", strings.Repeat("a", 31), "unknown", ""},
		{"33", strings.Repeat("a", 33), "unknown", ""},
		{"39", strings.Repeat("a", 39), "unknown", ""},
		{"41", strings.Repeat("a", 41), "unknown", ""},
		{"63", strings.Repeat("a", 63), "unknown", ""},
		{"65", strings.Repeat("a", 65), "unknown", ""},
		{"0", "", "none", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(tt.in, abcDigests())
			if got.State != tt.wantState {
				t.Errorf("State = %q, want %q", got.State, tt.wantState)
			}
			if got.Algorithm != tt.wantAlgorithm {
				t.Errorf("Algorithm = %q, want %q", got.Algorithm, tt.wantAlgorithm)
			}
			if tt.wantState == "unknown" && got.Message != notAHashMessage {
				t.Errorf("Message = %q, want %q", got.Message, notAHashMessage)
			}
		})
	}
}

func TestCompareRejectsNonHex(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"64 with a g", strings.Repeat("a", 63) + "g"},
		{"32 with a dash", strings.Repeat("a", 31) + "-"},
		{"40 with an underscore in the middle", strings.Repeat("a", 20) + "_" + strings.Repeat("a", 19)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(tt.in, abcDigests())
			if got.State != "unknown" {
				t.Errorf("State = %q, want %q", got.State, "unknown")
			}
			if got.Algorithm != "" {
				t.Errorf("Algorithm = %q, want empty", got.Algorithm)
			}
			if got.Actual != "" {
				t.Errorf("Actual = %q, want empty", got.Actual)
			}
			if got.Message != nonHexMessage {
				t.Errorf("Message = %q, want %q", got.Message, nonHexMessage)
			}
		})
	}
}

func TestCompareVerdicts(t *testing.T) {
	wrongMD5 := strings.Repeat("0", 32)
	wrongSHA1 := strings.Repeat("0", 40)
	wrongSHA256 := strings.Repeat("0", 64)

	tests := []struct {
		name          string
		in            string
		wantState     string
		wantAlgorithm string
		wantExpected  string
		wantActual    string
		wantMessage   string
	}{
		{"empty", "", "none", "", "", "", ""},
		{"md5 match", abcMD5, "match", "MD5", abcMD5, abcMD5,
			"This is the same file. The MD5 hash matches the value you pasted, so the file is complete and has not been altered since it was published."},
		{"sha1 match", abcSHA1, "match", "SHA-1", abcSHA1, abcSHA1,
			"This is the same file. The SHA-1 hash matches the value you pasted, so the file is complete and has not been altered since it was published."},
		{"sha256 match", abcSHA256, "match", "SHA-256", abcSHA256, abcSHA256,
			"This is the same file. The SHA-256 hash matches the value you pasted, so the file is complete and has not been altered since it was published."},
		{"md5 mismatch", wrongMD5, "mismatch", "MD5", wrongMD5, abcMD5,
			"This is NOT the same file. The MD5 hash does not match the value you pasted. Do not install or run it. Download it again, and if it still does not match, get the file from somewhere else."},
		{"sha1 mismatch", wrongSHA1, "mismatch", "SHA-1", wrongSHA1, abcSHA1,
			"This is NOT the same file. The SHA-1 hash does not match the value you pasted. Do not install or run it. Download it again, and if it still does not match, get the file from somewhere else."},
		{"sha256 mismatch", wrongSHA256, "mismatch", "SHA-256", wrongSHA256, abcSHA256,
			"This is NOT the same file. The SHA-256 hash does not match the value you pasted. Do not install or run it. Download it again, and if it still does not match, get the file from somewhere else."},
		{"unknown length", "abc", "unknown", "", "abc", "", notAHashMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(tt.in, abcDigests())
			want := Verdict{
				State:     tt.wantState,
				Algorithm: tt.wantAlgorithm,
				Expected:  tt.wantExpected,
				Actual:    tt.wantActual,
				Message:   tt.wantMessage,
			}
			if got != want {
				t.Errorf("Compare(%q) = %+v, want %+v", tt.in, got, want)
			}
		})
	}
}

func TestCompareIsCaseInsensitive(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		wantAlgorithm string
	}{
		{"md5", strings.ToUpper(abcMD5), "MD5"},
		{"sha1", strings.ToUpper(abcSHA1), "SHA-1"},
		{"sha256", strings.ToUpper(abcSHA256), "SHA-256"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(tt.in, abcDigests())
			if got.State != "match" {
				t.Errorf("State = %q, want match", got.State)
			}
			if got.Algorithm != tt.wantAlgorithm {
				t.Errorf("Algorithm = %q, want %q", got.Algorithm, tt.wantAlgorithm)
			}
			if got.Expected != strings.ToLower(tt.in) {
				t.Errorf("Expected = %q, want the lowercase form", got.Expected)
			}
		})
	}
}

// writeFixture puts content in a fresh temp file and returns its path and size.
func writeFixture(t *testing.T, name string, content []byte) (string, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	return path, int64(len(content))
}

func TestDigestsOfEmptyFile(t *testing.T) {
	path, size := writeFixture(t, "empty.bin", nil)

	got, err := hashPath(context.Background(), path, size, func(read, total int64) {})
	if err != nil {
		t.Fatalf("hashPath: %v", err)
	}
	if got.Bytes != 0 {
		t.Errorf("Bytes = %d, want 0", got.Bytes)
	}
	if got.MD5 != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("MD5 = %q", got.MD5)
	}
	if got.SHA1 != "da39a3ee5e6b4b0d3255bfef95601890afd80709" {
		t.Errorf("SHA1 = %q", got.SHA1)
	}
	if got.SHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("SHA256 = %q", got.SHA256)
	}
	if got.Name != "empty.bin" {
		t.Errorf("Name = %q, want empty.bin", got.Name)
	}
}

func TestDigestsOfKnownInput(t *testing.T) {
	path, size := writeFixture(t, "abc.txt", []byte("abc"))

	got, err := hashPath(context.Background(), path, size, func(read, total int64) {})
	if err != nil {
		t.Fatalf("hashPath: %v", err)
	}
	if got.Bytes != 3 {
		t.Errorf("Bytes = %d, want 3", got.Bytes)
	}
	if got.MD5 != abcMD5 {
		t.Errorf("MD5 = %q, want %q", got.MD5, abcMD5)
	}
	if got.SHA1 != abcSHA1 {
		t.Errorf("SHA1 = %q, want %q", got.SHA1, abcSHA1)
	}
	if got.SHA256 != abcSHA256 {
		t.Errorf("SHA256 = %q, want %q", got.SHA256, abcSHA256)
	}
}

func TestDigestsStreamALargeFile(t *testing.T) {
	content := bytes.Repeat([]byte{0x00, 0x7f, 0xff, 0x42}, 5<<20/4)
	path, size := writeFixture(t, "big.bin", content)

	calls := 0
	var last int64 = -1
	got, err := hashPath(context.Background(), path, size, func(read, total int64) {
		calls++
		if read < last {
			t.Errorf("read went backwards: %d after %d", read, last)
		}
		last = read
		if total != size {
			t.Errorf("total = %d, want %d", total, size)
		}
	})
	if err != nil {
		t.Fatalf("hashPath: %v", err)
	}

	want := sha256.Sum256(content)
	if got.SHA256 != hex.EncodeToString(want[:]) {
		t.Errorf("SHA256 = %q, want %q", got.SHA256, hex.EncodeToString(want[:]))
	}
	if got.Bytes != size {
		t.Errorf("Bytes = %d, want %d", got.Bytes, size)
	}
	if calls < 2 {
		t.Errorf("report was called %d times, want more than once", calls)
	}
	if last != size {
		t.Errorf("the last report said %d bytes, want %d", last, size)
	}
}

func TestHashCancelledMidFile(t *testing.T) {
	content := bytes.Repeat([]byte{0x5a}, 5<<20)
	path, size := writeFixture(t, "big.bin", content)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got, err := hashPath(ctx, path, size, func(read, total int64) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got != (Digests{}) {
		t.Errorf("Digests = %+v, want the zero value", got)
	}
}

func TestHashCancelledBeforeStart(t *testing.T) {
	content := bytes.Repeat([]byte{0x5a}, 1024)
	path, size := writeFixture(t, "small.bin", content)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	read := int64(-1)
	got, err := hashPath(ctx, path, size, func(r, total int64) { read = r })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got != (Digests{}) {
		t.Errorf("Digests = %+v, want the zero value", got)
	}
	if read != 0 {
		t.Errorf("the last report said %d bytes read, want 0", read)
	}
}

func TestValidateParams(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nothing-here.iso")

	tests := []struct {
		name        string
		path        string
		wantCode    string
		wantMessage string
	}{
		{"empty", "", core.CodeInvalidInput, "Choose a file to check, or type the full path to it."},
		{"whitespace", "   ", core.CodeInvalidInput, "Choose a file to check, or type the full path to it."},
		{"does not exist", missing, core.CodeNotFound,
			"There is no file at " + missing + ". Check the path, or press Choose file and pick it."},
		{"a folder", dir, core.CodeInvalidInput,
			dir + " is a folder, not a file. Pick the file inside it that you want to check."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := validate(Params{Path: tt.path})
			if err == nil {
				t.Fatal("validate accepted the path, want an error")
			}
			if got := core.CodeOf(err); got != tt.wantCode {
				t.Errorf("code = %q, want %q", got, tt.wantCode)
			}
			if got := core.MessageOf(err); got != tt.wantMessage {
				t.Errorf("message = %q, want %q", got, tt.wantMessage)
			}
		})
	}
}

func TestValidateAcceptsAFile(t *testing.T) {
	path, size := writeFixture(t, "abc.txt", []byte("abc"))

	gotPath, gotSize, err := validate(Params{Path: "  " + path + "  "})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if gotPath != path {
		t.Errorf("path = %q, want %q", gotPath, path)
	}
	if gotSize != size {
		t.Errorf("size = %d, want %d", gotSize, size)
	}
}

func TestNoteFor(t *testing.T) {
	tests := []struct {
		name string
		read int64
		size int64
		want string
	}{
		{"empty file", 0, 0,
			"That file is empty (0 bytes). These are the hashes of an empty file, which are the same for every empty file. If you were expecting content, the download or the copy did not finish."},
		{"read matches the size", 4096, 4096, ""},
		{"the file grew while it was read", 5000, 4096,
			"The file changed size while it was being read, so these hashes may not match the finished file. Wait until the download or the copy has finished, then check it again."},
		{"the file shrank while it was read", 2048, 4096,
			"The file changed size while it was being read, so these hashes may not match the finished file. Wait until the download or the copy has finished, then check it again."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noteFor(tt.read, tt.size); got != tt.want {
				t.Errorf("noteFor(%d, %d) = %q, want %q", tt.read, tt.size, got, tt.want)
			}
		})
	}
}

// TestSummaryForCarriesEveryKeyThePageReads pins the job:done summary. Nothing
// else can: a map[string]any is not type checked in Go and arrives in
// TypeScript as Record<string, unknown>, so a renamed key shows up only as a
// blank field on the page.
func TestSummaryForCarriesEveryKeyThePageReads(t *testing.T) {
	d := abcDigests()
	want := map[string]any{
		"path":   "/tmp/abc.txt",
		"name":   "abc.txt",
		"bytes":  int64(3),
		"md5":    abcMD5,
		"sha1":   abcSHA1,
		"sha256": abcSHA256,
		"note":   "",
	}

	got := summaryFor(d, d.Bytes)
	if len(got) != len(want) {
		t.Errorf("the summary has %d keys, want %d: %v", len(got), len(want), got)
	}
	for key, value := range want {
		switch have, ok := got[key]; {
		case !ok:
			t.Errorf("the summary has no %q key", key)
		case have != value:
			t.Errorf("summary[%q] = %v, want %v", key, have, value)
		}
	}

	grew := summaryFor(d, d.Bytes+1)
	if note := grew["note"]; note != "The file changed size while it was being read, so these hashes may not match the finished file. Wait until the download or the copy has finished, then check it again." {
		t.Errorf("summary[\"note\"] for a file that changed size = %v", note)
	}
}

// TestCancelledJobReturnsAnErrorCoreReadsAsCancelled pins the invariant the
// comment in StartHash calls out. core.JobManager.Start reports a job as
// cancelled when the context is done or the job function returned an error that
// wraps context.Canceled, and only then does it send job:done instead of
// job:error, so that is what the job body has to hand back.
func TestCancelledJobReturnsAnErrorCoreReadsAsCancelled(t *testing.T) {
	content := bytes.Repeat([]byte{0x5a}, 5<<20)
	path, size := writeFixture(t, "big.bin", content)

	jobs := core.NewJobManager()
	started := make(chan struct{})
	returned := make(chan error, 1)

	id := jobs.Start(JobKind, int(size), func(jc *core.JobContext) error {
		close(started)
		<-jc.Ctx().Done()
		returned <- runHash(jc, path, size)
		return nil
	})

	<-started
	if err := jobs.Cancel(id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("the job body returned %v (code %q), which core would send as job:error", err, core.CodeOf(err))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the job body did not return within 10 seconds of the cancel")
	}
}

func TestStartHashRejectsABadPathWithoutStartingAJob(t *testing.T) {
	jobs := core.NewJobManager()

	id, err := New(jobs).StartHash(Params{Path: "  "})
	if err == nil {
		t.Fatal("StartHash accepted an empty path")
	}
	if got := core.CodeOf(err); got != core.CodeInvalidInput {
		t.Errorf("code = %q, want %q", got, core.CodeInvalidInput)
	}
	if id != "" {
		t.Errorf("job id = %q, want an empty string", id)
	}
	if running := jobs.Running(); running != 0 {
		t.Errorf("%d jobs are running, want none", running)
	}
}

func TestStartHashRunsAJobToCompletion(t *testing.T) {
	path, _ := writeFixture(t, "abc.txt", []byte("abc"))
	jobs := core.NewJobManager()

	id, err := New(jobs).StartHash(Params{Path: path})
	if err != nil {
		t.Fatalf("StartHash: %v", err)
	}
	if id == "" {
		t.Fatal("StartHash returned an empty job id")
	}

	deadline := time.Now().Add(10 * time.Second)
	for jobs.Running() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the job was still running 10 seconds later")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
