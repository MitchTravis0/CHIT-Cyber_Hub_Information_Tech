package ntpcheck

import (
	"encoding/hex"
	"testing"
	"time"
)

// The four instants below and every expected number in this file came out of a
// python3 script using struct and datetime, run before any of this was written.
// The fractions are deliberately dyadic (31.25 ms, 515.625 ms): NTP's fraction
// is 32 bit binary fixed point, so a "round" 40 ms comes back as 39.99996 ms and
// a test using it would be pinning quantisation noise instead of the arithmetic.
var (
	fixtureT1 = time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	fixtureT2 = time.Date(2026, 7, 28, 9, 59, 58, 515625000, time.UTC)
	fixtureT3 = fixtureT2
	fixtureT4 = time.Date(2026, 7, 28, 10, 0, 0, 31250000, time.UTC)
)

// Built by python3, not by buildRequest, so the two are independent.
const (
	pythonRequestHex = "23000000000000000000000000000000000000000000000000000000000000000000000000000000ee12fc2000000000"
	pythonReplyHex   = "240204ec000001230000045647505300ee12fc1e84000000ee12fc2000000000ee12fc1e84000000ee12fc1e84000000"
	pythonKissHex    = "240004ec000001230000045652415445ee12fc1e84000000ee12fc2000000000ee12fc1e84000000ee12fc1e84000000"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	return b
}

func TestBuildRequest(t *testing.T) {
	got := buildRequest(fixtureT1)

	if len(got) != 48 {
		t.Fatalf("request is %d bytes, want 48", len(got))
	}
	// 0x23 is leap 0, version 4, mode 3. Written as a literal so a change to the
	// version or the mode fails here rather than confusing a time server.
	if got[0] != 0x23 {
		t.Errorf("first byte = %#x, want 0x23", got[0])
	}
	for i := 1; i < 40; i++ {
		if got[i] != 0 {
			t.Errorf("byte %d = %#x, want 0: a client fills in nothing but its transmit time", i, got[i])
		}
	}
	if want := mustHex(t, pythonRequestHex); hex.EncodeToString(got) != hex.EncodeToString(want) {
		t.Errorf("request\n got %s\nwant %s", hex.EncodeToString(got), hex.EncodeToString(want))
	}
}

func TestNTPTimestampRoundTrip(t *testing.T) {
	// The Unix epoch sits exactly this many seconds after the NTP epoch.
	// Confirmed against python3's datetime.
	if secs, frac := toNTP(time.Unix(0, 0).UTC()); secs != 2208988800 || frac != 0 {
		t.Errorf("unix epoch = (%d, %d), want (2208988800, 0)", secs, frac)
	}

	tests := []struct {
		name string
		in   time.Time
	}{
		{"whole second", fixtureT1},
		{"dyadic fraction", fixtureT2},
		{"small dyadic fraction", fixtureT4},
		{"quarter second", time.Date(2000, 1, 1, 0, 0, 0, 250000000, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secs, frac := toNTP(tt.in)
			got := fromNTP(secs, frac)
			if !got.Equal(tt.in) {
				t.Errorf("round trip of %s gave %s", tt.in, got)
			}
		})
	}

	if got := fromNTP(0, 0); !got.IsZero() {
		t.Errorf("all-zero timestamp decoded to %s, want the zero time", got)
	}
	if secs, frac := toNTP(time.Time{}); secs != 0 || frac != 0 {
		t.Errorf("zero time encoded to (%d, %d), want (0, 0)", secs, frac)
	}
}

func TestNTPTimestampFractionIsNotLossy(t *testing.T) {
	// A non-dyadic fraction cannot survive exactly, and the tolerance is one
	// unit of the 32 bit fraction, which is about 0.233 nanoseconds. This pins
	// that the conversion is doing real fixed point arithmetic rather than, say,
	// throwing the fraction away.
	in := time.Date(2026, 7, 28, 10, 0, 0, 123456789, time.UTC)
	got := fromNTP(toNTP(in))
	if d := got.Sub(in); d > time.Nanosecond || d < -time.Nanosecond {
		t.Errorf("round trip lost %v, want at most 1 ns", d)
	}
}

func TestParseReply(t *testing.T) {
	good := mustHex(t, pythonReplyHex)

	t.Run("good reply", func(t *testing.T) {
		r, err := parseReply(good, fixtureT1)
		if err != nil {
			t.Fatalf("parseReply errored: %v", err)
		}
		if r.stratum != 2 {
			t.Errorf("stratum = %d, want 2", r.stratum)
		}
		if !r.originate.Equal(fixtureT1) {
			t.Errorf("originate = %s, want %s", r.originate, fixtureT1)
		}
		if !r.receive.Equal(fixtureT2) {
			t.Errorf("receive = %s, want %s", r.receive, fixtureT2)
		}
		if !r.transmit.Equal(fixtureT3) {
			t.Errorf("transmit = %s, want %s", r.transmit, fixtureT3)
		}
	})

	t.Run("47 bytes is too short", func(t *testing.T) {
		if _, err := parseReply(good[:47], fixtureT1); err != errShort {
			t.Errorf("err = %v, want %v", err, errShort)
		}
	})

	t.Run("empty is too short", func(t *testing.T) {
		if _, err := parseReply(nil, fixtureT1); err != errShort {
			t.Errorf("err = %v, want %v", err, errShort)
		}
	})

	t.Run("mode 3 is a client packet, not an answer", func(t *testing.T) {
		b := append([]byte(nil), good...)
		b[0] = 0x23
		if _, err := parseReply(b, fixtureT1); err != errMode {
			t.Errorf("err = %v, want %v", err, errMode)
		}
	})

	t.Run("mode 5 broadcast is accepted", func(t *testing.T) {
		b := append([]byte(nil), good...)
		b[0] = (b[0] &^ 0x07) | 5
		if _, err := parseReply(b, fixtureT1); err != nil {
			t.Errorf("mode 5 rejected: %v", err)
		}
	})

	t.Run("stratum 0 is a kiss of death", func(t *testing.T) {
		r, err := parseReply(mustHex(t, pythonKissHex), fixtureT1)
		if err != errKiss {
			t.Fatalf("err = %v, want %v", err, errKiss)
		}
		if r.kiss != "RATE" {
			t.Errorf("kiss = %q, want %q", r.kiss, "RATE")
		}
		if r.stratum != 0 {
			t.Errorf("stratum = %d, want 0", r.stratum)
		}
	})

	t.Run("zero transmit timestamp", func(t *testing.T) {
		b := append([]byte(nil), good...)
		copy(b[40:48], make([]byte, 8))
		if _, err := parseReply(b, fixtureT1); err != errNoTime {
			t.Errorf("err = %v, want %v", err, errNoTime)
		}
	})

	t.Run("zero receive timestamp", func(t *testing.T) {
		b := append([]byte(nil), good...)
		copy(b[32:40], make([]byte, 8))
		if _, err := parseReply(b, fixtureT1); err != errNoTime {
			t.Errorf("err = %v, want %v", err, errNoTime)
		}
	})

	t.Run("originate that is not what we sent", func(t *testing.T) {
		// A reply that does not echo our transmit timestamp is an answer to
		// somebody else's question, or a forgery.
		if _, err := parseReply(good, fixtureT1.Add(time.Second)); err != errMismatch {
			t.Errorf("err = %v, want %v", err, errMismatch)
		}
	})

	t.Run("originate from a real clock reading", func(t *testing.T) {
		// The instant a client actually sends at has an arbitrary nanosecond,
		// which the 32 bit fraction cannot hold exactly. Comparing decoded times
		// here rejected every real reply; comparing the encoded form does not.
		sent := time.Date(2026, 7, 28, 10, 0, 0, 123456789, time.UTC)
		req := buildRequest(sent)
		b := append([]byte(nil), good...)
		copy(b[24:32], req[40:48])
		if _, err := parseReply(b, sent); err != nil {
			t.Errorf("a reply echoing a real send time was rejected: %v", err)
		}
	})

	t.Run("longer than 48 bytes is fine", func(t *testing.T) {
		b := append(append([]byte(nil), good...), make([]byte, 16)...)
		if _, err := parseReply(b, fixtureT1); err != nil {
			t.Errorf("a 64 byte reply was rejected: %v", err)
		}
	})
}

func TestOffsetAndDelay(t *testing.T) {
	r, err := parseReply(mustHex(t, pythonReplyHex), fixtureT1)
	if err != nil {
		t.Fatalf("parseReply errored: %v", err)
	}
	offset, delay := offsetAndDelay(fixtureT1, r, fixtureT4)

	// python3 computed these from the same four instants:
	//   theta  = ((T2-T1) + (T3-T4)) / 2 = -1.5 s   (the server is behind)
	//   offset = -theta                  = +1500 ms (this computer is ahead)
	//   delay  = (T4-T1) - (T3-T2)       = 31.25 ms
	if got := milliseconds(offset); got != 1500 {
		t.Errorf("offset = %v ms, want 1500", got)
	}
	if got := milliseconds(delay); got != 31.25 {
		t.Errorf("delay = %v ms, want 31.25", got)
	}
}

// TestOffsetAndDelaySubtractsServerTime is the case the main fixture cannot
// reach: it has T2 == T3, so the server spends no time thinking and the delay
// arithmetic reads the same whether that term is added or subtracted. A real
// server always takes some time, and it must not count as network delay.
func TestOffsetAndDelaySubtractsServerTime(t *testing.T) {
	// Dyadic fractions again. The server receives at T2 and answers 250 ms
	// later, so of the 1 second round trip only 750 ms was on the network.
	t1 := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	t4 := t1.Add(time.Second)
	r := reply{
		receive:  t1.Add(500 * time.Millisecond),
		transmit: t1.Add(750 * time.Millisecond),
	}
	offset, delay := offsetAndDelay(t1, r, t4)

	// delay  = (T4-T1) - (T3-T2) = 1000 - 250 = 750 ms
	// theta  = ((T2-T1) + (T3-T4)) / 2 = (500 + (-250)) / 2 = 125 ms
	// offset = -theta = -125 ms (this computer is behind)
	if got := milliseconds(delay); got != 750 {
		t.Errorf("delay = %v ms, want 750: the server's own 250 ms is not network delay", got)
	}
	if got := milliseconds(offset); got != -125 {
		t.Errorf("offset = %v ms, want -125", got)
	}
}

func TestOffsetSignFollowsThisComputer(t *testing.T) {
	// Every sentence the tool writes says how far *this computer* is from the
	// server, which is the opposite sign to the RFC's definition. Getting this
	// backwards would tell a tech to move the clock the wrong way.
	behind := reply{
		receive:  fixtureT1.Add(2 * time.Second),
		transmit: fixtureT1.Add(2 * time.Second),
	}
	offset, _ := offsetAndDelay(fixtureT1, behind, fixtureT1)
	if offset >= 0 {
		t.Errorf("a server 2 s ahead gave offset %v, want a negative number (this computer is behind)", offset)
	}
}
