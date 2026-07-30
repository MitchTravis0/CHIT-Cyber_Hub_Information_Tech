package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"chit/internal/update"
)

// resetState puts the package memory back the way a fresh launch has it, so
// tests cannot leak an "already installed" flag into each other.
func resetState(t *testing.T) {
	t.Helper()
	reset := func() {
		state.mu.Lock()
		state.installing = false
		state.version = ""
		state.target = ""
		state.mu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

// The workflow packages exactly these three names, so a drift here means every
// release becomes "no download for this machine". Written as literals on
// purpose: this test is the pin between the Go and the YAML.
func TestAssetNameFor(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"windows", "amd64", "chit-windows-amd64.exe"},
		{"linux", "amd64", "chit-linux-amd64.tar.gz"},
		{"darwin", "amd64", "chit-macos-universal.zip"},
		{"darwin", "arm64", "chit-macos-universal.zip"},
		{"windows", "arm64", ""},
		{"linux", "arm64", ""},
		{"freebsd", "amd64", ""},
	}
	for _, tt := range tests {
		if got := assetNameFor(tt.goos, tt.goarch); got != tt.want {
			t.Errorf("assetNameFor(%s, %s) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestSumsAssetName(t *testing.T) {
	// the literal the workflow's checksum step writes; see .github/workflows
	if sumsAsset != "sha256sums.txt" {
		t.Errorf("sumsAsset = %q, want %q to match the release workflow", sumsAsset, "sha256sums.txt")
	}
}

func TestMaxAssetBytesIsHalfAGigabyte(t *testing.T) {
	// a CHIT release is ~15 MB; the cap only exists so a wrong answer cannot
	// fill the disk, and it must stay far above any real release
	if maxAssetBytes != 536870912 {
		t.Errorf("maxAssetBytes = %d, want 536870912", maxAssetBytes)
	}
}

func TestTargetFor(t *testing.T) {
	tests := []struct {
		name       string
		goos, exe  string
		wantTarget string
		wantBundle bool
		wantNote   string
	}{
		{"linux binary", "linux", "/home/tech/tools/chit", "/home/tech/tools/chit", false, ""},
		{"windows exe", "windows", `C:\Tools\chit.exe`, `C:\Tools\chit.exe`, false, ""},
		{"mac bundle", "darwin", "/Applications/chit.app/Contents/MacOS/chit",
			"/Applications/chit.app", true, ""},
		{"mac bundle in a folder with spaces", "darwin",
			"/Users/a tech/My Tools/chit.app/Contents/MacOS/chit",
			"/Users/a tech/My Tools/chit.app", true, ""},
		{"mac bare binary outside a bundle", "darwin", "/Users/tech/chit", "", false, "not running from the chit.app bundle"},
		{"mac binary under Contents but no .app", "darwin", "/Users/tech/Contents/MacOS/chit", "", false, "not running from the chit.app bundle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, bundle, note := targetFor(tt.goos, tt.exe)
			if target != tt.wantTarget || bundle != tt.wantBundle {
				t.Errorf("targetFor(%s, %s) = (%q, %v), want (%q, %v)",
					tt.goos, tt.exe, target, bundle, tt.wantTarget, tt.wantBundle)
			}
			if tt.wantNote == "" && note != "" {
				t.Errorf("note = %q, want none", note)
			}
			if tt.wantNote != "" && !strings.Contains(note, tt.wantNote) {
				t.Errorf("note = %q, want it to contain %q", note, tt.wantNote)
			}
		})
	}
}

func TestPlanFor(t *testing.T) {
	assets := []update.Asset{
		{Name: "chit-windows-amd64.exe", URL: "https://example.com/w", Size: 100},
		{Name: "chit-macos-universal.zip", URL: "https://example.com/m", Size: 200},
		{Name: "chit-linux-amd64.tar.gz", URL: "https://example.com/l", Size: 300},
		{Name: "sha256sums.txt", URL: "https://example.com/s", Size: 10},
	}

	t.Run("linux picks the tarball and the sums", func(t *testing.T) {
		p, note := planFor(assets, "linux", "amd64", "/opt/chit")
		if note != "" {
			t.Fatalf("note = %q, want none", note)
		}
		if p.asset.URL != "https://example.com/l" || p.asset.Size != 300 {
			t.Errorf("asset = %+v, want the linux tarball", p.asset)
		}
		if p.sums.URL != "https://example.com/s" {
			t.Errorf("sums = %+v, want the checksum file", p.sums)
		}
		if p.target != "/opt/chit" || p.bundle {
			t.Errorf("target = %q bundle = %v, want the executable itself", p.target, p.bundle)
		}
	})

	t.Run("unsupported machine", func(t *testing.T) {
		_, note := planFor(assets, "linux", "arm64", "/opt/chit")
		if !strings.Contains(note, "linux/arm64") {
			t.Errorf("note = %q, want it to name the machine", note)
		}
	})

	t.Run("release missing this platform's file", func(t *testing.T) {
		_, note := planFor(assets[3:], "linux", "amd64", "/opt/chit")
		if !strings.Contains(note, "chit-linux-amd64.tar.gz") {
			t.Errorf("note = %q, want it to name the missing file", note)
		}
	})

	t.Run("release missing the checksum file", func(t *testing.T) {
		_, note := planFor(assets[:3], "linux", "amd64", "/opt/chit")
		if !strings.Contains(note, "no checksum file") {
			t.Errorf("note = %q, want the checksum sentence", note)
		}
	})

	t.Run("mac outside a bundle", func(t *testing.T) {
		_, note := planFor(assets, "darwin", "amd64", "/Users/tech/chit")
		if !strings.Contains(note, "chit.app") {
			t.Errorf("note = %q, want the bundle sentence", note)
		}
	})
}

func TestWritableNote(t *testing.T) {
	dir := t.TempDir()
	if note := writableNote(dir); note != "" {
		t.Errorf("writableNote(%q) = %q, want none for a writable folder", dir, note)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("the probe left %v behind", entries)
	}

	if runtime.GOOS == "windows" {
		t.Skip("a read-only folder does not refuse new files on Windows, so this half cannot run there")
	}
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	note := writableNote(locked)
	if !strings.Contains(note, locked) || !strings.Contains(note, "by hand") {
		t.Errorf("note = %q, want it to name the folder and the manual way out", note)
	}
}

func TestRelaunchArgs(t *testing.T) {
	tests := []struct {
		goos, target string
		want         []string
	}{
		{"linux", "/opt/chit", []string{"/opt/chit"}},
		{"windows", `C:\Tools\chit.exe`, []string{`C:\Tools\chit.exe`}},
		{"darwin", "/Applications/chit.app", []string{"open", "-n", "/Applications/chit.app"}},
	}
	for _, tt := range tests {
		got := relaunchArgs(tt.goos, tt.target)
		if len(got) != len(tt.want) {
			t.Errorf("relaunchArgs(%s) = %v, want %v", tt.goos, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("relaunchArgs(%s) = %v, want %v", tt.goos, got, tt.want)
				break
			}
		}
	}
}

func TestRelaunchWithNothingInstalled(t *testing.T) {
	resetState(t)
	err := Relaunch()
	if err == nil || !strings.Contains(err.Error(), "nothing to restart") {
		t.Errorf("err = %v, want the nothing-installed sentence", err)
	}
}

func TestCleanupLeftovers(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chit")
	mustWrite(t, target, "current")
	mustWrite(t, target+".old", "previous")
	stage := filepath.Join(dir, ".chit-update-stranded")
	if err := os.Mkdir(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(stage, "half-download"), "junk")
	mustWrite(t, filepath.Join(dir, "keep.txt"), "keep")

	cleanupLeftovers(target)

	for _, gone := range []string{target + ".old", stage} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s still exists after cleanup", gone)
		}
	}
	for _, kept := range []string{target, filepath.Join(dir, "keep.txt")} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was removed by cleanup: %v", kept, err)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
