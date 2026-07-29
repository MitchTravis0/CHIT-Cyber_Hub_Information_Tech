//go:build !linux && !darwin && !windows

package walk

// An operating system nobody has written a list for. Skipping nothing is the
// safe default: the walk still refuses to follow links, so it cannot loop.
var skipRoots = []string{}
