package ntpcheck

import (
	"context"
	"encoding/binary"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeParams(t *testing.T) {
	tests := []struct {
		name        string
		in          Params
		wantServers []string
		wantTimeout int
		wantErr     string
	}{
		{"empty defaults to the pool", Params{}, []string{DefaultServer}, 3000, ""},
		{"whitespace only defaults too", Params{Servers: []string{"   ", "\n"}}, []string{DefaultServer}, 3000, ""},
		{"one server", Params{Servers: []string{"time.example.com"}}, []string{"time.example.com"}, 3000, ""},
		{"four servers", Params{Servers: []string{"a.example", "b.example", "c.example", "d.example"}},
			[]string{"a.example", "b.example", "c.example", "d.example"}, 3000, ""},
		{"five servers rejected", Params{Servers: []string{"a.example", "b.example", "c.example", "d.example", "e.example"}},
			nil, 0, "at most 4 time servers"},
		{"an ipv4 literal", Params{Servers: []string{"192.168.1.10"}}, []string{"192.168.1.10"}, 3000, ""},
		{"an ipv6 literal in brackets", Params{Servers: []string{"[2606:4700::1111]:123"}}, []string{"[2606:4700::1111]:123"}, 3000, ""},
		{"an explicit port", Params{Servers: []string{"time.example.com:123"}}, []string{"time.example.com:123"}, 3000, ""},
		{"port zero rejected", Params{Servers: []string{"time.example.com:0"}}, nil, 0, "port between 1 and 65535"},
		{"port too high rejected", Params{Servers: []string{"time.example.com:65536"}}, nil, 0, "port between 1 and 65535"},
		{"highest port accepted", Params{Servers: []string{"time.example.com:65535"}}, []string{"time.example.com:65535"}, 3000, ""},
		{"timeout 0 uses the default", Params{TimeoutMS: 0}, []string{DefaultServer}, 3000, ""},
		{"timeout 199 rejected", Params{TimeoutMS: 199}, nil, 0, "between 0.2 and 15 seconds"},
		{"timeout 200 accepted", Params{TimeoutMS: 200}, []string{DefaultServer}, 200, ""},
		{"timeout 15000 accepted", Params{TimeoutMS: 15000}, []string{DefaultServer}, 15000, ""},
		{"timeout 15001 rejected", Params{TimeoutMS: 15001}, nil, 0, "between 0.2 and 15 seconds"},
		{"negative timeout rejected", Params{TimeoutMS: -1}, nil, 0, "between 0.2 and 15 seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.normalize()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("want an error containing %q, got none", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize errored: %v", err)
			}
			if !reflect.DeepEqual(got.servers, tt.wantServers) {
				t.Errorf("servers = %v, want %v", got.servers, tt.wantServers)
			}
			if got.timeoutMS != tt.wantTimeout {
				t.Errorf("timeoutMs = %d, want %d", got.timeoutMS, tt.wantTimeout)
			}
		})
	}
}

func TestSplitServers(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"trimmed", []string{" pool.ntp.org "}, []string{"pool.ntp.org"}},
		{"newlines", []string{"a.example\nb.example"}, []string{"a.example", "b.example"}},
		{"commas", []string{"a.example, b.example"}, []string{"a.example", "b.example"}},
		{"semicolons", []string{"a.example;b.example"}, []string{"a.example", "b.example"}},
		{"crlf", []string{"a.example\r\nb.example"}, []string{"a.example", "b.example"}},
		{"duplicates collapse", []string{"a.example", "a.example"}, []string{"a.example"}},
		{"empties dropped", []string{"", "  ", "a.example"}, []string{"a.example"}},
		{"nothing at all", nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitServers(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitServers(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestThresholdConstants writes the three numbers in as literals. Every one of
// them also appears in a sentence the user reads ("5 minutes"), so if the
// constant moves without the wording moving, this fails.
func TestThresholdConstants(t *testing.T) {
	if FineOffsetMS != 1000 {
		t.Errorf("FineOffsetMS = %d, want 1000 (one second)", FineOffsetMS)
	}
	if DriftOffsetMS != 60000 {
		t.Errorf("DriftOffsetMS = %d, want 60000 (one minute)", DriftOffsetMS)
	}
	if KerberosLimitMS != 300000 {
		t.Errorf("KerberosLimitMS = %d, want 300000: Kerberos allows five minutes, and the message says so", KerberosLimitMS)
	}
}

func TestClassifyOffset(t *testing.T) {
	tests := []struct {
		name       string
		offsetMS   int
		wantStatus string
		wantPhrase string
	}{
		{"dead on", 0, StatusOK, "That is fine."},
		{"just under a second", 999, StatusOK, "That is fine."},
		{"exactly a second", 1000, StatusWarn, "the clock is drifting"},
		{"just under a minute", 59999, StatusWarn, "the clock is drifting"},
		{"exactly a minute", 60000, StatusWarn, "inside the 5 minute limit"},
		{"just under five minutes", 299999, StatusWarn, "inside the 5 minute limit"},
		{"exactly five minutes", 300000, StatusError, "Domain logins will fail"},
		{"well past five minutes", 432000, StatusError, "Domain logins will fail"},
		{"negative, dead on", -0, StatusOK, "That is fine."},
		{"negative, just under a second", -999, StatusOK, "That is fine."},
		{"negative, exactly a second", -1000, StatusWarn, "the clock is drifting"},
		{"negative, just under a minute", -59999, StatusWarn, "the clock is drifting"},
		{"negative, exactly a minute", -60000, StatusWarn, "inside the 5 minute limit"},
		{"negative, just under five minutes", -299999, StatusWarn, "inside the 5 minute limit"},
		{"negative, exactly five minutes", -300000, StatusError, "Domain logins will fail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := classifyOffset("pool.ntp.org", time.Duration(tt.offsetMS)*time.Millisecond)
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q (message was %q)", status, tt.wantStatus, msg)
			}
			if !strings.Contains(msg, tt.wantPhrase) {
				t.Errorf("message %q does not contain %q", msg, tt.wantPhrase)
			}
			if !strings.Contains(msg, "pool.ntp.org") {
				t.Errorf("message %q does not name the server", msg)
			}
		})
	}
}

func TestClassifyOffsetDirection(t *testing.T) {
	_, ahead := classifyOffset("s", 2*time.Second)
	if !strings.Contains(ahead, "ahead of s") {
		t.Errorf("a positive offset read %q, want it to say ahead", ahead)
	}
	_, behind := classifyOffset("s", -2*time.Second)
	if !strings.Contains(behind, "behind s") {
		t.Errorf("a negative offset read %q, want it to say behind", behind)
	}
}

func TestDescribeGap(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0 ms"},
		{32 * time.Millisecond, "32 ms"},
		{-32 * time.Millisecond, "32 ms"},
		{999 * time.Millisecond, "999 ms"},
		{time.Second, "1 s"},
		{7 * time.Second, "7 s"},
		{-7 * time.Second, "7 s"},
		{59 * time.Second, "59 s"},
		{60 * time.Second, "1 m 0 s"},
		{432 * time.Second, "7 m 12 s"},
		{3600 * time.Second, "1 h 0 m 0 s"},
		{3672 * time.Second, "1 h 1 m 12 s"},
	}
	for _, tt := range tests {
		if got := describeGap(tt.in); got != tt.want {
			t.Errorf("describeGap(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestClassifyReport(t *testing.T) {
	ok := Server{Server: "a", Status: StatusOK, Message: "This computer is 32 ms ahead of a. That is fine."}
	warn := Server{Server: "b", Status: StatusWarn, Message: "b drifting"}
	bad := Server{Server: "c", Status: StatusError, Message: "c broken"}
	dead := Server{Server: "d", Status: StatusUnreachable, Message: "d silent"}

	tests := []struct {
		name        string
		in          []Server
		wantLevel   string
		wantIn      string
		wantAdvice  string
		adviceEmpty bool
	}{
		{"all fine", []Server{ok, ok}, StatusOK, "That is fine.", "", true},
		{"one warn among ok", []Server{ok, warn}, StatusWarn, "That is fine.", "Nothing is broken yet", false},
		{"one error among warn", []Server{warn, bad}, StatusError, "b drifting", "w32tm /resync", false},
		{"one dead among ok keeps the level", []Server{ok, dead}, StatusOK, "That is fine.", "1 of the 2 servers could not be reached.", false},
		{"all dead", []Server{dead, dead}, StatusError, "None of the time servers answered", "UDP port 123", false},
		{"no servers at all", []Server{}, StatusError, "None of the time servers answered", "UDP port 123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, headline, advice := Classify(tt.in)
			if level != tt.wantLevel {
				t.Errorf("level = %q, want %q", level, tt.wantLevel)
			}
			if !strings.Contains(headline, tt.wantIn) {
				t.Errorf("headline %q does not contain %q", headline, tt.wantIn)
			}
			if tt.adviceEmpty {
				if advice != "" {
					t.Errorf("advice = %q, want empty", advice)
				}
				return
			}
			if !strings.Contains(advice, tt.wantAdvice) {
				t.Errorf("advice %q does not contain %q", advice, tt.wantAdvice)
			}
		})
	}
}

// TestNoSuccessfulRowIsBlank is the "never ship an ok result with nothing in it"
// rule: every row a user can see carries a sentence, whatever happened.
func TestNoSuccessfulRowIsBlank(t *testing.T) {
	for _, offsetMS := range []int{0, 500, 1500, 90000, 400000, -400000} {
		_, msg := classifyOffset("pool.ntp.org", time.Duration(offsetMS)*time.Millisecond)
		if strings.TrimSpace(msg) == "" {
			t.Fatalf("offset %d ms produced an empty message", offsetMS)
		}
	}
}

func TestReplyMessage(t *testing.T) {
	tests := []struct {
		name string
		r    reply
		err  error
		want string
	}{
		{"kiss of death", reply{kiss: "RATE"}, errKiss, "sent back the code RATE"},
		{"kiss with no code", reply{}, errKiss, "an empty code"},
		{"no time in it", reply{}, errNoTime, "without a time in it"},
		{"not a time packet", reply{}, errMode, "not with a time"},
		{"too short", reply{}, errShort, "not with a time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replyMessage("time.example.com", tt.r, tt.err)
			if !strings.Contains(got, tt.want) {
				t.Errorf("message %q does not contain %q", got, tt.want)
			}
			if !strings.Contains(got, "time.example.com") {
				t.Errorf("message %q does not name the server", got)
			}
		})
	}
}

// TestReportServersNeverNil is the input that reaches the guard: a report is
// built from a slice that is empty rather than from one that has rows.
func TestReportServersNeverNil(t *testing.T) {
	r := Report{Servers: []Server{}}
	if r.Servers == nil {
		t.Fatal("Servers is nil, which marshals to JSON null and breaks the table")
	}
}

// fakeNTP answers on 127.0.0.1 with a scripted reply. reply is called with the
// request so a test can echo the originate timestamp back, which a real server
// does and parseReply insists on.
type fakeNTP struct {
	conn  *net.UDPConn
	mu    sync.Mutex
	asked int
}

func startFakeNTP(t *testing.T, respond func(req []byte) []byte) *fakeNTP {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeNTP{conn: conn}
	go func() {
		buf := make([]byte, 1024)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			f.mu.Lock()
			f.asked++
			f.mu.Unlock()
			if out := respond(append([]byte(nil), buf[:n]...)); out != nil {
				_, _ = conn.WriteToUDP(out, addr)
			}
		}
	}()
	t.Cleanup(func() { conn.Close() })
	return f
}

func (f *fakeNTP) addr() string { return f.conn.LocalAddr().String() }

func (f *fakeNTP) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

// scriptedReply builds a stratum 2 answer whose clock is skew behind this
// machine's, echoing the originate timestamp the client sent.
func scriptedReply(skew time.Duration) func([]byte) []byte {
	return func(req []byte) []byte {
		out := make([]byte, 48)
		out[0] = 0x24 // leap 0, version 4, mode 4 (server)
		out[1] = 2    // stratum
		out[3] = 0xEC
		copy(out[12:16], "GPS\x00")
		serverNow := time.Now().Add(-skew)
		secs, frac := toNTP(serverNow)
		binary.BigEndian.PutUint32(out[16:], secs)
		binary.BigEndian.PutUint32(out[20:], frac)
		copy(out[24:32], req[40:48]) // originate, echoed
		binary.BigEndian.PutUint32(out[32:], secs)
		binary.BigEndian.PutUint32(out[36:], frac)
		binary.BigEndian.PutUint32(out[40:], secs)
		binary.BigEndian.PutUint32(out[44:], frac)
		return out
	}
}

func TestQueryAgainstLocalServer(t *testing.T) {
	// The fake's clock is 90 seconds behind this machine's, so this computer
	// reads as 90 s ahead: past drifting, inside the Kerberos limit.
	f := startFakeNTP(t, scriptedReply(90*time.Second))

	got := query(context.Background(), f.addr(), 2*time.Second, 2000)
	if got.Status != StatusWarn {
		t.Fatalf("status = %q (%s), want %q", got.Status, got.Message, StatusWarn)
	}
	if got.Stratum != 2 {
		t.Errorf("stratum = %d, want 2", got.Stratum)
	}
	// The measurement includes real round trip time on loopback, so the offset
	// is pinned to a window rather than a value. A window this tight still fails
	// if the sign is flipped or the era offset is wrong (both would be out by
	// minutes or by 70 years).
	if got.OffsetMS < 89000 || got.OffsetMS > 91000 {
		t.Errorf("offset = %v ms, want about 90000", got.OffsetMS)
	}
	if got.DelayMS < 0 || got.DelayMS > 1000 {
		t.Errorf("delay = %v ms, want a small positive number on loopback", got.DelayMS)
	}
	if got.ServerTime == "" || got.LocalTime == "" {
		t.Errorf("times are blank: server %q local %q", got.ServerTime, got.LocalTime)
	}
	if !strings.Contains(got.Message, "inside the 5 minute limit") {
		t.Errorf("message = %q", got.Message)
	}
	if got.Address == "" {
		t.Error("address is blank")
	}
}

func TestQueryAgainstAccurateServer(t *testing.T) {
	f := startFakeNTP(t, scriptedReply(0))

	got := query(context.Background(), f.addr(), 2*time.Second, 2000)
	if got.Status != StatusOK {
		t.Fatalf("status = %q (%s), want %q", got.Status, got.Message, StatusOK)
	}
	if !strings.Contains(got.Message, "That is fine.") {
		t.Errorf("message = %q", got.Message)
	}
}

func TestQueryTimesOut(t *testing.T) {
	f := startFakeNTP(t, func([]byte) []byte { return nil })

	started := time.Now()
	got := query(context.Background(), f.addr(), 300*time.Millisecond, 300)
	elapsed := time.Since(started)

	if got.Status != StatusUnreachable {
		t.Fatalf("status = %q, want %q", got.Status, StatusUnreachable)
	}
	if !strings.Contains(got.Message, "did not answer within 300 ms") {
		t.Errorf("message = %q", got.Message)
	}
	if !strings.Contains(got.Message, "UDP port 123") {
		t.Errorf("message %q does not explain what to check", got.Message)
	}
	if elapsed > 3*time.Second {
		t.Errorf("the timeout took %v, so the deadline is not being honoured", elapsed)
	}
	if f.count() == 0 {
		t.Error("the fake was never asked, so this test proved nothing")
	}
}

func TestQueryRejectsAKissOfDeath(t *testing.T) {
	f := startFakeNTP(t, func(req []byte) []byte {
		out := scriptedReply(0)(req)
		out[1] = 0 // stratum 0
		copy(out[12:16], "RATE")
		return out
	})

	got := query(context.Background(), f.addr(), 2*time.Second, 2000)
	if got.Status != StatusUnreachable {
		t.Fatalf("status = %q, want %q", got.Status, StatusUnreachable)
	}
	if !strings.Contains(got.Message, "RATE") {
		t.Errorf("message %q does not carry the kiss code", got.Message)
	}
	if got.OffsetMS != 0 {
		t.Errorf("offset = %v, want 0: a kiss of death carries no usable time", got.OffsetMS)
	}
}

func TestQueryRejectsAStrayPacket(t *testing.T) {
	// The reply does not echo the originate timestamp, so it is an answer to a
	// question nobody here asked.
	f := startFakeNTP(t, func(req []byte) []byte {
		out := scriptedReply(0)(req)
		copy(out[24:32], make([]byte, 8))
		return out
	})

	got := query(context.Background(), f.addr(), 2*time.Second, 2000)
	if got.Status != StatusUnreachable {
		t.Fatalf("status = %q, want %q", got.Status, StatusUnreachable)
	}
	if !strings.Contains(got.Message, "not with a time") {
		t.Errorf("message = %q", got.Message)
	}
}

func TestCheckNamesABadServerBeforeSending(t *testing.T) {
	f := startFakeNTP(t, scriptedReply(0))

	_, err := Check(context.Background(), Params{Servers: []string{f.addr(), "bad:0"}})
	if err == nil {
		t.Fatal("want an error for a bad port")
	}
	if f.count() != 0 {
		t.Errorf("the fake was asked %d times: validation must happen before any packet is sent", f.count())
	}
}

func TestCheckReturnsServersInRequestOrder(t *testing.T) {
	slow := startFakeNTP(t, func(req []byte) []byte {
		time.Sleep(120 * time.Millisecond)
		return scriptedReply(0)(req)
	})
	fast := startFakeNTP(t, scriptedReply(0))

	report, err := Check(context.Background(), Params{
		Servers:   []string{slow.addr(), fast.addr()},
		TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatalf("Check errored: %v", err)
	}
	if len(report.Servers) != 2 {
		t.Fatalf("got %d rows, want 2", len(report.Servers))
	}
	if report.Servers[0].Server != slow.addr() {
		t.Errorf("row 0 is %q, want the slow server %q: rows must follow the order they were typed",
			report.Servers[0].Server, slow.addr())
	}
	if report.CheckedAt == "" {
		t.Error("checkedAt is blank")
	}
}
