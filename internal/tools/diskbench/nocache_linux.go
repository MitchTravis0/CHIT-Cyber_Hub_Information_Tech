//go:build linux

package diskbench

import (
	"os"

	"golang.org/x/sys/unix"
)

// openUncached opens path for reading with this operating system's page cache
// bypassed. FADV_DONTNEED drops the pages Linux is holding for the file, which
// is why the writer must Sync first: it only drops clean pages.
//
// This needs no privileges. golang.org/x/sys is a direct dependency, so
// importing unix changes nothing in go.mod.
func openUncached(path string, size int64) (*os.File, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	if err := unix.Fadvise(int(file.Fd()), 0, size, unix.FADV_DONTNEED); err != nil {
		return file, false, nil
	}
	return file, true, nil
}

// readBuffer is a plain allocation: only Windows needs an aligned one.
func readBuffer(n int) []byte { return make([]byte, n) }
