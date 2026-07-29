//go:build linux || darwin

package netinfo

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// run executes a system tool and returns its stdout. Every caller treats a
// failure as "this OS did not tell us", so the error detail is not needed.
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// readFile returns the contents of path, or "" when it cannot be read.
func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
