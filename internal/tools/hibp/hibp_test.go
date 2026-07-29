package hibp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

// The SHA-1 of "password" is 5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8, so the
// range endpoint sees 5BAA6 and everything below carries the 35 characters that
// stay on this machine.
const passwordSuffix = "1E4C9B93F3F0682250B6CF8331B7EE68FD8"

// hitBody is a range reply in the shape the service really sends: uppercase
// suffix, a colon, a count, CRLF line endings. The target is the third entry.
const hitBody = "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:3\r\n" +
	"00D4F6E8FA6EECAD2A3AA415EEC418D38EC:17\r\n" +
	passwordSuffix + ":42\r\n" +
	"012A7CA357541F0AC487871FEEC1891C49C:5\r\n" +
	"0136E006E24E7D152139815FB0FC6A50B15:1\r\n"

// missBody is the same five entries with the target replaced, plus two padded
// entries that a real Add-Padding reply always carries.
const missBody = "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:3\r\n" +
	"00D4F6E8FA6EECAD2A3AA415EEC418D38EC:17\r\n" +
	"011053FD0102E94D6AE2F8B83D76FAF94F6:9\r\n" +
	"012A7CA357541F0AC487871FEEC1891C49C:5\r\n" +
	"0136E006E24E7D152139815FB0FC6A50B15:1\r\n" +
	"0141C4A63A1CA2C1F7A5B54FC7A1C2E4D77:0\r\n" +
	"0152B9E8D3F4A1C6B2E7D8F9A0C1B2E3D44:0\r\n"

const verdict42 = "This password has appeared 42 times in public data breaches. Attackers already have it on a list. Change it everywhere it is used."

// testClient points a Client at srv with a budget short enough that a hung test
// fails instead of hanging.
func testClient(srv *httptest.Server) *Client {
	return &Client{HTTP: srv.Client(), Base: srv.URL + "/range/", Timeout: 5 * time.Second}
}

// bodyServer answers every request with body and records what it was sent.
func bodyServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request, *int) {
	t.Helper()
	var last http.Request
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		last = *r
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &last, &calls
}

func TestHashParts(t *testing.T) {
	tests := []struct {
		name       string
		password   string
		wantPrefix string
		wantSuffix string
	}{
		{"the classic", "password", "5BAA6", passwordSuffix},
		{"empty is still hashable", "", "DA39A", "3EE5E6B4B0D3255BFEF95601890AFD80709"},
		{"leetspeak", "P@ssw0rd", "21BD1", "2DC183F740EE76F27B78EB39C8AD972A757"},
		{"spaces are part of the password", " pass ", "EE57A", "8B3C1D252E71A6D6D299CA1C189B32B737D"},
		{"utf-8 bytes are hashed", "pässwörd", "F517D", "DF1D32A112FF1AD55C66D1B12CB38E7E8F7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, suffix := HashParts(tt.password)
			if prefix != tt.wantPrefix {
				t.Errorf("prefix = %q, want %q", prefix, tt.wantPrefix)
			}
			if suffix != tt.wantSuffix {
				t.Errorf("suffix = %q, want %q", suffix, tt.wantSuffix)
			}
			if len(prefix) != 5 {
				t.Errorf("prefix %q is %d characters, want 5", prefix, len(prefix))
			}
			if len(suffix) != 35 {
				t.Errorf("suffix %q is %d characters, want 35", suffix, len(suffix))
			}
			if prefix != strings.ToUpper(prefix) || suffix != strings.ToUpper(suffix) {
				t.Errorf("prefix/suffix %q/%q are not uppercase", prefix, suffix)
			}
		})
	}
}

func TestFindSuffixHit(t *testing.T) {
	count, compared := FindSuffix(hitBody, passwordSuffix)
	if count != 42 {
		t.Errorf("count = %d, want 42", count)
	}
	if compared != 5 {
		t.Errorf("compared = %d, want 5", compared)
	}
}

func TestFindSuffixMiss(t *testing.T) {
	count, compared := FindSuffix(hitBody, "0000000000000000000000000000000000A")
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if compared != 5 {
		t.Errorf("compared = %d, want 5", compared)
	}
}

func TestFindSuffixIgnoresPadding(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantCount    int
		wantCompared int
	}{
		{
			name:         "a padded hit reads as a miss",
			body:         "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:3\r\n" + passwordSuffix + ":0\r\n",
			wantCount:    0,
			wantCompared: 1,
		},
		{
			name: "real and padded entries side by side",
			body: "0141C4A63A1CA2C1F7A5B54FC7A1C2E4D77:0\r\n" +
				passwordSuffix + ":8\r\n" +
				"0152B9E8D3F4A1C6B2E7D8F9A0C1B2E3D44:0\r\n" +
				"0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:2\r\n" +
				"0136E006E24E7D152139815FB0FC6A50B15:0\r\n",
			wantCount:    8,
			wantCompared: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, compared := FindSuffix(tt.body, passwordSuffix)
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
			if compared != tt.wantCompared {
				t.Errorf("compared = %d, want %d", compared, tt.wantCompared)
			}
		})
	}
}

func TestFindSuffixLineEndings(t *testing.T) {
	lf := strings.ReplaceAll(hitBody, "\r\n", "\n")
	tests := []struct {
		name string
		body string
	}{
		{"crlf", hitBody},
		{"lf only", lf},
		{"crlf with a trailing blank line", hitBody + "\r\n"},
		{"no trailing newline", strings.TrimSuffix(lf, "\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, compared := FindSuffix(tt.body, passwordSuffix)
			if count != 42 || compared != 5 {
				t.Errorf("got (%d, %d), want (42, 5)", count, compared)
			}
		})
	}
}

func TestFindSuffixTolerantParsing(t *testing.T) {
	tests := []struct {
		name string
		bad  string
	}{
		{"no colon", "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0 3"},
		{"count is not a number", "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:abc"},
		{"suffix is 34 characters", "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3:7"},
		{"suffix is 36 characters", "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0A:7"},
		{"only whitespace", "   "},
		{"a third field", "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:7:extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.bad + "\r\n" + passwordSuffix + ":42\r\n"
			count, _ := FindSuffix(body, passwordSuffix)
			if count != 42 {
				t.Errorf("count = %d, want 42: a good entry after a bad line must still be found", count)
			}
		})
	}
}

func TestFindSuffixCountsAThirdFieldLine(t *testing.T) {
	// A line with a third field is not malformed, it is a future addition, so
	// only the first field after the colon is read and the entry still counts.
	body := "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:7:extra\r\n"
	count, compared := FindSuffix(body, "0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0")
	if count != 7 || compared != 1 {
		t.Errorf("got (%d, %d), want (7, 1)", count, compared)
	}
}

func TestCheckHit(t *testing.T) {
	srv, last, _ := bodyServer(t, http.StatusOK, hitBody)

	result, err := testClient(srv).Check(context.Background(), "password")
	if err != nil {
		t.Fatalf("Check returned %v", err)
	}

	if last.URL.Path != "/range/5BAA6" {
		t.Errorf("path = %q, want %q", last.URL.Path, "/range/5BAA6")
	}
	if got := last.Header.Get("User-Agent"); got != "CHIT-Password-Checker" {
		t.Errorf("User-Agent = %q, want %q", got, "CHIT-Password-Checker")
	}
	if got := last.Header.Get("Add-Padding"); got != "true" {
		t.Errorf("Add-Padding = %q, want %q", got, "true")
	}
	if got := last.Header.Get("Cookie"); got != "" {
		t.Errorf("Cookie = %q, want no cookie header at all", got)
	}
	if !result.Checked {
		t.Error("Checked = false, want true")
	}
	if !result.Found {
		t.Error("Found = false, want true")
	}
	if result.Count != 42 {
		t.Errorf("Count = %d, want 42", result.Count)
	}
	if result.Prefix != "5BAA6" {
		t.Errorf("Prefix = %q, want %q", result.Prefix, "5BAA6")
	}
	if result.Compared != 5 {
		t.Errorf("Compared = %d, want 5", result.Compared)
	}
	if result.Level != "danger" {
		t.Errorf("Level = %q, want %q", result.Level, "danger")
	}
	if result.Verdict != verdict42 {
		t.Errorf("Verdict = %q, want %q", result.Verdict, verdict42)
	}
	if _, err := time.Parse(time.RFC3339, result.CheckedAt); err != nil {
		t.Errorf("CheckedAt = %q, which does not parse as RFC3339: %v", result.CheckedAt, err)
	}
}

func TestCheckMiss(t *testing.T) {
	srv, _, _ := bodyServer(t, http.StatusOK, missBody)

	result, err := testClient(srv).Check(context.Background(), "password")
	if err != nil {
		t.Fatalf("Check returned %v", err)
	}
	if !result.Checked {
		t.Error("Checked = false, want true")
	}
	if result.Found {
		t.Error("Found = true, want false")
	}
	if result.Count != 0 {
		t.Errorf("Count = %d, want 0", result.Count)
	}
	if result.Compared != 5 {
		t.Errorf("Compared = %d, want 5: the two padded entries must not be counted", result.Compared)
	}
	if result.Level != "ok" {
		t.Errorf("Level = %q, want %q", result.Level, "ok")
	}
	want := "This password was not found in any of the leaked password lists. That does not make it a strong password, so read the strength rating above as well."
	if result.Verdict != want {
		t.Errorf("Verdict = %q, want %q", result.Verdict, want)
	}
}

// TestResultLeaksNothing uses a distinctive password rather than "password",
// because the verdict sentence itself is required to contain the word
// "password" and a literal search for it would always match the copy.
func TestResultLeaksNothing(t *testing.T) {
	const secret = "Tr0ub4dor&3"
	const secretPrefix = "87457"
	const secretSuffix = "2E7A5AE6A49466A6AC578B98ADBA78C6AA6"

	srv, _, _ := bodyServer(t, http.StatusOK, secretSuffix+":42\r\n"+
		"0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:3\r\n")

	result, err := testClient(srv).Check(context.Background(), secret)
	if err != nil {
		t.Fatalf("Check returned %v", err)
	}
	if !result.Found || result.Level != "danger" {
		t.Fatalf("Found/Level = %v/%q, want true/danger so the danger verdict is the one inspected",
			result.Found, result.Level)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal returned %v", err)
	}

	text := strings.ToUpper(string(encoded))
	if strings.Contains(text, strings.ToUpper(secret)) {
		t.Errorf("the marshalled result contains the password: %s", encoded)
	}
	if strings.Contains(text, secretPrefix+secretSuffix) {
		t.Errorf("the marshalled result contains the full hash: %s", encoded)
	}
	if strings.Contains(text, secretSuffix) {
		t.Errorf("the marshalled result contains the hash suffix: %s", encoded)
	}
}

func TestVerdictCounts(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{1, "This password has appeared 1 time in a public data breach. Attackers already have it on a list. Change it everywhere it is used."},
		{2, "This password has appeared 2 times in public data breaches. Attackers already have it on a list. Change it everywhere it is used."},
		{999, "This password has appeared 999 times in public data breaches. Attackers already have it on a list. Change it everywhere it is used."},
		{1000, "This password has appeared 1,000 times in public data breaches. Attackers already have it on a list. Change it everywhere it is used."},
		{83000, "This password has appeared 83,000 times in public data breaches. Attackers already have it on a list. Change it everywhere it is used."},
		{1234567, "This password has appeared 1,234,567 times in public data breaches. Attackers already have it on a list. Change it everywhere it is used."},
	}

	for _, tt := range tests {
		t.Run(strings.TrimSpace(groupDigits(tt.count)), func(t *testing.T) {
			if got := foundVerdict(tt.count); got != tt.want {
				t.Errorf("foundVerdict(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

func TestGroupDigits(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{12, "12"},
		{123, "123"},
		{1000, "1,000"},
		{10000, "10,000"},
		{999999, "999,999"},
		{1000000, "1,000,000"},
		{1234567, "1,234,567"},
	}

	for _, tt := range tests {
		if got := groupDigits(tt.in); got != tt.want {
			t.Errorf("groupDigits(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCheckNon200(t *testing.T) {
	srv, _, _ := bodyServer(t, http.StatusServiceUnavailable, "service unavailable")

	result, err := testClient(srv).Check(context.Background(), "password")
	if err != nil {
		t.Fatalf("Check returned %v, want no error for a refused request", err)
	}
	if result.Checked {
		t.Error("Checked = true, want false")
	}
	if result.Level != "unknown" {
		t.Errorf("Level = %q, want %q", result.Level, "unknown")
	}
	want := "The breach list refused the request, so nothing was checked. That is not the same as safe. Try again in a few minutes."
	if result.Verdict != want {
		t.Errorf("Verdict = %q, want %q", result.Verdict, want)
	}
}

func TestCheckUnreachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen returned %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	c := &Client{HTTP: &http.Client{}, Base: "http://" + addr + "/range/", Timeout: 5 * time.Second}
	result, err := c.Check(context.Background(), "password")
	if err != nil {
		t.Fatalf("Check returned %v, want no error for an unreachable service", err)
	}
	if result.Checked {
		t.Error("Checked = true, want false")
	}
	if result.Level != "unknown" {
		t.Errorf("Level = %q, want %q", result.Level, "unknown")
	}
	want := "The breach list could not be reached, so nothing was checked. That is not the same as safe. Check this machine's internet connection and try again."
	if result.Verdict != want {
		t.Errorf("Verdict = %q, want %q", result.Verdict, want)
	}
}

func TestCheckTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	c := &Client{HTTP: srv.Client(), Base: srv.URL + "/range/", Timeout: 50 * time.Millisecond}
	result, err := c.Check(context.Background(), "password")
	if err != nil {
		t.Fatalf("Check returned %v, want no error for a timeout", err)
	}
	if result.Checked {
		t.Error("Checked = true, want false")
	}
	if result.Level != "unknown" {
		t.Errorf("Level = %q, want %q", result.Level, "unknown")
	}
	want := "The breach list did not answer within 10 seconds, so nothing was checked. That is not the same as safe. Something on this network may be blocking api.pwnedpasswords.com."
	if result.Verdict != want {
		t.Errorf("Verdict = %q, want %q", result.Verdict, want)
	}
}

func TestCheckEmptyPassword(t *testing.T) {
	srv, _, calls := bodyServer(t, http.StatusOK, hitBody)

	_, err := testClient(srv).Check(context.Background(), "")
	if err == nil {
		t.Fatal("Check returned no error for an empty password")
	}
	want := "Type a password first. Only the first five characters of its fingerprint are ever sent."
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
	if code := core.CodeOf(err); code != core.CodeInvalidInput {
		t.Errorf("code = %q, want %q", code, core.CodeInvalidInput)
	}
	if *calls != 0 {
		t.Errorf("the server was called %d times, want 0: nothing may be sent for an empty password", *calls)
	}
}

func TestCheckEmptyRangeReadsAsUnknown(t *testing.T) {
	refused := "The breach list refused the request, so nothing was checked. That is not the same as safe. Try again in a few minutes."
	notFound := "This password was not found in any of the leaked password lists. That does not make it a strong password, so read the strength rating above as well."

	tests := []struct {
		name string
		body string
	}{
		{
			name: "only padded entries",
			body: "0141C4A63A1CA2C1F7A5B54FC7A1C2E4D77:0\r\n" +
				"0152B9E8D3F4A1C6B2E7D8F9A0C1B2E3D44:0\r\n" +
				"0018A45C4D5C0A9AA7B6C0C1E4C9B93F3F0:0\r\n",
		},
		{
			name: "a captive portal answering 200 with html",
			body: "<html><body>Sign in to use the guest network</body></html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _, _ := bodyServer(t, http.StatusOK, tt.body)
			result, err := testClient(srv).Check(context.Background(), "password")
			if err != nil {
				t.Fatalf("Check returned %v", err)
			}
			if result.Level != "unknown" {
				t.Errorf("Level = %q, want %q", result.Level, "unknown")
			}
			if result.Verdict == notFound {
				t.Error("Verdict reads as a clean password, want the refused sentence")
			}
			if result.Verdict != refused {
				t.Errorf("Verdict = %q, want %q", result.Verdict, refused)
			}
		})
	}
}

func TestCheckCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &Client{HTTP: srv.Client(), Base: srv.URL + "/range/", Timeout: 5 * time.Second}
	started := time.Now()
	result, err := c.Check(ctx, "password")
	if err != nil {
		t.Fatalf("Check returned %v, want no error for a cancelled context", err)
	}
	if result.Checked {
		t.Error("Checked = true, want false")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("Check took %s, want it to give up at once", elapsed)
	}
}
