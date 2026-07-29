//go:build linux || darwin

package walk

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWalkSkipsNonRegularFiles(t *testing.T) {
	root := tree(t, map[string]int{"real.txt": 10})
	fifo := filepath.Join(root, "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("this machine will not create a FIFO: %v", err)
	}

	files, res := collect(t, root, Options{})

	if len(files) != 1 {
		t.Fatalf("visited %d files, want 1: reading a FIFO would block for ever", len(files))
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", res.Skipped)
	}
	if _, err := os.Stat(fifo); err != nil {
		t.Fatalf("the FIFO went missing: %v", err)
	}
}
