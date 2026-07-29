package dnslook

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestQueryOutcome(t *testing.T) {
	cases := []struct {
		name        string
		count       int
		err         error
		wantStatus  string
		wantMessage string
	}{
		{"two answers", 2, nil, StatusOK, ""},
		{"no answers", 0, nil, StatusEmpty,
			"No MX record for example.com according to 8.8.8.8."},
		{"not found", 0, &net.DNSError{Err: "no such host", IsNotFound: true}, StatusEmpty,
			"No MX record for example.com according to 8.8.8.8."},
		{"timed out", 0, &net.DNSError{Err: "i/o timeout", IsTimeout: true}, StatusError,
			"8.8.8.8 did not answer within 3000 ms."},
		{"deadline exceeded", 0, context.DeadlineExceeded, StatusError,
			"8.8.8.8 did not answer within 3000 ms."},
		{"anything else", 0, errors.New("boom"), StatusError,
			"8.8.8.8 could not answer that question."},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, message := queryOutcome("8.8.8.8", "MX", "example.com", 3000, c.count, c.err)
			if status != c.wantStatus {
				t.Errorf("status = %q, want %q", status, c.wantStatus)
			}
			if message != c.wantMessage {
				t.Errorf("message = %q, want %q", message, c.wantMessage)
			}
			if strings.Contains(message, "boom") {
				t.Errorf("message %q leaks the raw error text", message)
			}
		})
	}
}

func TestQueryOutcomeNeverLeaksRawText(t *testing.T) {
	raw := []error{
		errors.New("lookup example.com on 127.0.0.53:53: i/o timeout"),
		errors.New("dial udp 8.8.8.8:53: connect: connection refused"),
		&net.DNSError{Err: "server misbehaving", Name: "example.com", Server: "8.8.8.8"},
		&net.OpError{Op: "read", Net: "udp", Err: errors.New("connection refused")},
		errors.New("no such host"),
	}
	leaks := []string{"lookup", "i/o timeout", "connection refused", "no such host"}

	for _, err := range raw {
		status, message := queryOutcome("8.8.8.8", "A", "example.com", 3000, 0, err)
		if status != StatusError {
			t.Errorf("status for %v = %q, want %q", err, status, StatusError)
		}
		for _, leak := range leaks {
			if strings.Contains(strings.ToLower(message), leak) {
				t.Errorf("message %q leaks %q", message, leak)
			}
		}
	}
}

// TestEverySupportedTypeIsHandled proves no record type falls through the
// dispatch switch. Nothing listens on 127.0.0.1:1, so every question fails
// fast and no test touches a real DNS server.
func TestEverySupportedTypeIsHandled(t *testing.T) {
	server := target{id: "127.0.0.1:1", label: "127.0.0.1:1", addr: "127.0.0.1:1"}

	for _, typ := range SupportedTypes {
		t.Run(typ, func(t *testing.T) {
			name := "chit.test"
			if typ == "PTR" {
				name = "192.0.2.1"
			}
			st := settings{name: name, timeout: 300 * time.Millisecond, timeoutMS: 300}

			records := ask(context.Background(), question{server: server, typ: typ}, st)
			if len(records) != 1 {
				t.Fatalf("got %d records, want exactly one", len(records))
			}
			r := records[0]
			if r.Status != StatusError {
				t.Errorf("status = %q, want %q", r.Status, StatusError)
			}
			if r.Message == "" {
				t.Error("an unreachable server produced no explanation")
			}
			if r.Type != typ || r.Name != name || r.Server != server.label {
				t.Errorf("record = %+v, want it to name the question it answers", r)
			}
		})
	}
}

func TestAskDropsEverythingWhenTheJobIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := target{id: "127.0.0.1:1", label: "127.0.0.1:1", addr: "127.0.0.1:1"}
	st := settings{name: "chit.test", timeout: 300 * time.Millisecond, timeoutMS: 300}

	if records := ask(ctx, question{server: server, typ: "A"}, st); records != nil {
		t.Errorf("a cancelled job produced %+v, want no records", records)
	}
}

func TestMXRecords(t *testing.T) {
	cases := []struct {
		name           string
		in             []*net.MX
		wantValues     []string
		wantPriorities []int
		wantNullMX     bool
	}{
		{"ordinary domain", []*net.MX{{Host: "mail.example.com.", Pref: 10}},
			[]string{"mail.example.com"}, []int{10}, false},
		{"several sorted by preference", []*net.MX{{Host: "a.example.com.", Pref: 5}, {Host: "b.example.com.", Pref: 20}},
			[]string{"a.example.com", "b.example.com"}, []int{5, 20}, false},
		{"null mx only", []*net.MX{{Host: ".", Pref: 0}}, nil, nil, true},
		{"empty host is the same null answer", []*net.MX{{Host: "", Pref: 0}}, nil, nil, true},
		{"null mx alongside a real one keeps the real one", []*net.MX{{Host: ".", Pref: 0}, {Host: "mail.example.com.", Pref: 10}},
			[]string{"mail.example.com"}, []int{10}, false},
		{"no records at all is not a null mx", nil, []string{}, []int{}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			values, priorities, err := mxRecords(c.in)
			if c.wantNullMX {
				if !errors.Is(err, errNullMX) {
					t.Fatalf("err = %v, want errNullMX", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if len(values) != len(c.wantValues) {
				t.Fatalf("values = %v, want %v", values, c.wantValues)
			}
			for i := range values {
				if values[i] != c.wantValues[i] {
					t.Errorf("values[%d] = %q, want %q", i, values[i], c.wantValues[i])
				}
				if priorities[i] != c.wantPriorities[i] {
					t.Errorf("priorities[%d] = %d, want %d", i, priorities[i], c.wantPriorities[i])
				}
			}
		})
	}
}

// A domain that publishes a null MX has an answer, so it must never render as a
// blank row marked ok. example.com is the live case that exposed this.
func TestQueryOutcomeNullMX(t *testing.T) {
	status, message := queryOutcome("8.8.8.8", "MX", "example.com", 3000, 0, errNullMX)
	if status != StatusEmpty {
		t.Errorf("status = %q, want %q", status, StatusEmpty)
	}
	want := "example.com accepts no email at all. It publishes an empty MX record, which is the standard way for a domain to say so, according to 8.8.8.8."
	if message != want {
		t.Errorf("message = %q, want %q", message, want)
	}
}
