//go:build linux

package walk

import "testing"

// The skip list is behaviour, not decoration: a walk of / that descends into
// /proc takes minutes and reports sizes that are not on the disk. These
// expectations are written out rather than read from skipRoots.
func TestLinuxSkipsPseudoFilesystems(t *testing.T) {
	skipped := []string{"/proc", "/sys", "/dev", "/run"}
	for _, dir := range skipped {
		if !shouldSkip(dir) {
			t.Errorf("shouldSkip(%q) = false, want true", dir)
		}
	}

	kept := []string{"/", "/home", "/var", "/usr", "/home/me/dev", "/devices", "/proc/1"}
	for _, dir := range kept {
		if shouldSkip(dir) {
			t.Errorf("shouldSkip(%q) = true, want false", dir)
		}
	}
}
