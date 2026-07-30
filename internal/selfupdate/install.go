package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"chit/internal/core"
	"chit/internal/update"
)

const (
	// maxAssetBytes caps a download. A CHIT release is around 15 MB; this
	// exists so a wrong or hostile answer cannot fill the disk.
	maxAssetBytes = 512 << 20
	// maxSumsBytes caps the checksum file, which is a few hundred bytes.
	maxSumsBytes = 64 << 10
	// copyChunk is the read size while downloading, small enough that progress
	// and cancellation both stay responsive.
	copyChunk = 128 << 10
)

// Sink is the install job's outputs as callbacks, so a test can hand in a
// recorder: a *core.JobContext's events go out through the Wails runtime and
// cannot be read back in a test.
type Sink struct {
	Progress func(done, total int, msg string)
	Summary  func(map[string]any)
}

// runInstall is the whole install: re-check the release, download this
// machine's asset, verify it against the release's checksum file, unpack it,
// and swap it into place. It returns the installed version and the path that
// was replaced. On any error nothing on disk has changed except a staging
// folder that is removed on the way out.
func runInstall(ctx context.Context, sink Sink, current, exe, goos, goarch string) (string, string, error) {
	sink.Progress(0, 0, "Checking the latest release")
	res, err := update.Check(ctx, current)
	if err != nil {
		return "", "", err
	}
	if !res.Newer {
		return "", "", core.Errorf(core.CodeInvalidInput,
			"You are already running the latest release (%s), so there is nothing to install.", current)
	}
	p, note := planFor(res.Assets, goos, goarch, exe)
	if note != "" {
		return "", "", core.Errorf(core.CodeInvalidInput, "%s", note)
	}
	if p.asset.Size <= 0 || p.asset.Size > maxAssetBytes {
		return "", "", core.Errorf(core.CodeNetwork,
			"github.com describes the download as %d bytes, which is not a size a CHIT release can be. Refusing it. Use the download page instead.", p.asset.Size)
	}
	if note := writableNote(filepath.Dir(p.target)); note != "" {
		return "", "", core.Errorf(core.CodePermission, "%s", note)
	}

	want, err := fetchExpectedSum(ctx, p.sums.URL, p.asset.Name)
	if err != nil {
		return "", "", err
	}

	stage, err := os.MkdirTemp(filepath.Dir(p.target), stagePrefix)
	if err != nil {
		return "", "", core.Errorf(core.CodePermission,
			"CHIT could not create a working folder next to itself (%s), so it cannot install the update. Download it by hand instead.", filepath.Dir(p.target))
	}
	defer os.RemoveAll(stage)

	downloaded, got, err := downloadTo(ctx, p.asset, stage, sink)
	if err != nil {
		return "", "", err
	}

	sink.Progress(int(p.asset.Size), int(p.asset.Size), "Verifying the download")
	if got != want {
		return "", "", core.Errorf(core.CodeNetwork,
			"The downloaded file does not match the checksum the release published, so it was thrown away and nothing was replaced. Try again, and if it keeps happening use the download page instead.")
	}

	newPath, err := unpack(downloaded, p.asset.Name, stage)
	if err != nil {
		return "", "", err
	}

	// Last stop before anything on disk changes: a cancel that lands during
	// the download or the verify must mean "nothing was replaced", so the page
	// can say exactly that.
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	sink.Progress(int(p.asset.Size), int(p.asset.Size), "Installing")
	if err := replaceTarget(p.target, newPath); err != nil {
		return "", "", err
	}

	sink.Summary(map[string]any{
		"installed": res.Latest,
		"note": fmt.Sprintf(
			"Version %s is installed. It takes over the next time CHIT starts: restart now, or keep working and close CHIT when you are ready.", res.Latest),
	})
	return res.Latest, p.target, nil
}

// unpack turns the downloaded asset into the thing that replaces the target:
// the exe is already it, the tar.gz holds the Linux binary, the zip holds the
// whole chit.app bundle.
func unpack(downloaded, assetName, stage string) (string, error) {
	switch {
	case strings.HasSuffix(assetName, ".exe"):
		return downloaded, nil
	case strings.HasSuffix(assetName, ".tar.gz"):
		return extractTarGz(downloaded, stage)
	case strings.HasSuffix(assetName, ".zip"):
		return extractZip(downloaded, stage)
	}
	// unreachable while assetNameFor only names the three above, but the
	// compiler cannot know that
	return "", core.Errorf(core.CodeInternal,
		"CHIT does not know how to unpack %s. This is a fault in CHIT itself.", assetName)
}

// downloadTo streams the asset into the staging folder, reporting progress in
// bytes and hashing as it goes. The byte count must land exactly on the size
// the API declared, or the file is not trusted.
func downloadTo(ctx context.Context, asset update.Asset, stage string, sink Sink) (string, string, error) {
	body, err := fetch(ctx, asset.URL, asset.Size+1)
	if err != nil {
		return "", "", err
	}
	defer body.Close()

	path := filepath.Join(stage, asset.Name)
	out, err := os.Create(path)
	if err != nil {
		return "", "", core.Errorf(core.CodePermission,
			"CHIT could not write the download next to itself, so nothing was installed. Download it by hand instead.")
	}
	defer out.Close()

	digest := sha256.New()
	buf := make([]byte, copyChunk)
	written := int64(0)
	sink.Progress(0, int(asset.Size), "Downloading "+asset.Name)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return "", "", core.Errorf(core.CodePermission,
					"CHIT could not finish writing the download (the disk may be full), so nothing was installed.")
			}
			digest.Write(buf[:n])
			written += int64(n)
			sink.Progress(int(written), int(asset.Size), "Downloading "+asset.Name)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			return "", "", core.Errorf(core.CodeNetwork,
				"The download from github.com was cut short, so nothing was installed. Try again.")
		}
	}
	if written != asset.Size {
		return "", "", core.Errorf(core.CodeNetwork,
			"The download ended at %d bytes where the release says it is %d, so it was thrown away and nothing was installed. Try again.", written, asset.Size)
	}
	if err := out.Close(); err != nil {
		return "", "", core.Errorf(core.CodePermission,
			"CHIT could not finish writing the download (the disk may be full), so nothing was installed.")
	}
	return path, hex.EncodeToString(digest.Sum(nil)), nil
}

// fetchExpectedSum downloads the release's checksum file and returns the
// SHA-256 it publishes for name.
func fetchExpectedSum(ctx context.Context, url, name string) (string, error) {
	body, err := fetch(ctx, url, maxSumsBytes)
	if err != nil {
		return "", err
	}
	defer body.Close()
	text, err := io.ReadAll(body)
	if err != nil {
		return "", core.Errorf(core.CodeNetwork,
			"The release's checksum file could not be read, so the download cannot be verified and nothing was installed. Try again.")
	}
	sum, ok := parseSums(string(text), name)
	if !ok {
		return "", core.Errorf(core.CodeNetwork,
			"The release's checksum file does not mention %s, so the download cannot be verified and nothing was installed. Use the download page instead.", name)
	}
	return sum, nil
}

// parseSums reads sha256sum output ("<hex><spaces><name>", one file per line,
// a leading "*" on the name meaning binary mode) and returns the checksum
// recorded for name.
func parseSums(text, name string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := strings.ToLower(fields[0])
		if len(sum) != 64 || !isHex(sum) {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == name {
			return sum, true
		}
	}
	return "", false
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// fetch GETs a release URL and returns the body, capped at limit bytes. The
// URL always comes from the GitHub API's own answer, never from the page.
func fetch(ctx context.Context, url string, limit int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, core.Errorf(core.CodeInternal,
			"CHIT could not build the download request. This is a fault in CHIT itself.")
	}
	req.Header.Set("User-Agent", "CHIT")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, core.Errorf(core.CodeTimeout,
				"github.com stopped answering during the download, so nothing was installed. Try again.")
		}
		return nil, core.Errorf(core.CodeNetwork,
			"CHIT could not reach github.com to fetch the update. This machine may be offline, or a firewall or proxy may be blocking it.")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, core.Errorf(core.CodeNetwork,
			"github.com would not hand over the download, so nothing was installed. Try again in a few minutes, or use the download page instead.")
	}
	return readCloser{io.LimitReader(resp.Body, limit), resp.Body}, nil
}

// readCloser pairs a limited reader with the body it wraps so the connection
// still closes.
type readCloser struct {
	io.Reader
	closer io.Closer
}

func (rc readCloser) Close() error { return rc.closer.Close() }
