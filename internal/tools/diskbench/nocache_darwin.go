//go:build darwin

package diskbench

import (
	"os"

	"golang.org/x/sys/unix"
)

// openUncached opens path for reading with this operating system's page cache
// bypassed. F_NOCACHE tells macOS not to keep the pages this handle reads, which
// is the unprivileged equivalent of Linux's FADV_DONTNEED.
func openUncached(path string, size int64) (*os.File, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	if _, err := unix.FcntlInt(file.Fd(), unix.F_NOCACHE, 1); err != nil {
		return file, false, nil
	}
	return file, true, nil
}

// readBuffer is a plain allocation: only Windows needs an aligned one.
func readBuffer(n int) []byte { return make([]byte, n) }
