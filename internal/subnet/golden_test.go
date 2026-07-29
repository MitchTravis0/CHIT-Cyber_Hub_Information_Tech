package subnet

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// TestGoldenCases holds this package to the same golden file as the frontend
// copy of the same arithmetic (frontend/src/tools/subnet-calculator/subnet.ts,
// exercised by frontend/tests/subnet.test.ts). The Subnet Calculator recomputes
// on every keystroke, so the maths it shows runs in the frontend; this package
// is the reference the frontend copy is checked against, and the golden file is
// what stops the two from drifting apart.
func TestGoldenCases(t *testing.T) {
	var golden struct {
		Cases []Info `json:"cases"`
	}

	raw, err := os.ReadFile("../../testdata/subnet-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Cases) < 50 {
		t.Fatalf("golden file holds only %d cases, it is probably truncated", len(golden.Cases))
	}

	for _, want := range golden.Cases {
		t.Run(want.Input, func(t *testing.T) {
			got, err := Calculate(want.Input)
			if err != nil {
				t.Fatalf("Calculate(%q) failed: %v", want.Input, err)
			}
			if !reflect.DeepEqual(got, want) {
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				t.Errorf("Calculate(%q)\n got %s\nwant %s", want.Input, gotJSON, wantJSON)
			}
		})
	}
}
