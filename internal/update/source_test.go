package update

import (
	"os"
	"testing"
)

// readSource returns update.go so a test can assert on what the package does not
// do. Grepping our own source is the only way to pin "never writes to disk":
// there is no input that makes a missing write observable.
func readSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("update.go")
	if err != nil {
		t.Fatalf("reading update.go: %v", err)
	}
	return string(data)
}
