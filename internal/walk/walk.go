// Package walk streams a directory tree once, without ever following a
// symbolic link, so a scan cannot loop for ever and cannot wander onto a
// network share by accident. It is shared by the Disk Space Visualizer and the
// Duplicate File Finder.
package walk

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chit/internal/core"
)

// File is one regular file found by a walk.
type File struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// Result is the tally a walk hands back. Unreadable and Skipped are the two
// numbers a user needs to judge whether the total can be trusted.
type Result struct {
	Files int
	Dirs  int
	Bytes int64
	// Unreadable counts directories the operating system refused to list.
	// Their contents are missing from the totals.
	Unreadable int
	// Skipped counts entries stepped over on purpose: symbolic links, device
	// nodes, sockets, pipes, and the pseudo filesystems in skipRoots.
	Skipped int
}

// Options controls one walk. The zero value of MinSize keeps every file.
type Options struct {
	Root string
	// MinSize drops files smaller than this. Zero keeps every file.
	MinSize int64
}

// Add folds one result into another, so a caller walking several subtrees can
// report one tally.
func (r *Result) Add(other Result) {
	r.Files += other.Files
	r.Dirs += other.Dirs
	r.Bytes += other.Bytes
	r.Unreadable += other.Unreadable
	r.Skipped += other.Skipped
}

// Walk visits every regular file under opts.Root. visit is called on the
// walking goroutine, one file at a time, so it needs no locking. Returning an
// error from visit stops the walk and Walk returns that error. Walk checks ctx
// before every directory read and before every visit, so a cancel is noticed
// within one directory.
func Walk(ctx context.Context, opts Options, visit func(File) error) (Result, error) {
	var res Result
	err := walkDir(ctx, opts.Root, opts, &res, visit)
	return res, err
}

func walkDir(ctx context.Context, dir string, opts Options, res *Result, visit func(File) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if shouldSkip(dir) {
		res.Skipped++
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		res.Unreadable++
		return nil
	}
	res.Dirs++

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, entry.Name())

		// Type() comes from the directory entry, so a symbolic link is spotted
		// without a second syscall and is never resolved.
		mode := entry.Type()
		switch {
		case mode&fs.ModeSymlink != 0:
			res.Skipped++
		case entry.IsDir():
			if err := walkDir(ctx, path, opts, res, visit); err != nil {
				return err
			}
		case mode.IsRegular():
			info, err := entry.Info()
			if err != nil {
				// The file went away between the listing and the stat.
				res.Skipped++
				continue
			}
			if opts.MinSize > 0 && info.Size() < opts.MinSize {
				continue
			}
			res.Files++
			res.Bytes += info.Size()
			if err := visit(File{Path: path, Size: info.Size(), ModTime: info.ModTime()}); err != nil {
				return err
			}
		default:
			// Devices, sockets and pipes have no size worth counting and
			// reading one would block.
			res.Skipped++
		}
	}
	return nil
}

// shouldSkip reports whether a directory is one of this operating system's
// pseudo filesystems.
func shouldSkip(dir string) bool { return skipPath(dir, skipRoots) }

// skipPath takes the list rather than reading the package variable, so the
// exact-match rule can be tested on any operating system.
//
// The match is exact on the cleaned path: a user's own folder called
// /home/me/dev must not be mistaken for /dev.
func skipPath(dir string, roots []string) bool {
	clean := filepath.Clean(dir)
	for _, root := range roots {
		if clean == root {
			return true
		}
	}
	return false
}

// Root cleans and checks a folder the user chose, returning the absolute path.
func Root(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", core.Errorf(core.CodeInvalidInput,
			"Choose a folder to scan, or type the full path to it.")
	}

	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", core.Errorf(core.CodeInternal,
			"%s could not be opened. It may be on a drive or network share that is no longer connected.", trimmed)
	}

	info, err := os.Stat(abs)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return "", core.Errorf(core.CodeNotFound,
				"There is no folder at %s. Check the path, or press Choose folder and pick it.", abs)
		case errors.Is(err, fs.ErrPermission):
			return "", core.Errorf(core.CodePermission,
				"This computer will not let CHIT look inside %s. Pick a folder you have access to, such as your own Documents folder.", abs)
		default:
			return "", core.Errorf(core.CodeInternal,
				"%s could not be opened. It may be on a drive or network share that is no longer connected.", abs)
		}
	}
	if !info.IsDir() {
		return "", core.Errorf(core.CodeInvalidInput,
			"%s is a file, not a folder. Pick the folder it is in.", abs)
	}

	// A folder that stats but cannot be listed is the case a user hits on
	// another account's profile. Catching it here means a field error rather
	// than a job that starts and finds nothing.
	f, err := os.Open(abs)
	if err != nil {
		return "", core.Errorf(core.CodePermission,
			"This computer will not let CHIT look inside %s. Pick a folder you have access to, such as your own Documents folder.", abs)
	}
	_, readErr := f.ReadDir(1)
	f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", core.Errorf(core.CodePermission,
			"This computer will not let CHIT look inside %s. Pick a folder you have access to, such as your own Documents folder.", abs)
	}

	return abs, nil
}
