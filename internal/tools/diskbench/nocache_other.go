//go:build !windows && !linux && !darwin

package diskbench

import "os"

// openUncached on an operating system nobody has written a cache hint for. The
// read still happens; the false says the figure may be coming from memory, and
// the page prints that rather than pretending.
func openUncached(path string, size int64) (*os.File, bool, error) {
	file, err := os.Open(path)
	return file, false, err
}

func readBuffer(n int) []byte { return make([]byte, n) }
