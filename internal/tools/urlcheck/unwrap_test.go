package urlcheck

import (
	"net/url"
	"testing"
)

func TestUnwrapSafeLinks(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"regional host", "https://eur02.safelinks.protection.outlook.com/?url=https%3A%2F%2Fexample.com%2Flogin&data=05%7C01%7C"},
		{"bare host", "https://safelinks.protection.outlook.com/?url=https%3A%2F%2Fexample.com%2Flogin"},
		{"uppercase host", "https://EUR02.SafeLinks.Protection.Outlook.com/?url=https%3A%2F%2Fexample.com%2Flogin"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, steps, notes := unwrapAll(tc.raw)
			if out != "https://example.com/login" {
				t.Errorf("out = %q, want %q", out, "https://example.com/login")
			}
			if len(steps) != 1 {
				t.Fatalf("got %d steps, want 1", len(steps))
			}
			if steps[0].Wrapper != "Microsoft Defender Safe Links" {
				t.Errorf("wrapper = %q", steps[0].Wrapper)
			}
			if steps[0].To != "https://example.com/login" {
				t.Errorf("to = %q", steps[0].To)
			}
			if len(notes) != 0 {
				t.Errorf("got %d notes, want none", len(notes))
			}
		})
	}
}

func TestUnwrapProofpointV2(t *testing.T) {
	// The value carries both of the substitutions Proofpoint makes: "-" for a
	// percent sign and "_" for a slash.
	raw := "https://urldefense.proofpoint.com/v2/url?u=https-3A__example.com_path-3Fa-3D1-26b-3D2&d=DwMFaQ&c=abc"
	want := "https://example.com/path?a=1&b=2"

	out, steps, _ := unwrapAll(raw)
	if out != want {
		t.Fatalf("out = %q, want %q", out, want)
	}
	if len(steps) != 1 || steps[0].Wrapper != "Proofpoint URL Defense" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestUnwrapProofpointV3(t *testing.T) {
	raw := "https://urldefense.proofpoint.com/v3/__https://example.com/login__;!!abc$"

	out, steps, notes := unwrapAll(raw)
	if out != raw {
		t.Errorf("out = %q, want it unchanged", out)
	}
	if len(steps) != 0 {
		t.Errorf("got %d steps, want none", len(steps))
	}
	if len(notes) != 1 || notes[0].ID != "urldefense-v3" {
		t.Fatalf("notes = %+v", notes)
	}
	if notes[0].Severity != "info" {
		t.Errorf("severity = %q, want info", notes[0].Severity)
	}
}

func TestUnwrapGoogle(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"q parameter", "https://www.google.com/url?q=https%3A%2F%2Fexample.com%2F", "https://example.com/"},
		{"url parameter", "https://www.google.com/url?url=https%3A%2F%2Fexample.com%2F", "https://example.com/"},
		{"country domain", "https://google.co.uk/url?q=https%3A%2F%2Fexample.com%2F", "https://example.com/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, steps, _ := unwrapAll(tc.raw)
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if len(steps) != 1 || steps[0].Wrapper != "Google redirect" {
				t.Errorf("steps = %+v", steps)
			}
		})
	}

	search := "https://www.google.com/search?q=https://example.com"
	out, steps, _ := unwrapAll(search)
	if out != search || len(steps) != 0 {
		t.Errorf("a search URL was unwrapped: out = %q, steps = %+v", out, steps)
	}
}

func TestUnwrapMimecast(t *testing.T) {
	raw := "https://protect-eu.mimecast.com/s/abc123"

	out, steps, notes := unwrapAll(raw)
	if out != raw {
		t.Errorf("out = %q, want it unchanged", out)
	}
	if len(steps) != 0 {
		t.Errorf("got %d steps, want none", len(steps))
	}
	if len(notes) != 1 || notes[0].ID != "mimecast" {
		t.Fatalf("notes = %+v", notes)
	}
}

func TestUnwrapBarracuda(t *testing.T) {
	raw := "https://linkprotect.cudasvc.com/url?a=https%3A%2F%2Fexample.com%2F&c=E,1,abc"

	out, steps, _ := unwrapAll(raw)
	if out != "https://example.com/" {
		t.Errorf("out = %q", out)
	}
	if len(steps) != 1 || steps[0].Wrapper != "Barracuda Link Protection" {
		t.Errorf("steps = %+v", steps)
	}
}

func TestUnwrapNested(t *testing.T) {
	inner := "https://safelinks.protection.outlook.com/?url=" + url.QueryEscape("https://example.com/")
	raw := "https://www.google.com/url?q=" + url.QueryEscape(inner)

	out, steps, _ := unwrapAll(raw)
	if out != "https://example.com/" {
		t.Fatalf("out = %q", out)
	}
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}
	if steps[0].Wrapper != "Google redirect" || steps[1].Wrapper != "Microsoft Defender Safe Links" {
		t.Errorf("steps out of order: %+v", steps)
	}
	if steps[0].To != steps[1].From {
		t.Errorf("step 1 ends at %q but step 2 starts at %q", steps[0].To, steps[1].From)
	}
}

func TestUnwrapLeavesOrdinaryURL(t *testing.T) {
	raw := "https://example.com/a?b=c"

	out, steps, notes := unwrapAll(raw)
	if out != raw {
		t.Errorf("out = %q, want it unchanged", out)
	}
	if len(steps) != 0 || len(notes) != 0 {
		t.Errorf("steps = %+v, notes = %+v", steps, notes)
	}
}

func TestUnwrapRejectsBadInner(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"relative", "https://safelinks.protection.outlook.com/?url=" + url.QueryEscape("/relative")},
		{"javascript", "https://safelinks.protection.outlook.com/?url=" + url.QueryEscape("javascript:alert(1)")},
		{"missing", "https://safelinks.protection.outlook.com/?data=1"},
		{"empty", "https://safelinks.protection.outlook.com/?url="},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, steps, _ := unwrapAll(tc.raw)
			if out != tc.raw {
				t.Errorf("out = %q, want it unchanged", out)
			}
			if len(steps) != 0 {
				t.Errorf("got %d steps, want none", len(steps))
			}
		})
	}
}

func TestUnwrapStopsAfterFive(t *testing.T) {
	// A URL cannot literally contain itself, so the worst real case is a deeply
	// nested one. Eight layers is more than the guard allows.
	raw := "https://example.com/"
	for i := 0; i < 8; i++ {
		raw = "https://safelinks.protection.outlook.com/?url=" + url.QueryEscape(raw)
	}

	out, steps, _ := unwrapAll(raw)
	if len(steps) != maxUnwraps {
		t.Fatalf("got %d steps, want %d", len(steps), maxUnwraps)
	}
	if out == "" {
		t.Error("out is empty")
	}
}
