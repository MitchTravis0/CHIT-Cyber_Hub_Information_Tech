// Package selfupdate downloads a newer CHIT release and swaps it into place.
// It is the deliberately separate, larger half of the update story:
// internal/update only compares versions and keeps its promise never to write
// to disk or run anything, while this package may do both, on a button press
// and never on its own. Added in Phase 10 at the repo owner's request; the
// design is written up in SPECS/PHASE10.md.
//
// The safety rules, each of them tested:
//   - The install job takes no input from the page. It asks the GitHub API
//     itself, so the only download it can ever perform is the one the API named.
//   - The download must match the SHA-256 the release published in
//     sha256sums.txt, or nothing is replaced. A release without that file is
//     reported as not installable rather than trusted.
//   - The swap is rename-based and fails closed: the running version is moved
//     aside first and moved back if anything goes wrong, so a failed update
//     always leaves a working CHIT.
//   - It never asks for elevation. A folder it cannot write to is a sentence
//     telling the tech to update by hand, not a prompt.
package selfupdate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"chit/internal/core"
	"chit/internal/update"
)

// JobKind names the install job in job events.
const JobKind = "update-install"

const (
	// sumsAsset is the checksum file the release workflow publishes beside the
	// three platform artifacts. The updater refuses a release without it.
	sumsAsset = "sha256sums.txt"
	// oldSuffix is where the running version is renamed while the new one takes
	// its place. Windows cannot delete a running exe even after renaming it, so
	// the leftover is removed on the next launch by CleanupLeftovers.
	oldSuffix = ".old"
	// stagePrefix names the temporary staging folder the download lands in. It
	// sits next to the target so the final rename cannot cross filesystems, and
	// CleanupLeftovers removes any that a crash leaves behind.
	stagePrefix = ".chit-update-"
)

// Status is what the settings page gets from a check: the plain comparison
// plus whether this build can install the newer release itself.
type Status struct {
	update.Result
	// CanInstall is true only when Newer is true, a download exists for this
	// machine, the release carries a checksum file, and the folder holding the
	// executable is writable without elevation.
	CanInstall bool `json:"canInstall"`
	// InstallNote says why CanInstall is false when Newer is true, in one
	// sentence that also says what to do instead. Empty otherwise.
	InstallNote string `json:"installNote"`
	// AssetName and AssetSize describe the download, for the page to show
	// before the tech commits to it.
	AssetName string `json:"assetName"`
	AssetSize int64  `json:"assetSize"`
}

// state is the one piece of memory between bound calls: whether an install is
// running, and what was installed this session so Restart knows what to start.
// Package-level because app.go is frozen and App cannot grow a field.
var state struct {
	mu         sync.Mutex
	installing bool
	version    string
	target     string
}

// Check asks GitHub for the latest release and works out whether this build
// can install it by itself.
func Check(ctx context.Context, current string) (Status, error) {
	res, err := update.Check(ctx, current)
	if err != nil {
		return Status{}, err
	}
	exe, exeErr := executable()
	return statusFor(res, exe, exeErr, runtime.GOOS, runtime.GOARCH), nil
}

// statusFor decides what the settings page may offer for a check result:
// the Install button, or one sentence saying why not.
func statusFor(res update.Result, exe string, exeErr error, goos, goarch string) Status {
	st := Status{Result: res}
	if !res.Newer {
		return st
	}

	state.mu.Lock()
	installed := state.version
	state.mu.Unlock()
	if installed != "" {
		st.InstallNote = fmt.Sprintf(
			"Version %s is already installed. Restart CHIT to start using it.", installed)
		return st
	}

	if exeErr != nil {
		st.InstallNote = "CHIT could not work out where its own executable is, so it cannot replace it. Download the new version from the releases page instead."
		return st
	}
	p, note := planFor(res.Assets, goos, goarch, exe)
	if note != "" {
		st.InstallNote = note
		return st
	}
	if note := writableNote(filepath.Dir(p.target)); note != "" {
		st.InstallNote = note
		return st
	}
	st.CanInstall = true
	st.AssetName = p.asset.Name
	st.AssetSize = p.asset.Size
	return st
}

// plan is everything the install needs once the release is known: which asset,
// which checksum file, and what on disk gets replaced.
type plan struct {
	asset  update.Asset
	sums   update.Asset
	target string
	bundle bool
}

// planFor picks this machine's download out of the release and resolves what
// it replaces. A non-empty note means the install cannot happen and says why.
func planFor(assets []update.Asset, goos, goarch, exe string) (plan, string) {
	name := assetNameFor(goos, goarch)
	if name == "" {
		return plan{}, fmt.Sprintf(
			"CHIT releases carry no download for this machine (%s/%s), so it cannot update itself here. The releases page may still have something that runs on it.",
			goos, goarch)
	}
	var p plan
	for _, a := range assets {
		switch a.Name {
		case name:
			p.asset = a
		case sumsAsset:
			p.sums = a
		}
	}
	if p.asset.URL == "" {
		return plan{}, fmt.Sprintf(
			"This release has no download named %s, so CHIT cannot install it for you. Use the download page instead.", name)
	}
	if p.sums.URL == "" {
		return plan{}, "This release has no checksum file, so CHIT will not install it automatically. Use the download page instead."
	}
	target, bundle, note := targetFor(goos, exe)
	if note != "" {
		return plan{}, note
	}
	p.target, p.bundle = target, bundle
	return p, ""
}

// assetNameFor is the exact artifact name the release workflow publishes for a
// platform, or empty when there is none (the workflow only builds these three).
func assetNameFor(goos, goarch string) string {
	switch {
	case goos == "windows" && goarch == "amd64":
		return "chit-windows-amd64.exe"
	case goos == "darwin":
		// one universal binary covers amd64 and arm64
		return "chit-macos-universal.zip"
	case goos == "linux" && goarch == "amd64":
		return "chit-linux-amd64.tar.gz"
	}
	return ""
}

// targetFor resolves what the install replaces: the executable itself, except
// on macOS, where the asset is a whole chit.app and swapping anything less
// would leave the bundle's Info.plist claiming the old version.
func targetFor(goos, exe string) (target string, bundle bool, note string) {
	if goos != "darwin" {
		return exe, false, ""
	}
	// .../chit.app/Contents/MacOS/chit -> .../chit.app. macOS paths always use
	// a forward slash, so this is checkable on any development machine.
	idx := strings.LastIndex(exe, "/Contents/MacOS/")
	if idx < 0 || !strings.HasSuffix(exe[:idx], ".app") {
		return "", false, "CHIT is not running from the chit.app bundle it ships as, so it cannot replace itself. Download the new version and replace it by hand."
	}
	return exe[:idx], true, ""
}

// writableNote probes whether dir can be written without elevation, which is
// the no-admin promise applied to updating: if it cannot, the answer is a
// sentence, never a prompt.
func writableNote(dir string) string {
	probe, err := os.CreateTemp(dir, stagePrefix)
	if err != nil {
		return fmt.Sprintf(
			"CHIT cannot write to its own folder (%s), so it cannot replace itself. Download the new file and replace it by hand, or run CHIT from a folder you can write to.", dir)
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return ""
}

// executable is os.Executable with symlinks resolved, so the file that gets
// replaced is the real one rather than a link pointing at it.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// StartInstall begins the download-verify-swap job and returns its id. It
// takes no parameters on purpose: the job re-checks the release itself, so the
// page cannot hand the backend a URL.
func StartInstall(jobs *core.JobManager, current string) (string, error) {
	exe, err := executable()
	if err != nil {
		return "", core.Errorf(core.CodeInternal,
			"CHIT could not work out where its own executable is, so it cannot replace it.")
	}
	return startInstallAt(jobs, current, exe, runtime.GOOS, runtime.GOARCH)
}

// startInstallAt is StartInstall with the executable path and platform
// injectable, so a test can run the whole job against a file that is not the
// test binary, on any CI runner: a closure that captured runtime.GOOS would
// make the job demand a different release asset on every platform.
func startInstallAt(jobs *core.JobManager, current, exe, goos, goarch string) (string, error) {
	state.mu.Lock()
	if state.installing {
		state.mu.Unlock()
		return "", core.Errorf(core.CodeInvalidInput,
			"An update is already being installed. Wait for it to finish.")
	}
	if state.version != "" {
		installed := state.version
		state.mu.Unlock()
		return "", core.Errorf(core.CodeInvalidInput,
			"Version %s is already installed. Restart CHIT to start using it.", installed)
	}
	state.installing = true
	state.mu.Unlock()

	return jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		defer clearInstalling()
		sink := Sink{Progress: jc.Progress, Summary: jc.SetSummary}
		version, target, err := runInstall(jc.Ctx(), sink, current, exe, goos, goarch)
		if err != nil {
			return err
		}
		state.mu.Lock()
		state.version, state.target = version, target
		state.mu.Unlock()
		return nil
	}), nil
}

func clearInstalling() {
	state.mu.Lock()
	state.installing = false
	state.mu.Unlock()
}

// Relaunch starts the version the install put on disk. It uses the target
// recorded at install time: after the swap, os.Executable can point at the
// deleted ".old" file on Linux, so it must not be consulted here.
func Relaunch() error {
	state.mu.Lock()
	target := state.target
	state.mu.Unlock()
	if target == "" {
		return core.Errorf(core.CodeInvalidInput,
			"No update has been installed, so there is nothing to restart into.")
	}
	argv := relaunchArgs(runtime.GOOS, target)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = filepath.Dir(target)
	if err := cmd.Start(); err != nil {
		return core.Errorf(core.CodeInternal,
			"The new version would not start (%s). Close CHIT and open it again yourself; the update is already in place.", target)
	}
	return nil
}

// relaunchArgs is how the new version gets started per OS. On macOS the target
// is the .app bundle and "open -n" launches it the way Finder would; elsewhere
// the target is the executable itself.
func relaunchArgs(goos, target string) []string {
	if goos == "darwin" {
		return []string{"open", "-n", target}
	}
	return []string{target}
}

// CleanupLeftovers removes what an update could not delete while the previous
// version was still running: the renamed-aside old binary, and any staging
// folder a crash stranded. Called once at startup. Every error is ignored on
// purpose, because the worst case is trying again on the next launch.
func CleanupLeftovers() {
	exe, err := executable()
	if err != nil {
		return
	}
	target, _, note := targetFor(runtime.GOOS, exe)
	if note != "" {
		target = exe
	}
	cleanupLeftovers(target)
}

func cleanupLeftovers(target string) {
	os.RemoveAll(target + oldSuffix)
	dir := filepath.Dir(target)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagePrefix) {
			os.RemoveAll(filepath.Join(dir, entry.Name()))
		}
	}
}
