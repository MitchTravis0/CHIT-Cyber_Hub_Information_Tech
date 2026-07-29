package rawprint

import (
	"os"
	"testing"
)

// sourceOf reads one of this package's own files, so a test can assert what the
// code does not contain. internal/tools/dupfind uses the same trick to prove it
// never deletes anything.
func sourceOf(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}
