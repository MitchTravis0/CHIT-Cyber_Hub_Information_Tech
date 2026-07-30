package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"chit/internal/core"
)

// bundleRoot is the top-level folder inside the macOS zip, and linuxBinary the
// single member of the Linux tar.gz. Both come from the release workflow's
// packaging steps, which archive build/bin/chit.app and build/bin/chit.
const (
	bundleRoot  = "chit.app"
	linuxBinary = "chit"
)

// extractTarGz pulls the chit binary out of the Linux release archive into
// dir and returns its path, executable bit set.
func extractTarGz(archive, dir string) (string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", core.Errorf(core.CodeInternal,
			"CHIT lost track of the file it just downloaded. Nothing was installed; try again.")
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", badArchive()
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", badArchive()
		}
		if header.Typeflag != tar.TypeReg || path.Clean(header.Name) != linuxBinary {
			continue
		}
		out := filepath.Join(dir, linuxBinary)
		if err := writeFile(out, reader, 0o755); err != nil {
			return "", err
		}
		return out, nil
	}
	return "", core.Errorf(core.CodeNetwork,
		"The downloaded archive does not contain the chit program, so nothing was installed. Use the download page instead.")
}

// extractZip unpacks the chit.app bundle out of the macOS release archive into
// dir and returns the bundle's path. Every entry must sit inside chit.app/ and
// be an ordinary file or folder: anything else is refused rather than guessed
// at, because what gets written here is about to become the installed app.
func extractZip(archive, dir string) (string, error) {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return "", badArchive()
	}
	defer zr.Close()

	total := int64(0)
	for _, entry := range zr.File {
		name := path.Clean(entry.Name)
		if name != bundleRoot && !strings.HasPrefix(name, bundleRoot+"/") {
			return "", badArchive()
		}
		dest := filepath.Join(dir, filepath.FromSlash(name))
		mode := entry.Mode()
		switch {
		case entry.FileInfo().IsDir():
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return "", writeFailed()
			}
		case mode&os.ModeType != 0:
			// a symlink or device node; a Wails bundle contains neither
			return "", badArchive()
		default:
			total += int64(entry.UncompressedSize64)
			if total > maxAssetBytes {
				return "", badArchive()
			}
			file, err := entry.Open()
			if err != nil {
				return "", badArchive()
			}
			// keep the executable bit for Contents/MacOS/chit, floor the rest
			perm := os.FileMode(0o644)
			if mode&0o111 != 0 {
				perm = 0o755
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				file.Close()
				return "", writeFailed()
			}
			err = writeFile(dest, io.LimitReader(file, maxAssetBytes), perm)
			file.Close()
			if err != nil {
				return "", err
			}
		}
	}
	bundle := filepath.Join(dir, bundleRoot)
	if _, err := os.Stat(filepath.Join(bundle, "Contents", "MacOS", linuxBinary)); err != nil {
		return "", core.Errorf(core.CodeNetwork,
			"The downloaded archive does not contain the chit program, so nothing was installed. Use the download page instead.")
	}
	return bundle, nil
}

func writeFile(dest string, from io.Reader, perm os.FileMode) error {
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return writeFailed()
	}
	if _, err := io.Copy(out, from); err != nil {
		out.Close()
		return writeFailed()
	}
	if err := out.Close(); err != nil {
		return writeFailed()
	}
	return nil
}

func badArchive() error {
	return core.Errorf(core.CodeNetwork,
		"The downloaded file is not the archive a CHIT release ships, so it was thrown away and nothing was installed. Try again, or use the download page instead.")
}

func writeFailed() error {
	return core.Errorf(core.CodePermission,
		"CHIT could not write the unpacked update next to itself (the disk may be full), so nothing was installed.")
}

// replaceTarget swaps newPath into target's place. It fails closed: the
// running version is renamed aside first, and renamed back if the new one
// cannot take its place, so a failed update always leaves a working CHIT.
// Windows allows renaming a running exe but not deleting it, which is why the
// final removal is best-effort and CleanupLeftovers exists.
func replaceTarget(target, newPath string) error {
	old := target + oldSuffix
	if err := os.RemoveAll(old); err != nil {
		return core.Errorf(core.CodePermission,
			"CHIT could not clear %s, left over from an earlier update. Delete it by hand and try again.", old)
	}
	if err := os.Rename(target, old); err != nil {
		return core.Errorf(core.CodePermission,
			"CHIT could not move its current version aside, so nothing was replaced. Download the update by hand instead.")
	}
	if err := os.Rename(newPath, target); err != nil {
		if back := os.Rename(old, target); back != nil {
			return core.Errorf(core.CodeInternal,
				"The new version could not be moved into place, and putting the old one back failed too. The copy that was running is at %s: rename it back to %s by hand before closing CHIT.", old, target)
		}
		return core.Errorf(core.CodeInternal,
			"The new version could not be moved into place, so the old one was put back and nothing changed. Try again, or use the download page instead.")
	}
	_ = os.RemoveAll(old)
	return nil
}
