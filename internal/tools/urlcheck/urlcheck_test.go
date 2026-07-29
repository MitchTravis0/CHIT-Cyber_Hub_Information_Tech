package urlcheck

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"chit/internal/core"
)

// testClient is DefaultClient without the blockPrivate hook, because every
// httptest server listens on a loopback address that the hook would refuse. The
// block itself is tested directly in TestBlockPrivate.
func testClient() *Client {
	return &Client{
		HTTP: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				DisableKeepAlives: true,
				// httptest's certificate is self-signed; this client only ever talks to the test server
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		MaxHops: maxHops,
	}
}

// hasFinding reports whether the report carries a finding with this id, so a
// test can assert on the one it cares about without listing the ones that an
// httptest server unavoidably produces.
func hasFinding(r Report, id string) bool {
	_, ok := findingByID(r.Findings, id)
	return ok
}

// inspect runs one inspection against a test server. The age lookup is off, so
// nothing in this file can reach anything but the loopback server it started.
func inspect(t *testing.T, c *Client, target string) Report {
	t.Helper()
	report, err := c.Inspect(context.Background(), Params{URL: target, SkipAge: true})
	if err != nil {
		t.Fatalf("Inspect(%q) returned an error: %v", target, err)
	}
	return report
}

func TestInspectSingleHop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := inspect(t, testClient(), srv.URL)

	if len(r.Hops) != 1 {
		t.Fatalf("got %d hops, want 1", len(r.Hops))
	}
	hop := r.Hops[0]
	if hop.N != 1 || hop.Method != http.MethodHead || hop.Status != 200 || hop.Next != "" {
		t.Errorf("hop = %+v", hop)
	}
	if hop.HeadRejected {
		t.Error("HEAD was reported as rejected")
	}
	if r.Final != r.Start {
		t.Errorf("final %q != start %q", r.Final, r.Start)
	}
	if r.Stopped != "" {
		t.Errorf("stopped = %q, want empty", r.Stopped)
	}
}

func TestInspectChainOfThree(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/b", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/c", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/c", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := inspect(t, testClient(), srv.URL+"/a")

	if len(r.Hops) != 3 {
		t.Fatalf("got %d hops, want 3", len(r.Hops))
	}
	for i, hop := range r.Hops {
		if hop.N != i+1 {
			t.Errorf("hop %d has N = %d", i, hop.N)
		}
		if i < 2 && hop.Next != r.Hops[i+1].URL {
			t.Errorf("hop %d points at %q but hop %d requested %q", i+1, hop.Next, i+2, r.Hops[i+1].URL)
		}
	}
	if !strings.HasSuffix(r.Final, "/c") {
		t.Errorf("final = %q, want it to end at /c", r.Final)
	}
}

func TestInspectLoop(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/b", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/a", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := inspect(t, testClient(), srv.URL+"/a")

	if len(r.Hops) != 2 {
		t.Fatalf("got %d hops, want 2", len(r.Hops))
	}
	if !hasFinding(r, "redirect-loop") {
		t.Error("redirect-loop did not fire")
	}
	if r.Stopped != "The link redirects back to an address it has already visited, so it goes round in a circle." {
		t.Errorf("stopped = %q", r.Stopped)
	}
}

func TestInspectHopCap(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/"+string(rune('a'+n.Add(1))), http.StatusFound)
	}))
	defer srv.Close()

	c := testClient()
	c.MaxHops = 3
	r := inspect(t, c, srv.URL+"/start")

	if len(r.Hops) != 3 {
		t.Fatalf("got %d hops, want 3", len(r.Hops))
	}
	if !hasFinding(r, "hop-cap") {
		t.Error("hop-cap did not fire")
	}
	if !strings.Contains(r.Stopped, "3 hops") {
		t.Errorf("stopped = %q, want it to name 3 hops", r.Stopped)
	}
}

func TestInspect3xxWithNoLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	r := inspect(t, testClient(), srv.URL)

	if len(r.Hops) != 1 {
		t.Fatalf("got %d hops, want 1", len(r.Hops))
	}
	if !hasFinding(r, "no-location") {
		t.Error("no-location did not fire")
	}
	want := "That address said it was redirecting but did not say where to, so the chain stops here."
	if r.Hops[0].Error != want {
		t.Errorf("hop error = %q, want %q", r.Hops[0].Error, want)
	}
}

func TestInspectRelativeLocation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/next")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := inspect(t, testClient(), srv.URL+"/start")

	if len(r.Hops) != 2 {
		t.Fatalf("got %d hops, want 2", len(r.Hops))
	}
	if r.Hops[0].Location != "/next" {
		t.Errorf("location = %q, want the header as sent", r.Hops[0].Location)
	}
	if r.Hops[0].Next != srv.URL+"/next" {
		t.Errorf("next = %q, want %q", r.Hops[0].Next, srv.URL+"/next")
	}
	if r.Hops[1].URL != srv.URL+"/next" {
		t.Errorf("hop 2 requested %q", r.Hops[1].URL)
	}
}

func TestInspectSchemeDowngrade(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/landed", http.StatusFound)
	}))
	defer secure.Close()

	r := inspect(t, testClient(), secure.URL)

	if !hasFinding(r, "downgrade") {
		t.Error("downgrade did not fire")
	}
	if !hasFinding(r, "insecure") {
		t.Error("insecure did not fire")
	}
	if !strings.HasPrefix(r.Final, "http://") {
		t.Errorf("final = %q, want a plain http address", r.Final)
	}
}

func TestInspectJavascriptTarget(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Location", "javascript:alert(1)")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	r := inspect(t, testClient(), srv.URL)

	if len(r.Hops) != 1 {
		t.Fatalf("got %d hops, want 1", len(r.Hops))
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("the server was asked %d times, want 1", got)
	}
	if r.Hops[0].Next != "javascript:alert(1)" {
		t.Errorf("next = %q", r.Hops[0].Next)
	}
	finding, ok := findingByID(r.Findings, "bad-scheme")
	if !ok {
		t.Fatal("bad-scheme did not fire")
	}
	want := "The link redirects to a javascript: address, which is code rather than a web page. CHIT did not run it. Nothing legitimate does this."
	if finding.Text != want {
		t.Errorf("text = %q, want %q", finding.Text, want)
	}
	if r.Stopped != "The chain was not followed past that point, because the next address is not a web address." {
		t.Errorf("stopped = %q", r.Stopped)
	}
}

func TestInspectDataAndFileTargets(t *testing.T) {
	cases := []struct {
		location string
		want     string
	}{
		{
			"data:text/html,<h1>hi",
			"The link redirects to a data: address, which carries the whole page inside the link itself. That is how a fake login page is hidden from a mail filter. CHIT did not open it.",
		},
		{
			"file:///etc/passwd",
			"The link redirects to a file: address, which points at a file on the computer rather than at a website. CHIT did not open it.",
		},
		{
			"ms-msdt:x",
			"The link redirects to \"ms-msdt:\", which is not a web address. CHIT did not open it. A link that hands off to another program on the machine is worth asking about.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.location, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", tc.location)
				w.WriteHeader(http.StatusFound)
			}))
			defer srv.Close()

			r := inspect(t, testClient(), srv.URL)

			if len(r.Hops) != 1 {
				t.Fatalf("got %d hops, want 1", len(r.Hops))
			}
			finding, ok := findingByID(r.Findings, "bad-scheme")
			if !ok {
				t.Fatal("bad-scheme did not fire")
			}
			if finding.Text != tc.want {
				t.Errorf("text = %q, want %q", finding.Text, tc.want)
			}
		})
	}
}

func TestInspectHeadRejectedThenGet(t *testing.T) {
	for _, status := range []int{405, 403, 501} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					w.WriteHeader(status)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			r := inspect(t, testClient(), srv.URL)

			if len(r.Hops) != 1 {
				t.Fatalf("got %d hops, want 1", len(r.Hops))
			}
			hop := r.Hops[0]
			if hop.Method != http.MethodGet || !hop.HeadRejected || hop.Status != 200 {
				t.Errorf("hop = %+v", hop)
			}
		})
	}

	t.Run("404 is a real answer", func(t *testing.T) {
		var gets atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				gets.Add(1)
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		r := inspect(t, testClient(), srv.URL)

		if gets.Load() != 0 {
			t.Errorf("a GET was sent after a 404 to HEAD")
		}
		if r.Hops[0].Method != http.MethodHead || r.Hops[0].Status != 404 {
			t.Errorf("hop = %+v", r.Hops[0])
		}
	})
}

func TestInspectCredentialsInURL(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := strings.Replace(srv.URL, "http://", "http://microsoft.com@", 1)
	r := inspect(t, testClient(), target)

	if !hasFinding(r, "credentials") {
		t.Error("credentials did not fire")
	}
	if requests.Load() != 1 {
		t.Errorf("the test server was asked %d times, want 1", requests.Load())
	}
}

func TestInspectUnwrapsBeforeRequesting(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wrapped := "https://eur02.safelinks.protection.outlook.com/?url=" + url.QueryEscape(srv.URL+"/login")
	r := inspect(t, testClient(), wrapped)

	if len(r.Unwrapped) != 1 {
		t.Fatalf("got %d unwrap steps, want 1", len(r.Unwrapped))
	}
	if r.Unwrapped[0].Wrapper != "Microsoft Defender Safe Links" {
		t.Errorf("wrapper = %q", r.Unwrapped[0].Wrapper)
	}
	if len(r.Hops) != 1 {
		t.Fatalf("got %d hops, want 1", len(r.Hops))
	}
	if r.Hops[0].URL != srv.URL+"/login" {
		t.Errorf("hop 1 requested %q, want %q", r.Hops[0].URL, srv.URL+"/login")
	}
	if requests.Load() != 1 {
		t.Errorf("the test server was asked %d times, want 1", requests.Load())
	}
}

func TestInspectRejectsBadParams(t *testing.T) {
	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{
			name:   "empty",
			params: Params{URL: "   "},
			want:   "Paste a link to inspect, for example https://example.com/login.",
		},
		{
			name:   "not a web address",
			params: Params{URL: "ftp://example.com"},
			want:   "\"ftp://example.com\" is not a web address CHIT can follow. Paste a link that starts with http:// or https://.",
		},
		{
			name:   "wait too long",
			params: Params{URL: "https://example.com", TimeoutMS: 60001},
			want:   "The wait must be between 1 and 60 seconds. 60001 ms is outside that.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testClient().Inspect(context.Background(), tc.params)
			if err == nil {
				t.Fatal("no error")
			}
			if core.CodeOf(err) != core.CodeInvalidInput {
				t.Errorf("code = %q, want %q", core.CodeOf(err), core.CodeInvalidInput)
			}
			if got := core.MessageOf(err); got != tc.want {
				t.Errorf("message = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("no wait means the default", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		report, err := testClient().Inspect(context.Background(), Params{URL: srv.URL, SkipAge: true})
		if err != nil {
			t.Fatalf("Inspect returned an error: %v", err)
		}
		if len(report.Hops) != 1 || report.Hops[0].Status != 200 {
			t.Errorf("hops = %+v", report.Hops)
		}
	})
}

func TestInspectNoCookiesNoReferer(t *testing.T) {
	type seen struct {
		cookie    string
		referer   string
		userAgent string
	}
	headers := make(chan seen, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("/one", func(w http.ResponseWriter, r *http.Request) {
		headers <- seen{r.Header.Get("Cookie"), r.Header.Get("Referer"), r.Header.Get("User-Agent")}
		http.SetCookie(w, &http.Cookie{Name: "track", Value: "me"})
		http.Redirect(w, r, "/two", http.StatusFound)
	})
	mux.HandleFunc("/two", func(w http.ResponseWriter, r *http.Request) {
		headers <- seen{r.Header.Get("Cookie"), r.Header.Get("Referer"), r.Header.Get("User-Agent")}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := inspect(t, testClient(), srv.URL+"/one")
	if len(r.Hops) != 2 {
		t.Fatalf("got %d hops, want 2", len(r.Hops))
	}

	close(headers)
	count := 0
	for got := range headers {
		count++
		if got.cookie != "" {
			t.Errorf("request %d carried a Cookie header: %q", count, got.cookie)
		}
		if got.referer != "" {
			t.Errorf("request %d carried a Referer header: %q", count, got.referer)
		}
		if got.userAgent != userAgent {
			t.Errorf("request %d sent User-Agent %q, want %q", count, got.userAgent, userAgent)
		}
	}
	if count != 2 {
		t.Fatalf("the server saw %d requests, want 2", count)
	}
}

func TestInspectAlwaysReturnsInitialisedSlices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := inspect(t, testClient(), srv.URL)
	if r.Unwrapped == nil || r.Hops == nil || r.Findings == nil {
		t.Fatalf("a nil slice escaped: unwrapped=%v hops=%v findings=%v", r.Unwrapped, r.Hops, r.Findings)
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// A loopback server is a bare IP on an unusual port over plain http, so the
	// findings list is never empty here. The unwrapped list is.
	if !strings.Contains(string(data), `"unwrapped":[]`) {
		t.Errorf("unwrapped marshalled as null: %s", data)
	}
	if strings.Contains(string(data), `"findings":null`) || strings.Contains(string(data), `"hops":null`) {
		t.Errorf("a slice marshalled as null: %s", data)
	}
}

func TestInspectContextCancelled(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := testClient().Inspect(ctx, Params{URL: srv.URL, SkipAge: true}); err == nil {
		t.Fatal("a cancelled context produced no error")
	}
	if requests.Load() != 0 {
		t.Errorf("the server was asked %d times, want 0", requests.Load())
	}
}

func TestNormalizeInput(t *testing.T) {
	ok := []struct {
		raw  string
		want string
	}{
		{"example.com", "https://example.com/"},
		{"HTTP://Example.COM/Path", "http://example.com/Path"},
		{"  https://example.com  ", "https://example.com/"},
		{"https://example.com?a=B", "https://example.com/?a=B"},
	}
	for _, tc := range ok {
		got, err := NormalizeInput(tc.raw)
		if err != nil {
			t.Errorf("NormalizeInput(%q) returned an error: %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizeInput(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}

	bad := []string{"ftp://x", "", "   ", "javascript:alert(1)", "https://"}
	for _, raw := range bad {
		if got, err := NormalizeInput(raw); err == nil {
			t.Errorf("NormalizeInput(%q) = %q, want an error", raw, got)
		}
	}
}
