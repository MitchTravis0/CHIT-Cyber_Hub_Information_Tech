package ntpcheck

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestGapCases pins Go's wording of a clock difference against the same golden
// file frontend/tests/ntp-offset.test.ts reads. The two languages word the same
// gap in two places on the same page (the sentence under the table and the
// Difference column), and without this they are free to drift apart.
func TestGapCases(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/ntp-gap-cases.json")
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	var golden struct {
		Cases []struct {
			MS  int64  `json:"ms"`
			Gap string `json:"gap"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("parse golden file: %v", err)
	}
	if len(golden.Cases) < 20 {
		t.Fatalf("golden file has %d cases, want at least 20: a shrunken file would make this vacuous", len(golden.Cases))
	}

	for _, c := range golden.Cases {
		d := time.Duration(c.MS) * time.Millisecond
		if got := describeGap(d); got != c.Gap {
			t.Errorf("describeGap(%d ms) = %q, want %q", c.MS, got, c.Gap)
		}
		// The sign is carried by the surrounding sentence, not by the gap, so
		// the negative of every case must word identically.
		if got := describeGap(-d); got != c.Gap {
			t.Errorf("describeGap(-%d ms) = %q, want %q", c.MS, got, c.Gap)
		}
	}
}
