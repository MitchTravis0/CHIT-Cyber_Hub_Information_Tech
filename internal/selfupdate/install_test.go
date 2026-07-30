package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
	"chit/internal/update"
)

type fakeAsset struct {
	name    string
	content []byte
	// sizeOverride makes the API lie about the size; 0 means tell the truth.
	sizeOverride int64
}

// sumsFor writes the checksum file the release workflow would: real SHA-256s
// in sha256sum's own output format.
func sumsFor(assets ...fakeAsset) []byte {
	out := ""
	for _, a := range assets {
		digest := sha256.Sum256(a.content)
		out += hex.EncodeToString(digest[:]) + "  " + a.name + "\n"
	}
	return []byte(out)
}

// serveRelease stands up a fake GitHub API plus download host for one test and
// points update.BaseURL at it.
func serveRelease(t *testing.T, tag string, assets []fakeAsset) {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	type apiAsset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	}
	list := []apiAsset{}
	for _, a := range assets {
		size := int64(len(a.content))
		if a.sizeOverride != 0 {
			size = a.sizeOverride
		}
		list = append(list, apiAsset{a.name, server.URL + "/a/" + a.name, size})
		content := a.content
		mux.HandleFunc("/a/"+a.name, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(content)
		})
	}
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		payload, err := json.Marshal(map[string]any{"tag_name": tag, "assets": list})
		if err != nil {
			t.Error(err)
		}
		_, _ = w.Write(payload)
	})

	previous := update.BaseURL
	update.BaseURL = server.URL + "/release"
	t.Cleanup(func() { update.BaseURL = previous })
}

type progressCall struct {
	done, total int
	msg         string
}

type recorder struct {
	progress []progressCall
	summary  map[string]any
}

func (r *recorder) sink() Sink {
	return Sink{
		Progress: func(done, total int, msg string) {
			r.progress = append(r.progress, progressCall{done, total, msg})
		},
		Summary: func(s map[string]any) { r.summary = s },
	}
}

// noStageLeftovers asserts every staging folder is gone whatever the outcome.
func noStageLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".chit-update-") {
			t.Errorf("staging leftover %s was not cleaned up", entry.Name())
		}
	}
}

func TestRunInstallReplacesTheLinuxBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "the old binary")

	tarball := fakeAsset{name: "chit-linux-amd64.tar.gz", content: makeTarGz(t, map[string]string{"chit": "the new binary"})}
	serveRelease(t, "v9.9.9", []fakeAsset{tarball, {name: "sha256sums.txt", content: sumsFor(tarball)}})

	rec := &recorder{}
	version, target, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if version != "9.9.9" || target != exe {
		t.Errorf("version = %q target = %q", version, target)
	}
	content, err := os.ReadFile(exe)
	if err != nil || string(content) != "the new binary" {
		t.Fatalf("target = %q, %v; want the new binary in place", content, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(exe)
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("installed binary mode = %v, want executable", info.Mode())
		}
	}
	noStageLeftovers(t, dir)

	if rec.summary["installed"] != "9.9.9" {
		t.Errorf("summary = %v, want installed 9.9.9", rec.summary)
	}
	note, _ := rec.summary["note"].(string)
	if !strings.Contains(note, "restart now") {
		t.Errorf("summary note = %q, want it to offer the restart", note)
	}
	size := len(tarball.content)
	sawFullDownload := false
	for _, p := range rec.progress {
		if p.done == size && p.total == size && strings.Contains(p.msg, "chit-linux-amd64.tar.gz") {
			sawFullDownload = true
		}
	}
	if !sawFullDownload {
		t.Errorf("progress never reported the full download; got %v", rec.progress)
	}
}

func TestRunInstallReplacesTheMacBundle(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "chit.app", "Contents", "MacOS")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(inner, "chit")
	mustWrite(t, exe, "the old mac binary")

	zipAsset := fakeAsset{name: "chit-macos-universal.zip", content: appZip(t, "the new mac binary")}
	serveRelease(t, "v9.9.9", []fakeAsset{zipAsset, {name: "sha256sums.txt", content: sumsFor(zipAsset)}})

	rec := &recorder{}
	version, target, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if version != "9.9.9" || target != filepath.Join(dir, "chit.app") {
		t.Errorf("version = %q target = %q, want the bundle root", version, target)
	}
	content, err := os.ReadFile(exe)
	if err != nil || string(content) != "the new mac binary" {
		t.Fatalf("bundle binary = %q, %v", content, err)
	}
	if plist, _ := os.ReadFile(filepath.Join(dir, "chit.app", "Contents", "Info.plist")); string(plist) != "<plist/>" {
		t.Errorf("Info.plist = %q, want the new bundle's", plist)
	}
	noStageLeftovers(t, dir)
}

func TestRunInstallReplacesTheWindowsExe(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit.exe")
	mustWrite(t, exe, "the old exe")

	exeAsset := fakeAsset{name: "chit-windows-amd64.exe", content: []byte("the new exe")}
	serveRelease(t, "v9.9.9", []fakeAsset{exeAsset, {name: "sha256sums.txt", content: sumsFor(exeAsset)}})

	rec := &recorder{}
	if _, _, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "windows", "amd64"); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(exe); string(content) != "the new exe" {
		t.Errorf("target = %q", content)
	}
	noStageLeftovers(t, dir)
}

func TestRunInstallRefusesAChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "the old binary")

	tarball := fakeAsset{name: "chit-linux-amd64.tar.gz", content: makeTarGz(t, map[string]string{"chit": "evil"})}
	wrong := fakeAsset{name: "sha256sums.txt",
		content: []byte(strings.Repeat("ab", 32) + "  chit-linux-amd64.tar.gz\n")}
	serveRelease(t, "v9.9.9", []fakeAsset{tarball, wrong})

	rec := &recorder{}
	_, _, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "does not match the checksum") {
		t.Fatalf("err = %v, want the checksum sentence", err)
	}
	if content, _ := os.ReadFile(exe); string(content) != "the old binary" {
		t.Errorf("target = %q, want it untouched", content)
	}
	noStageLeftovers(t, dir)
}

func TestRunInstallRefusesAShortDownload(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "the old binary")

	tarball := fakeAsset{
		name:         "chit-linux-amd64.tar.gz",
		content:      makeTarGz(t, map[string]string{"chit": "new"}),
		sizeOverride: 5000000,
	}
	serveRelease(t, "v9.9.9", []fakeAsset{tarball, {name: "sha256sums.txt", content: sumsFor(tarball)}})

	rec := &recorder{}
	_, _, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "ended at") {
		t.Fatalf("err = %v, want the short-download sentence", err)
	}
	if content, _ := os.ReadFile(exe); string(content) != "the old binary" {
		t.Errorf("target = %q, want it untouched", content)
	}
	noStageLeftovers(t, dir)
}

// The server sending more than the API declared is as suspect as sending
// less; the reader stops at one byte past the declared size and refuses.
func TestRunInstallRefusesAnOverlongDownload(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "the old binary")

	tarball := fakeAsset{
		name:         "chit-linux-amd64.tar.gz",
		content:      makeTarGz(t, map[string]string{"chit": "new"}),
		sizeOverride: 10,
	}
	serveRelease(t, "v9.9.9", []fakeAsset{tarball, {name: "sha256sums.txt", content: sumsFor(tarball)}})

	rec := &recorder{}
	_, _, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "ended at 11 bytes where the release says it is 10") {
		t.Fatalf("err = %v, want the exact over-length sentence", err)
	}
	if content, _ := os.ReadFile(exe); string(content) != "the old binary" {
		t.Errorf("target = %q, want it untouched", content)
	}
}

func TestRunInstallRefusesAnAbsurdSize(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "the old binary")

	tarball := fakeAsset{
		name:         "chit-linux-amd64.tar.gz",
		content:      []byte("tiny"),
		sizeOverride: 600 << 20,
	}
	serveRelease(t, "v9.9.9", []fakeAsset{tarball, {name: "sha256sums.txt", content: sumsFor(tarball)}})

	rec := &recorder{}
	_, _, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "not a size a CHIT release can be") {
		t.Fatalf("err = %v, want the size-cap sentence", err)
	}
}

func TestRunInstallWhenAlreadyOnTheLatest(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "current")
	serveRelease(t, "v1.0.0", []fakeAsset{})

	rec := &recorder{}
	_, _, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "already running the latest release") {
		t.Fatalf("err = %v, want the already-latest sentence", err)
	}
}

func TestRunInstallWithoutAChecksumFile(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "current")

	tarball := fakeAsset{name: "chit-linux-amd64.tar.gz", content: makeTarGz(t, map[string]string{"chit": "new"})}
	serveRelease(t, "v9.9.9", []fakeAsset{tarball})

	rec := &recorder{}
	_, _, err := runInstall(context.Background(), rec.sink(), "1.0.0", exe, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "no checksum file") {
		t.Fatalf("err = %v, want the no-checksum sentence", err)
	}
	if content, _ := os.ReadFile(exe); string(content) != "current" {
		t.Errorf("target = %q, want it untouched", content)
	}
}

// A cancelled download must leave the old binary in place and nothing else on
// disk. The fake serves the checksum file normally and then never finishes the
// binary, which is what a dead Wi-Fi link looks like.
func TestRunInstallCancelledMidDownload(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "the old binary")

	big := make([]byte, 1<<20)
	tarball := fakeAsset{name: "chit-linux-amd64.tar.gz", content: big}
	sums := fakeAsset{name: "sha256sums.txt", content: sumsFor(tarball)}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		payload := fmt.Sprintf(`{"tag_name": "v9.9.9", "assets": [
			{"name": %q, "browser_download_url": %q, "size": %d},
			{"name": %q, "browser_download_url": %q, "size": %d}]}`,
			tarball.name, server.URL+"/a/bin", len(big),
			sums.name, server.URL+"/a/sums", len(sums.content))
		_, _ = w.Write([]byte(payload))
	})
	mux.HandleFunc("/a/sums", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(sums.content)
	})
	mux.HandleFunc("/a/bin", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(big[:64<<10])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	})
	previous := update.BaseURL
	update.BaseURL = server.URL + "/release"
	t.Cleanup(func() { update.BaseURL = previous })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := Sink{
		Progress: func(done, total int, msg string) {
			if done > 0 && done < total {
				cancel()
			}
		},
		Summary: func(map[string]any) {},
	}
	_, _, err := runInstall(ctx, sink, "1.0.0", exe, "linux", "amd64")
	if err == nil {
		t.Fatal("want an error after cancelling")
	}
	if content, _ := os.ReadFile(exe); string(content) != "the old binary" {
		t.Errorf("target = %q, want it untouched", content)
	}
	noStageLeftovers(t, dir)
}

// A cancel that lands after the download finished but before the swap must
// still leave the old binary in place: the page tells the tech nothing was
// changed, and that has to be true.
func TestRunInstallCancelledBeforeTheSwap(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "the old binary")

	tarball := fakeAsset{name: "chit-linux-amd64.tar.gz", content: makeTarGz(t, map[string]string{"chit": "new"})}
	serveRelease(t, "v9.9.9", []fakeAsset{tarball, {name: "sha256sums.txt", content: sumsFor(tarball)}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := Sink{
		Progress: func(done, total int, msg string) {
			if msg == "Verifying the download" {
				cancel()
			}
		},
		Summary: func(map[string]any) {},
	}
	_, _, err := runInstall(ctx, sink, "1.0.0", exe, "linux", "amd64")
	if err == nil {
		t.Fatal("want an error after cancelling")
	}
	if content, _ := os.ReadFile(exe); string(content) != "the old binary" {
		t.Errorf("target = %q, want it untouched after a late cancel", content)
	}
	noStageLeftovers(t, dir)
}

// The whole job through the real JobManager: startInstallAt wires runInstall's
// outcome into the package state that Restart and a second Install read.
func TestStartInstallRunsTheJobAndRecordsTheInstall(t *testing.T) {
	resetState(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "the old binary")

	tarball := fakeAsset{name: "chit-linux-amd64.tar.gz", content: makeTarGz(t, map[string]string{"chit": "the new binary"})}
	serveRelease(t, "v9.9.9", []fakeAsset{tarball, {name: "sha256sums.txt", content: sumsFor(tarball)}})

	jobs := core.NewJobManager()
	jobID, err := startInstallAt(jobs, "1.0.0", exe)
	if err != nil {
		t.Fatal(err)
	}
	if jobID == "" {
		t.Fatal("no job id")
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		state.mu.Lock()
		version := state.version
		state.mu.Unlock()
		if version != "" && jobs.Running() == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the install job never recorded a version")
		}
		time.Sleep(10 * time.Millisecond)
	}

	state.mu.Lock()
	version, target, installing := state.version, state.target, state.installing
	state.mu.Unlock()
	if version != "9.9.9" || target != exe || installing {
		t.Errorf("state = (%q, %q, %v), want (9.9.9, %q, false)", version, target, installing, exe)
	}
	if content, _ := os.ReadFile(exe); string(content) != "the new binary" {
		t.Errorf("target = %q", content)
	}

	// pressing Install again now must refuse and point at the restart
	_, err = startInstallAt(jobs, "1.0.0", exe)
	if err == nil || !strings.Contains(err.Error(), "Restart CHIT") {
		t.Errorf("second install err = %v, want the restart sentence", err)
	}
}

func TestStartInstallWhileOneIsRunning(t *testing.T) {
	resetState(t)
	state.mu.Lock()
	state.installing = true
	state.mu.Unlock()

	_, err := startInstallAt(core.NewJobManager(), "1.0.0", "/nowhere/chit")
	if err == nil || !strings.Contains(err.Error(), "already being installed") {
		t.Errorf("err = %v, want the already-installing sentence", err)
	}
}

// Check wires feasibility onto the plain result: whenever the release is newer
// it either offers the install or says in one sentence why not.
func TestCheckWiresFeasibility(t *testing.T) {
	resetState(t)
	assets := []fakeAsset{
		{name: "chit-windows-amd64.exe", content: []byte("w")},
		{name: "chit-macos-universal.zip", content: []byte("m")},
		{name: "chit-linux-amd64.tar.gz", content: []byte("l")},
		{name: "sha256sums.txt", content: []byte("s")},
	}
	serveRelease(t, "v9.9.9", assets)

	got, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Newer {
		t.Fatal("Newer = false")
	}
	// On dev and CI machines the test binary is in a writable folder, so the
	// only reason install can be off is the per-OS plan (macOS: the test
	// binary is not a .app bundle). Either way the page always has something
	// to show: the button, or the sentence.
	if got.CanInstall == (got.InstallNote != "") {
		t.Errorf("CanInstall = %v with InstallNote = %q; exactly one must be set", got.CanInstall, got.InstallNote)
	}
	if got.CanInstall && (got.AssetName == "" || got.AssetSize <= 0) {
		t.Errorf("CanInstall with no asset details: %q %d", got.AssetName, got.AssetSize)
	}
}

// statusFor holds the whole may-we-offer-the-button decision, including the
// branch Check itself cannot exercise in a test: a folder the tech cannot
// write to must turn into a sentence, never an elevation prompt.
func TestStatusForFeasibility(t *testing.T) {
	resetState(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "chit")
	mustWrite(t, exe, "current")

	assets := []update.Asset{
		{Name: "chit-linux-amd64.tar.gz", URL: "https://example.com/l", Size: 12345},
		{Name: "sha256sums.txt", URL: "https://example.com/s", Size: 100},
	}
	newer := update.Result{Newer: true, Latest: "9.9.9", Assets: assets}

	t.Run("writable folder offers the install", func(t *testing.T) {
		st := statusFor(newer, exe, nil, "linux", "amd64")
		if !st.CanInstall || st.InstallNote != "" {
			t.Fatalf("Status = %+v, want CanInstall with no note", st)
		}
		if st.AssetName != "chit-linux-amd64.tar.gz" || st.AssetSize != 12345 {
			t.Errorf("asset = %q %d, want the tarball's name and size", st.AssetName, st.AssetSize)
		}
	})

	t.Run("not newer offers nothing and says nothing", func(t *testing.T) {
		st := statusFor(update.Result{Newer: false}, exe, nil, "linux", "amd64")
		if st.CanInstall || st.InstallNote != "" {
			t.Errorf("Status = %+v, want neither button nor note", st)
		}
	})

	t.Run("unreadable executable path", func(t *testing.T) {
		st := statusFor(newer, "", os.ErrNotExist, "linux", "amd64")
		if st.CanInstall || !strings.Contains(st.InstallNote, "could not work out where") {
			t.Errorf("Status = %+v, want the lost-executable sentence", st)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("unwritable folder turns into a sentence", func(t *testing.T) {
			locked := filepath.Join(dir, "locked")
			if err := os.Mkdir(locked, 0o555); err != nil {
				t.Fatal(err)
			}
			st := statusFor(newer, filepath.Join(locked, "chit"), nil, "linux", "amd64")
			if st.CanInstall || !strings.Contains(st.InstallNote, "cannot write to its own folder") {
				t.Errorf("Status = %+v, want the unwritable sentence", st)
			}
		})
	}
}

func TestCheckAfterAnInstallPointsAtTheRestart(t *testing.T) {
	resetState(t)
	state.mu.Lock()
	state.version = "9.9.9"
	state.target = "/opt/chit"
	state.mu.Unlock()

	serveRelease(t, "v9.9.9", []fakeAsset{})
	got, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.CanInstall {
		t.Error("CanInstall = true with an install already waiting for a restart")
	}
	if !strings.Contains(got.InstallNote, "already installed") || !strings.Contains(got.InstallNote, "Restart") {
		t.Errorf("InstallNote = %q, want the restart sentence", got.InstallNote)
	}
}
