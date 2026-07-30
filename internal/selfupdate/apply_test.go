package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeTarGz builds a release-shaped tarball: name -> content, every entry a
// regular file.
func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type zipEntry struct {
	name    string
	content string
	mode    os.FileMode
	dir     bool
}

func makeZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		if entry.dir {
			header.SetMode(entry.mode | os.ModeDir)
		} else {
			header.SetMode(entry.mode)
		}
		w, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if !entry.dir {
			if _, err := w.Write([]byte(entry.content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// appZip is the shape the release workflow's "ditto" step produces.
func appZip(t *testing.T, binaryContent string) []byte {
	t.Helper()
	return makeZip(t, []zipEntry{
		{name: "chit.app/", mode: 0o755, dir: true},
		{name: "chit.app/Contents/", mode: 0o755, dir: true},
		{name: "chit.app/Contents/Info.plist", content: "<plist/>", mode: 0o644},
		{name: "chit.app/Contents/MacOS/", mode: 0o755, dir: true},
		{name: "chit.app/Contents/MacOS/chit", content: binaryContent, mode: 0o755},
	})
}

func stageFile(t *testing.T, dir string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, "downloaded")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractTarGz(t *testing.T) {
	dir := t.TempDir()
	archive := stageFile(t, dir, makeTarGz(t, map[string]string{"chit": "NEWBINARY"}))

	out, err := extractTarGz(archive, dir)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(out)
	if err != nil || string(content) != "NEWBINARY" {
		t.Fatalf("extracted content = %q, %v", content, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(out)
		if err != nil || info.Mode().Perm()&0o111 == 0 {
			// the tar header says 0644 on purpose: the extractor must set the
			// executable bit itself or the installed update will not start
			t.Errorf("mode = %v, want the executable bit set", info.Mode())
		}
	}
}

func TestExtractTarGzWithDotSlashName(t *testing.T) {
	dir := t.TempDir()
	archive := stageFile(t, dir, makeTarGz(t, map[string]string{"./chit": "NEWBINARY"}))
	out, err := extractTarGz(archive, dir)
	if err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(out); string(content) != "NEWBINARY" {
		t.Errorf("content = %q", content)
	}
}

func TestExtractTarGzWithoutTheBinary(t *testing.T) {
	dir := t.TempDir()
	archive := stageFile(t, dir, makeTarGz(t, map[string]string{"README": "hello"}))
	_, err := extractTarGz(archive, dir)
	if err == nil || !strings.Contains(err.Error(), "does not contain the chit program") {
		t.Errorf("err = %v, want the missing-binary sentence", err)
	}
}

func TestExtractTarGzOnGarbage(t *testing.T) {
	dir := t.TempDir()
	archive := stageFile(t, dir, []byte("this is not a gzip stream"))
	_, err := extractTarGz(archive, dir)
	if err == nil || !strings.Contains(err.Error(), "not the archive a CHIT release ships") {
		t.Errorf("err = %v, want the bad-archive sentence", err)
	}
}

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	archive := stageFile(t, dir, appZip(t, "MACBINARY"))

	bundle, err := extractZip(archive, dir)
	if err != nil {
		t.Fatal(err)
	}
	if bundle != filepath.Join(dir, "chit.app") {
		t.Errorf("bundle = %q", bundle)
	}
	binary := filepath.Join(bundle, "Contents", "MacOS", "chit")
	content, err := os.ReadFile(binary)
	if err != nil || string(content) != "MACBINARY" {
		t.Fatalf("binary content = %q, %v", content, err)
	}
	if plist, _ := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist")); string(plist) != "<plist/>" {
		t.Errorf("Info.plist = %q", plist)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(binary)
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("binary mode = %v, want the executable bit kept", info.Mode())
		}
		plistInfo, _ := os.Stat(filepath.Join(bundle, "Contents", "Info.plist"))
		if plistInfo.Mode().Perm()&0o111 != 0 {
			t.Errorf("Info.plist mode = %v, want no executable bit", plistInfo.Mode())
		}
	}
}

func TestExtractZipRefusesWhatItMust(t *testing.T) {
	tests := []struct {
		name    string
		entries []zipEntry
		wantErr string
	}{
		{"a path that climbs out", []zipEntry{
			{name: "chit.app/../../evil", content: "x", mode: 0o644},
		}, "not the archive a CHIT release ships"},
		{"a file outside the bundle", []zipEntry{
			{name: "chit.app/Contents/MacOS/chit", content: "x", mode: 0o755},
			{name: "loose.txt", content: "x", mode: 0o644},
		}, "not the archive a CHIT release ships"},
		{"a symlink", []zipEntry{
			{name: "chit.app/Contents/MacOS/chit", content: "/etc/passwd", mode: 0o755 | os.ModeSymlink},
		}, "not the archive a CHIT release ships"},
		{"a bundle with no binary inside", []zipEntry{
			{name: "chit.app/Contents/Info.plist", content: "<plist/>", mode: 0o644},
		}, "does not contain the chit program"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := stageFile(t, dir, makeZip(t, tt.entries))
			_, err := extractZip(archive, dir)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, tt.wantErr)
			}
			// nothing may have escaped the staging folder's parent
			if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "evil")); !os.IsNotExist(statErr) {
				t.Error("a climbing path escaped the staging folder")
			}
		})
	}
}

func TestReplaceTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chit")
	staged := filepath.Join(dir, "staged")
	mustWrite(t, target, "old version")
	mustWrite(t, staged, "new version")

	if err := replaceTarget(target, staged); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(target); string(content) != "new version" {
		t.Errorf("target = %q, want the new version", content)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Error("the .old copy was not removed (it should only survive on Windows)")
	}
}

func TestReplaceTargetClearsAStaleOld(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chit")
	staged := filepath.Join(dir, "staged")
	mustWrite(t, target, "old")
	mustWrite(t, target+".old", "stale leftover")
	mustWrite(t, staged, "new")

	if err := replaceTarget(target, staged); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(target); string(content) != "new" {
		t.Errorf("target = %q", content)
	}
}

func TestReplaceTargetPutsTheOldOneBackOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chit")
	mustWrite(t, target, "old version")

	err := replaceTarget(target, filepath.Join(dir, "does-not-exist"))
	if err == nil || !strings.Contains(err.Error(), "the old one was put back") {
		t.Fatalf("err = %v, want the fail-closed sentence", err)
	}
	if content, _ := os.ReadFile(target); string(content) != "old version" {
		t.Errorf("target = %q, want the old version restored", content)
	}
}

func TestReplaceTargetSwapsAWholeBundle(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chit.app")
	staged := filepath.Join(dir, "staged.app")
	for _, bundle := range []struct{ root, version string }{{target, "old"}, {staged, "new"}} {
		inner := filepath.Join(bundle.root, "Contents", "MacOS")
		if err := os.MkdirAll(inner, 0o755); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(inner, "chit"), bundle.version)
	}

	if err := replaceTarget(target, staged); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(target, "Contents", "MacOS", "chit"))
	if err != nil || string(content) != "new" {
		t.Errorf("bundle binary = %q, %v", content, err)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Error("the old bundle was not removed")
	}
}

func TestParseSums(t *testing.T) {
	text := "d2c1665e0b0bd6a4a04b0b26b18f4e5d0c4b09a6f0b8f37e60cf27e0e5a9f8a1  chit-linux-amd64.tar.gz\n" +
		"AB12665E0B0BD6A4A04B0B26B18F4E5D0C4B09A6F0B8F37E60CF27E0E5A9F8A1 *chit-windows-amd64.exe\n" +
		"not-a-checksum-line\n" +
		"deadbeef  too-short.bin\n"

	tests := []struct {
		name, want string
		ok         bool
	}{
		{"chit-linux-amd64.tar.gz", "d2c1665e0b0bd6a4a04b0b26b18f4e5d0c4b09a6f0b8f37e60cf27e0e5a9f8a1", true},
		// uppercase hex and the "*" binary-mode marker both normalise away
		{"chit-windows-amd64.exe", "ab12665e0b0bd6a4a04b0b26b18f4e5d0c4b09a6f0b8f37e60cf27e0e5a9f8a1", true},
		{"too-short.bin", "", false},
		{"never-mentioned.zip", "", false},
	}
	for _, tt := range tests {
		got, ok := parseSums(text, tt.name)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseSums(%q) = (%q, %v), want (%q, %v)", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}
