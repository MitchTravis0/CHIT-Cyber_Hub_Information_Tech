package urlcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const rdapBody = `{
  "objectClassName": "domain",
  "ldhName": "EXAMPLE.COM",
  "events": [
    {"eventAction": "registration", "eventDate": "2026-07-14T10:00:00Z"},
    {"eventAction": "last changed", "eventDate": "2026-07-20T10:00:00Z"}
  ]
}`

func TestParseRDAP(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"registration event", rdapBody, "2026-07-14"},
		{"action in another case", `{"events":[{"eventAction":"Registration","eventDate":"2019-03-05T00:00:00Z"}]}`, "2019-03-05"},
		{"date only", `{"events":[{"eventAction":"registration","eventDate":"2019-03-05"}]}`, "2019-03-05"},
		{"offset without a colon", `{"events":[{"eventAction":"registration","eventDate":"2019-03-05T00:00:00+0000"}]}`, "2019-03-05"},
		{"no registration event", `{"events":[{"eventAction":"expiration","eventDate":"2019-03-05T00:00:00Z"}]}`, ""},
		{"unparseable date", `{"events":[{"eventAction":"registration","eventDate":"March 2019"}]}`, ""},
		{"not json", `<html>not found</html>`, ""},
		{"empty body", ``, ""},
		{"null events", `{"events":null}`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseRDAP([]byte(tc.body))
			if tc.want == "" {
				if ok {
					t.Errorf("parseRDAP reported %s, want no answer", got.Format("2006-01-02"))
				}
				return
			}
			if !ok {
				t.Fatal("parseRDAP found no registration date")
			}
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("parseRDAP = %s, want %s", got.Format("2006-01-02"), tc.want)
			}
		})
	}
}

func TestLookupAge(t *testing.T) {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)

	t.Run("known", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/rdap+json")
			w.Write([]byte(rdapBody))
		}))
		defer srv.Close()

		c := testClient()
		c.RDAPBase = srv.URL + "/domain/"
		age := c.LookupAge(context.Background(), "example.com", now)

		if !age.Known {
			t.Fatalf("age = %+v, want it known", age)
		}
		if age.Registered != "2026-07-14" || age.Days != 12 || age.Human != "12 days" || age.Note != "" {
			t.Errorf("age = %+v", age)
		}
	})

	t.Run("follows the front end's redirect", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/domain/example.com", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/registry/example.com", http.StatusFound)
		})
		mux.HandleFunc("/registry/example.com", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(rdapBody))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := testClient()
		c.RDAPBase = srv.URL + "/domain/"
		age := c.LookupAge(context.Background(), "example.com", now)

		if !age.Known || age.Registered != "2026-07-14" {
			t.Errorf("age = %+v", age)
		}
	})

	unknown := []struct {
		name    string
		status  int
		body    string
		clockAt time.Time
	}{
		{"not covered", http.StatusNotFound, `{"errorCode":404}`, now},
		{"not json", http.StatusOK, `<html>hello</html>`, now},
		{"registered in the future", http.StatusOK, rdapBody, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range unknown {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c := testClient()
			c.RDAPBase = srv.URL + "/domain/"
			age := c.LookupAge(context.Background(), "example.com", tc.clockAt)

			if age.Known {
				t.Fatalf("age = %+v, want it unknown", age)
			}
			if age.Note != ageUnknown {
				t.Errorf("note = %q, want %q", age.Note, ageUnknown)
			}
		})
	}
}

func TestLookupAgeSkipped(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Write([]byte(rdapBody))
	}))
	defer srv.Close()

	c := testClient()
	c.RDAPBase = srv.URL + "/domain/"
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)

	skipped := c.age(context.Background(), Params{SkipAge: true}, HostName{Registrable: "example.com"}, now)
	if skipped.Known || skipped.Note != "Domain age was not looked up." {
		t.Errorf("age = %+v", skipped)
	}

	bare := c.age(context.Background(), Params{}, HostName{Raw: "93.184.216.34", IsIP: true}, now)
	if bare.Known || bare.Note != "A bare IP address has no registration date to look up." {
		t.Errorf("age = %+v", bare)
	}

	if requests.Load() != 0 {
		t.Errorf("the RDAP service was asked %d times, want 0", requests.Load())
	}
}

func TestBlockPrivate(t *testing.T) {
	blocked := []string{
		"127.0.0.1:80", "10.0.0.1:80", "192.168.1.1:443", "172.16.0.1:80",
		"169.254.1.1:80", "100.64.0.1:80", "0.0.0.0:80", "[::1]:80",
		"[fc00::1]:443", "[fe80::1]:80", "224.0.0.1:80",
	}
	for _, address := range blocked {
		err := blockPrivate("tcp", address, nil)
		if err == nil {
			t.Errorf("blockPrivate(%q) allowed it", address)
			continue
		}
		if !errors.Is(err, errBlockedAddress) {
			t.Errorf("blockPrivate(%q) returned %v, want errBlockedAddress", address, err)
		}
	}

	allowed := []string{
		"8.8.8.8:53", "93.184.216.34:443", "172.32.0.1:80", "100.128.0.1:80",
		"[2606:2800:220:1:248:1893:25c8:1946]:443",
	}
	for _, address := range allowed {
		if err := blockPrivate("tcp", address, nil); err != nil {
			t.Errorf("blockPrivate(%q) refused it: %v", address, err)
		}
	}
}
