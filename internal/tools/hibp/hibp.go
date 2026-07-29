// Package hibp asks haveibeenpwned whether a password appears in a public
// breach, using the k-anonymity range endpoint so that only the first five
// characters of the password's SHA-1 fingerprint ever leave this machine.
//
// Password strength scoring is deliberately not here: it lives in
// frontend/src/tools/breach-checker/strength.ts so the meter can move on every
// keystroke without the password crossing the bridge.
package hibp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chit/internal/core"
)

const (
	// rangeBase is the k-anonymity endpoint. Only a five character hash prefix
	// is ever appended to it.
	rangeBase = "https://api.pwnedpasswords.com/range/"
	// userAgent is required: the API rejects a request that does not send one.
	userAgent = "CHIT-Password-Checker"
	// maxBody caps the response. A padded range is a few hundred KB at most.
	maxBody = 4 << 20
)

// The three levels the verdict card is tinted by. There is no "warn" level:
// a password is either in the corpus or it is not.
const (
	levelOK      = "ok"
	levelDanger  = "danger"
	levelUnknown = "unknown"
)

const (
	verdictNotFound    = "This password was not found in any of the leaked password lists. That does not make it a strong password, so read the strength rating above as well."
	verdictUnreachable = "The breach list could not be reached, so nothing was checked. That is not the same as safe. Check this machine's internet connection and try again."
	verdictRefused     = "The breach list refused the request, so nothing was checked. That is not the same as safe. Try again in a few minutes."
	verdictTimeout     = "The breach list did not answer within 10 seconds, so nothing was checked. That is not the same as safe. Something on this network may be blocking api.pwnedpasswords.com."
)

// Result is one breach check. It is returned even when the check could not be
// made, so the UI can say "we could not check" rather than showing nothing and
// letting a tech read that as "safe". Every field is a scalar, so there is no
// nil-slice-marshals-to-null problem here.
type Result struct {
	// Checked is false when the service could not be reached or refused. Found
	// and Count mean nothing in that case.
	Checked bool `json:"checked"`
	// Found is true when this exact password appears in the breach corpus.
	Found bool `json:"found"`
	// Count is how many times the password appears across the leaked datasets.
	// 0 when Found is false or Checked is false.
	Count int `json:"count"`
	// Prefix is the five hex characters that were sent, so a tech can see for
	// themselves how little left the machine.
	Prefix string `json:"prefix"`
	// Compared is how many real fingerprints came back and were compared here.
	// Padding entries are not counted.
	Compared int `json:"compared"`
	// Level is "ok", "danger" or "unknown", and colours the verdict card.
	Level string `json:"level"`
	// Verdict is one sentence a junior can read out loud.
	Verdict string `json:"verdict"`
	// CheckedAt is RFC3339 local time, so a screenshot is self-dating.
	CheckedAt string `json:"checkedAt"`
}

// Client is the range-request client. The fields exist so the tests can point it
// at a local httptest server with a short timeout; nothing in the UI sets them.
type Client struct {
	HTTP *http.Client
	// Base is the range endpoint with its trailing slash.
	Base string
	// Timeout is the budget for the whole request.
	Timeout time.Duration
}

// DefaultClient is the client the bound method uses.
func DefaultClient() *Client {
	return &Client{
		HTTP: &http.Client{
			// No Jar: the range endpoint has no reason to set a cookie and CHIT
			// has no reason to keep one.
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				TLSHandshakeTimeout: 5 * time.Second,
				ForceAttemptHTTP2:   true,
			},
		},
		Base:    rangeBase,
		Timeout: 10 * time.Second,
	}
}

// HashParts returns the uppercase SHA-1 hex of password, split into the five
// character prefix that is sent and the thirty-five character suffix that stays
// here. password is hashed exactly as typed: leading and trailing spaces are
// part of a password, so nothing is trimmed.
func HashParts(password string) (prefix, suffix string) {
	sum := sha1.Sum([]byte(password))
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	return h[:5], h[5:]
}

// FindSuffix scans a range response for suffix and returns how many times that
// password was seen, and how many real entries were compared. A padded entry has
// a count of 0 and is ignored, which is what the padding is for: a padded hit
// must read as a miss.
func FindSuffix(body, suffix string) (count, compared int) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		left, right, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		left = strings.ToUpper(strings.TrimSpace(left))
		if len(left) != 35 {
			continue
		}
		countText, _, _ := strings.Cut(right, ":")
		n, err := strconv.Atoi(strings.TrimSpace(countText))
		if err != nil || n <= 0 {
			continue
		}
		compared++
		if left == suffix {
			count = n
		}
		// The loop runs on past a match so compared is the true number of real
		// fingerprints, which is the privacy evidence shown on the page.
	}
	return count, compared
}

// Check hashes password here, sends the first five characters of the hash, and
// compares the rest against the response without it leaving this machine.
func (c *Client) Check(ctx context.Context, password string) (Result, error) {
	if password == "" {
		return Result{}, core.Errorf(core.CodeInvalidInput,
			"Type a password first. Only the first five characters of its fingerprint are ever sent.")
	}

	prefix, suffix := HashParts(password)

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+prefix, nil)
	if err != nil {
		return unknown(prefix, verdictUnreachable), nil
	}
	req.Header.Set("User-Agent", userAgent)
	// Add-Padding makes the service pad the reply with fake zero-count entries,
	// so even the size of the reply says nothing about which prefix was asked for.
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("Accept", "text/plain")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return unknown(prefix, failureVerdict(ctx, err)), nil
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		return unknown(prefix, verdictRefused), nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	resp.Body.Close()
	if err != nil {
		return unknown(prefix, failureVerdict(ctx, err)), nil
	}

	count, compared := FindSuffix(string(body), suffix)
	// A genuine range reply always carries hundreds of entries, so an answer
	// that parsed to nothing is a captive portal or a proxy page, not a clean
	// password. It must never render as "not found".
	if compared == 0 {
		return unknown(prefix, verdictRefused), nil
	}

	result := Result{
		Checked:   true,
		Found:     count > 0,
		Count:     count,
		Prefix:    prefix,
		Compared:  compared,
		Level:     levelOK,
		Verdict:   verdictNotFound,
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	if result.Found {
		result.Level = levelDanger
		result.Verdict = foundVerdict(count)
	}
	return result, nil
}

func unknown(prefix, verdict string) Result {
	return Result{
		Prefix:    prefix,
		Level:     levelUnknown,
		Verdict:   verdict,
		CheckedAt: time.Now().Format(time.RFC3339),
	}
}

// failureVerdict tells a request that ran out of time apart from one that never
// got anywhere, because the two need different things from the tech.
func failureVerdict(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return verdictTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return verdictTimeout
	}
	return verdictUnreachable
}

func foundVerdict(count int) string {
	if count == 1 {
		return "This password has appeared 1 time in a public data breach. Attackers already have it on a list. Change it everywhere it is used."
	}
	return fmt.Sprintf(
		"This password has appeared %s times in public data breaches. Attackers already have it on a list. Change it everywhere it is used.",
		groupDigits(count))
}

// groupDigits puts a comma every three digits, because "83000 times" reads as a
// smaller number than "83,000 times" and the size is the point.
func groupDigits(n int) string {
	s := strconv.Itoa(n)
	lead := len(s) % 3
	if lead == 0 {
		lead = 3
	}
	if lead >= len(s) {
		return s
	}
	var b strings.Builder
	b.WriteString(s[:lead])
	for i := lead; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
