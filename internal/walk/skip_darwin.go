//go:build darwin

package walk

// skipRoots are the paths that are not really reclaimable disk space.
// /System/Volumes/VM holds the swap file, which is several GB and is not
// something a tech can delete, so counting it would send them chasing it.
var skipRoots = []string{"/dev", "/System/Volumes/VM"}
