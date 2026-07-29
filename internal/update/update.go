// Package update asks GitHub whether a newer release of CHIT exists. It never
// downloads or replaces anything: it compares two version strings and hands the
// settings page a link. This sits beside internal/core and internal/store rather
// than under internal/tools, because it is app-level and belongs to no tool.
//
// Nothing here runs unless a person presses the button. CHIT is handed to junior
// techs on customer networks, so it does not contact GitHub on its own.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chit/internal/core"
)

const (
	// Repo is the project this build checks against.
	Repo = "MitchTravis0/CHIT-Cyber_Hub_Information_Tech"
	// releasesPage is where a person is sent to download a newer build.
	releasesPage = "https://github.com/" + Repo + "/releases"
	// timeout is the whole check, including connecting.
	timeout = 10 * time.Second
	// maxBody caps what is read from the API. A release payload is a few KB;
	// this exists so a wrong endpoint cannot stream forever.
	maxBody = 1 << 20
)

// BaseURL is the GitHub API endpoint. A variable so the tests can point it at an
// httptest server; nothing in the app sets it.
var BaseURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

// Result is what the settings page shows.
type Result struct {
	// Current is the version this build reports.
	Current string `json:"current"`
	// Latest is the newest published release, without a leading "v". Empty when
	// there are no releases.
	Latest string `json:"latest"`
	// URL is where to download it. Always set, so the page can offer the
	// releases page even when there is nothing newer.
	URL string `json:"url"`
	// Newer is true only when Latest is genuinely a later version than Current.
	Newer bool `json:"newer"`
	// Published is the release date as YYYY-MM-DD, empty when unknown.
	Published string `json:"published"`
	// Note is the sentence to show. Always set.
	Note string `json:"note"`
}

// release is the part of GitHub's answer this package reads.
type release struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

// Check asks GitHub for the latest release and compares it with current.
func Check(ctx context.Context, current string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out := Result{Current: current, URL: releasesPage}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL, nil)
	if err != nil {
		return Result{}, core.Errorf(core.CodeInternal,
			"CHIT could not build the request to check for updates. This is a fault in CHIT itself.")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CHIT")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{}, core.Errorf(core.CodeTimeout,
				"github.com did not answer within %d seconds. Check the update by hand at %s.",
				int(timeout.Seconds()), releasesPage)
		}
		return Result{}, core.Errorf(core.CodeNetwork,
			"CHIT could not reach github.com to check for updates. This machine may be offline, or a firewall or proxy may be blocking it. Everything else in CHIT works without it.")
	}
	defer resp.Body.Close()

	switch {
	// GitHub answers 404 when a repository has published no releases at all,
	// which is not a failure and must not be shown as one.
	case resp.StatusCode == http.StatusNotFound:
		out.Note = "There are no published releases yet, so there is nothing newer than the copy you are running."
		return out, nil
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return Result{}, core.Errorf(core.CodeNetwork,
			"github.com is rate limiting this machine, so the check could not run. Wait a few minutes, or look at %s yourself.", releasesPage)
	case resp.StatusCode != http.StatusOK:
		return Result{}, core.Errorf(core.CodeNetwork,
			"github.com answered the update check with an error. Look at %s yourself.", releasesPage)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Result{}, core.Errorf(core.CodeNetwork,
			"The answer from github.com was cut short, so the update check could not finish. Try it again.")
	}

	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return Result{}, core.Errorf(core.CodeNetwork,
			"CHIT could not make sense of the answer from github.com. Look at %s yourself.", releasesPage)
	}

	latest := strings.TrimSpace(rel.TagName)
	if latest == "" {
		out.Note = "github.com did not name a version for the latest release, so there is nothing to compare against. Look at " + releasesPage + " yourself."
		return out, nil
	}
	out.Latest = strings.TrimPrefix(latest, "v")
	if rel.HTMLURL != "" {
		out.URL = rel.HTMLURL
	}
	if published, err := time.Parse(time.RFC3339, rel.PublishedAt); err == nil {
		out.Published = published.Format("2006-01-02")
	}

	out.Newer = IsNewer(current, latest)
	switch {
	case out.Newer:
		out.Note = fmt.Sprintf(
			"Version %s is available. You are running %s. CHIT does not update itself: download the new file and replace this one.",
			out.Latest, current)
	case Compare(current, latest) > 0:
		out.Note = fmt.Sprintf(
			"You are running %s, which is newer than the latest published release (%s). This is a development build.",
			current, out.Latest)
	default:
		out.Note = fmt.Sprintf("You are running %s, which is the latest release.", current)
	}
	return out, nil
}

// IsNewer reports whether latest is a later version than current.
func IsNewer(current, latest string) bool {
	return Compare(current, latest) < 0
}

// Compare returns -1 when a is older than b, 1 when it is newer, and 0 when they
// are the same version. A leading "v" is ignored, so "v1.0.0" and "1.0.0" match,
// and any pre-release or build suffix is dropped: "1.2.0-rc1" compares as 1.2.0.
// Missing parts count as zero, so "1.2" and "1.2.0" are the same version.
func Compare(a, b string) int {
	left, right := parts(a), parts(b)
	for i := 0; i < len(left) || i < len(right); i++ {
		x, y := 0, 0
		if i < len(left) {
			x = left[i]
		}
		if i < len(right) {
			y = right[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parts turns "v1.10.2-rc1" into [1, 10, 2]. A part that is not a number ends
// the version, so a tag CHIT does not understand compares as far as it made
// sense and no further, rather than being read as zero.
func parts(version string) []int {
	text := strings.TrimSpace(version)
	text = strings.TrimPrefix(strings.TrimPrefix(text, "v"), "V")
	if cut := strings.IndexAny(text, "-+ "); cut >= 0 {
		text = text[:cut]
	}
	out := []int{}
	for _, field := range strings.Split(text, ".") {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			break
		}
		out = append(out, n)
	}
	return out
}
