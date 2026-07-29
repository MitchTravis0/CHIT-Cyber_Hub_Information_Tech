//go:build linux

package walk

// skipRoots are the kernel's pseudo filesystems. Walking them wastes minutes,
// reports sizes that are not really on the disk, and in /proc changes under
// your feet as processes come and go.
var skipRoots = []string{"/proc", "/sys", "/dev", "/run"}
