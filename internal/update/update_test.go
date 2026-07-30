package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chit/internal/core"
)

// Every want below was computed by an independent python implementation of the
// same rule before being written in, so nothing here is checked only against the
// code it is testing.
func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0", "V1.0.0", 0},
		// The classic. A string comparison gets this one backwards.
		{"0.9.9", "0.10.0", -1},
		{"0.10.0", "0.9.9", 1},
		{"10.0.0", "9.0.0", 1},
		{"1.2", "1.2.0", 0},
		{"1.2.0", "1.2.1", -1},
		{"1.2.0-rc1", "1.2.0", 0},
		{"1.2.0+build7", "1.2.0", 0},
		// Found by mutation. With a final component of 0 the suffix could be left
		// in and the comparison still came out right, because "0-rc1" fails to
		// parse and the missing part counts as zero anyway. A non-zero final
		// component is the only input that tells the two apart.
		{"1.2.3-rc1", "1.2.3", 0},
		{"1.2.3-rc1", "1.2.4", -1},
		{"2.10.7+meta", "2.10.7", 0},
		// A part that is not a number ends the version. Skipping it instead would
		// read "1.x.3" as 1.3, which is a different and much later version.
		{"1.x.3", "1.0.3", -1},
		{"1.x.3", "1.0.0", 0},
		{"2.0.0", "1.99.99", 1},
		{"0.1.0", "1.0.0", -1},
		{"1.0.0", "1.0.0.1", -1},
		{"", "1.0.0", -1},
		{"abc", "1.0.0", -1},
		{"1.0.0", "v1.0.1", -1},
		{"  1.0.0  ", "1.0.0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			if got := Compare(tt.a, tt.b); got != tt.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			// Reversing the arguments must reverse the sign, always.
			if got := Compare(tt.b, tt.a); got != -tt.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tt.b, tt.a, got, -tt.want)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "v1.0.1", true},
		{"0.9.9", "0.10.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "v1.0.0", false},
		{"1.0.1", "1.0.0", false},
		{"1.0.0", "", false},
	}
	for _, tt := range tests {
		if got := IsNewer(tt.current, tt.latest); got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}

// serve points BaseURL at a fake GitHub for the length of one test.
func serve(t *testing.T, status int, body string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept header = %q", got)
		}
		// Asserted as the exact value, not just non-empty: Go's http client
		// supplies "Go-http-client/1.1" of its own accord, so a non-empty check
		// passes with the header removed entirely.
		if got := r.Header.Get("User-Agent"); got != "CHIT" {
			t.Errorf("User-Agent = %q, want %q; GitHub refuses requests without one", got, "CHIT")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	previous := BaseURL
	BaseURL = server.URL
	t.Cleanup(func() { BaseURL = previous })
}

func TestCheckFindsANewerRelease(t *testing.T) {
	serve(t, http.StatusOK, `{
		"tag_name": "v1.4.0",
		"html_url": "https://github.com/MitchTravis0/CHIT-Cyber_Hub_Information_Tech/releases/tag/v1.4.0",
		"published_at": "2026-08-01T09:30:00Z",
		"assets": [
			{"name": "chit-linux-amd64.tar.gz", "browser_download_url": "https://example.com/d/chit-linux-amd64.tar.gz", "size": 7340032},
			{"name": "sha256sums.txt", "browser_download_url": "https://example.com/d/sha256sums.txt", "size": 300}
		]
	}`)

	got, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !got.Newer {
		t.Error("Newer = false, want true")
	}
	if got.Latest != "1.4.0" {
		t.Errorf("Latest = %q, want %q (the leading v is stripped)", got.Latest, "1.4.0")
	}
	if got.Current != "1.0.0" {
		t.Errorf("Current = %q", got.Current)
	}
	if got.Published != "2026-08-01" {
		t.Errorf("Published = %q, want %q", got.Published, "2026-08-01")
	}
	if got.URL != "https://github.com/MitchTravis0/CHIT-Cyber_Hub_Information_Tech/releases/tag/v1.4.0" {
		t.Errorf("URL = %q", got.URL)
	}
	for _, want := range []string{"1.4.0", "1.0.0"} {
		if !strings.Contains(got.Note, want) {
			t.Errorf("Note = %q, want it to contain %q", got.Note, want)
		}
	}
	// selfupdate picks its download out of this list, so losing it silently
	// would turn every release into "no download for this machine".
	if len(got.Assets) != 2 {
		t.Fatalf("Assets = %v, want the 2 the release carries", got.Assets)
	}
	if got.Assets[0].Name != "chit-linux-amd64.tar.gz" ||
		got.Assets[0].URL != "https://example.com/d/chit-linux-amd64.tar.gz" ||
		got.Assets[0].Size != 7340032 {
		t.Errorf("Assets[0] = %+v, want the name, URL and size GitHub sent", got.Assets[0])
	}
}

func TestCheckOnTheLatestRelease(t *testing.T) {
	serve(t, http.StatusOK, `{"tag_name": "v1.0.0", "html_url": "https://example.com/r"}`)

	got, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if got.Newer {
		t.Error("Newer = true for the same version")
	}
	if got.Note != "You are running 1.0.0, which is the latest release." {
		t.Errorf("Note = %q", got.Note)
	}
	if got.Published != "" {
		t.Errorf("Published = %q, want empty when the API did not send a date", got.Published)
	}
}

func TestCheckOnADevelopmentBuild(t *testing.T) {
	serve(t, http.StatusOK, `{"tag_name": "v1.0.0"}`)

	got, err := Check(context.Background(), "1.1.0")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if got.Newer {
		t.Error("Newer = true while running ahead of the latest release")
	}
	if !strings.Contains(got.Note, "development build") {
		t.Errorf("Note = %q, want it to say this is a development build", got.Note)
	}
	// No html_url in the answer, so it must fall back to the releases page
	// rather than leaving the button pointing nowhere.
	if got.URL != "https://github.com/MitchTravis0/CHIT-Cyber_Hub_Information_Tech/releases" {
		t.Errorf("URL = %q, want the releases page", got.URL)
	}
}

// GitHub answers 404 when a repository has never published a release, which is
// exactly the state this repository is in. It is not a failure.
func TestCheckWithNoReleasesIsNotAnError(t *testing.T) {
	serve(t, http.StatusNotFound, `{"message": "Not Found"}`)

	got, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("a 404 must not be an error, got %v", err)
	}
	if got.Newer {
		t.Error("Newer = true with no releases at all")
	}
	if got.Latest != "" {
		t.Errorf("Latest = %q, want empty", got.Latest)
	}
	if !strings.Contains(got.Note, "no published releases yet") {
		t.Errorf("Note = %q", got.Note)
	}
	if got.URL != "https://github.com/MitchTravis0/CHIT-Cyber_Hub_Information_Tech/releases" {
		t.Errorf("URL = %q, want the releases page", got.URL)
	}
}

func TestCheckErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
		wantText string
	}{
		{"rate limited", http.StatusForbidden, `{}`, core.CodeNetwork, "rate limiting"},
		{"too many requests", http.StatusTooManyRequests, `{}`, core.CodeNetwork, "rate limiting"},
		{"server error", http.StatusInternalServerError, `{}`, core.CodeNetwork, "answered the update check with an error"},
		{"unauthorised", http.StatusUnauthorized, `{}`, core.CodeNetwork, "answered the update check with an error"},
		{"garbage", http.StatusOK, `not json at all`, core.CodeNetwork, "could not make sense"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serve(t, tt.status, tt.body)
			_, err := Check(context.Background(), "1.0.0")
			if err == nil {
				t.Fatal("want an error")
			}
			if core.CodeOf(err) != tt.wantCode {
				t.Errorf("code = %q, want %q", core.CodeOf(err), tt.wantCode)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("message = %q, want it to contain %q", err.Error(), tt.wantText)
			}
			// No raw Go or GitHub wording ever reaches the user.
			for _, banned := range []string{"http:", "status code", "json:", "unexpected"} {
				if strings.Contains(strings.ToLower(err.Error()), banned) {
					t.Errorf("message %q leaks library wording %q", err.Error(), banned)
				}
			}
		})
	}
}

// A release with no tag is not something to compare against, but it is also not
// a failure worth an error dialog.
func TestCheckWithNoTagName(t *testing.T) {
	serve(t, http.StatusOK, `{"html_url": "https://example.com/r"}`)

	got, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if got.Newer || got.Latest != "" {
		t.Errorf("Newer = %v, Latest = %q, want false and empty", got.Newer, got.Latest)
	}
	if !strings.Contains(got.Note, "did not name a version") {
		t.Errorf("Note = %q", got.Note)
	}
}

func TestCheckCannotReachGitHub(t *testing.T) {
	previous := BaseURL
	// Reserved for documentation examples, so nothing is listening.
	BaseURL = "http://127.0.0.1:1/releases/latest"
	t.Cleanup(func() { BaseURL = previous })

	_, err := Check(context.Background(), "1.0.0")
	if err == nil {
		t.Fatal("want an error when github.com cannot be reached")
	}
	if core.CodeOf(err) != core.CodeNetwork {
		t.Errorf("code = %q, want %q", core.CodeOf(err), core.CodeNetwork)
	}
	if !strings.Contains(err.Error(), "Everything else in CHIT works without it") {
		t.Errorf("message = %q, want it to reassure the tech", err.Error())
	}
}

// The note is the only thing the page shows, so it can never be empty.
func TestNoteIsAlwaysSet(t *testing.T) {
	cases := []struct {
		status int
		body   string
	}{
		{http.StatusOK, `{"tag_name": "v2.0.0"}`},
		{http.StatusOK, `{"tag_name": "v1.0.0"}`},
		{http.StatusOK, `{"tag_name": "v0.1.0"}`},
		{http.StatusOK, `{}`},
		{http.StatusNotFound, `{}`},
	}
	for _, tc := range cases {
		serve(t, tc.status, tc.body)
		got, err := Check(context.Background(), "1.0.0")
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}
		if strings.TrimSpace(got.Note) == "" {
			t.Errorf("%s gave an empty Note", tc.body)
		}
		if got.URL == "" {
			t.Errorf("%s gave an empty URL", tc.body)
		}
	}
}

// This package must never gain the ability to write to disk or run anything.
// Since Phase 10 the app can replace its own executable, but that larger
// promise lives entirely in internal/selfupdate, where its safeguards are
// tested; this package stays the read-only half so the split is enforceable.
func TestPackageNeverWritesOrExecutes(t *testing.T) {
	source := readSource(t)
	for _, banned := range []string{"os.WriteFile", "os.Create", "os.OpenFile", "os.Remove", "exec.Command", "os.Rename"} {
		if strings.Contains(source, banned) {
			t.Errorf("update.go calls %s; this package compares versions and does not self-patch", banned)
		}
	}
}
