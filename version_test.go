package main

import (
	"encoding/json"
	"os"
	"testing"

	"chit/internal/update"
)

// Version is in two places that cannot see each other: the Go constant the
// settings page and the update check read, and wails.json's productVersion,
// which is what Windows shows in the file's properties. Nothing but this test
// makes them agree, and a mismatch means the update check compares against a
// version the binary does not claim to be.
func TestVersionMatchesWailsJSON(t *testing.T) {
	data, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatalf("reading wails.json: %v", err)
	}
	var config struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing wails.json: %v", err)
	}
	if config.Info.ProductVersion == "" {
		t.Fatal("wails.json has no info.productVersion; this test would pass over nothing")
	}
	if config.Info.ProductVersion != Version {
		t.Errorf("wails.json productVersion = %q, Version = %q; they have to match",
			config.Info.ProductVersion, Version)
	}
}

// The update check compares Version against a GitHub tag. A version it cannot
// parse would compare as 0.0.0 and report every release as newer, for ever.
func TestVersionIsComparable(t *testing.T) {
	if update.Compare(Version, "0.0.0") <= 0 {
		t.Errorf("Version %q does not compare as later than 0.0.0, so it did not parse", Version)
	}
	if update.Compare(Version, Version) != 0 {
		t.Errorf("Version %q does not compare equal to itself", Version)
	}
	// The literal is written in rather than read from the constant: this is the
	// version being shipped, and changing the constant should be a deliberate
	// act that fails here first.
	if Version != "1.0.2" {
		t.Errorf("Version = %q, want %q. If this is intentional, update the literal and the git tag together.", Version, "1.0.2")
	}
}
