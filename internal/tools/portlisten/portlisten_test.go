package portlisten

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		in       Params
		wantPort int
		wantProt string
		wantErr  string
	}{
		// The literals below are written in on purpose. Reading them out of the
		// constants would prove only that a constant equals itself, and the port
		// floor is in a user-facing sentence.
		{"zero port takes the default", Params{}, 8730, ProtoTCP, ""},
		{"zero protocol takes tcp", Params{Port: 5000}, 5000, ProtoTCP, ""},
		{"uppercase protocol", Params{Port: 5000, Protocol: "TCP"}, 5000, ProtoTCP, ""},
		{"padded protocol", Params{Port: 5000, Protocol: " udp "}, 5000, ProtoUDP, ""},
		{"both", Params{Port: 5000, Protocol: "both"}, 5000, ProtoBoth, ""},
		{"lowest allowed port", Params{Port: 1024, Protocol: "tcp"}, 1024, ProtoTCP, ""},
		{"highest allowed port", Params{Port: 65535, Protocol: "tcp"}, 65535, ProtoTCP, ""},
		{
			"one below the floor", Params{Port: 1023}, 0, "",
			"The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.",
		},
		{
			"one above the ceiling", Params{Port: 65536}, 0, "",
			"The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.",
		},
		{
			"negative port", Params{Port: -1}, 0, "",
			"The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.",
		},
		{"unknown protocol", Params{Port: 5000, Protocol: "sctp"}, 0, "", "Choose TCP, UDP, or both."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := tt.in.validate()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("wanted an error, got none")
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("message\n got %q\nwant %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts.port != tt.wantPort {
				t.Errorf("port = %d, want %d", opts.port, tt.wantPort)
			}
			if opts.protocol != tt.wantProt {
				t.Errorf("protocol = %q, want %q", opts.protocol, tt.wantProt)
			}
		})
	}
}

func TestWantTCPAndUDP(t *testing.T) {
	tests := []struct {
		protocol string
		tcp, udp bool
	}{
		{ProtoTCP, true, false},
		{ProtoUDP, false, true},
		{ProtoBoth, true, true},
	}
	for _, tt := range tests {
		o := options{protocol: tt.protocol}
		if o.wantTCP() != tt.tcp || o.wantUDP() != tt.udp {
			t.Errorf("%s: tcp=%v udp=%v, want %v %v", tt.protocol, o.wantTCP(), o.wantUDP(), tt.tcp, tt.udp)
		}
	}
}

func TestPreview(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", nil, ""},
		{"plain ascii", []byte("GET / HTTP/1.1"), "GET / HTTP/1.1"},
		{"control bytes become dots", []byte{'a', 0x00, 0x1f, 'b', 0x7f, 'c'}, "a..b.c"},
		{"newline is not printable", []byte("a\nb"), "a.b"},
		{"invalid utf8 becomes a dot", []byte{0xff, 0xfe, 'a'}, "..a"},
		{"multi byte runes survive", []byte("café 日本"), "café 日本"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := preview(tt.in); got != tt.want {
				t.Errorf("preview = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPreviewCutsAtEightyRunes(t *testing.T) {
	// 80 is written in rather than read from previewChars: a test that reads the
	// cap it is checking pins nothing.
	got := preview([]byte(strings.Repeat("x", 300)))
	if len(got) != 80 {
		t.Fatalf("length = %d, want 80", len(got))
	}
}

func TestPreviewCutsOnARuneBoundary(t *testing.T) {
	// 100 three-byte runes. Cutting by bytes would leave a broken rune at the
	// end; cutting by runes gives exactly 80 of them, which is 240 bytes.
	got := preview([]byte(strings.Repeat("日", 100)))
	if want := strings.Repeat("日", 80); got != want {
		t.Fatalf("got %d bytes, want %d and no partial rune", len(got), len(want))
	}
}

func TestSessionNote(t *testing.T) {
	const blocked = "Nothing reached this port. The block is usually this computer's own firewall (Windows asks the first time CHIT listens) or a firewall, ACL or NAT rule between the two machines."
	const udpCaveat = " Nothing acknowledges a blocked datagram, so on UDP an empty list is not proof on its own."

	tests := []struct {
		name                  string
		tcp, udp, dropped     int
		protocol, wantContain string
		want                  string
	}{
		{name: "nothing on tcp", protocol: ProtoTCP, want: blocked},
		{name: "nothing on udp", protocol: ProtoUDP, want: blocked + udpCaveat},
		{name: "nothing on both", protocol: ProtoBoth, want: blocked + udpCaveat},
		{name: "arrivals and nothing dropped", tcp: 3, protocol: ProtoTCP, want: ""},
		{
			name: "arrivals with rows dropped", tcp: 500, dropped: 1284, protocol: ProtoTCP,
			want: "Stopped listing arrivals after 500 rows. 1,284 more arrived and are counted in the totals above.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionNote(tt.tcp, tt.udp, tt.dropped, tt.protocol)
			if got != tt.want {
				t.Errorf("note\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestThousands(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"}, {7, "7"}, {999, "999"}, {1000, "1,000"},
		{1284, "1,284"}, {12345, "12,345"}, {1234567, "1,234,567"},
	}
	for _, tt := range tests {
		if got := thousands(tt.in); got != tt.want {
			t.Errorf("thousands(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestProtocolWords(t *testing.T) {
	tests := map[string]string{ProtoTCP: "TCP", ProtoUDP: "UDP", ProtoBoth: "TCP and UDP"}
	for in, want := range tests {
		if got := protocolWords(in); got != want {
			t.Errorf("protocolWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArrivalLine(t *testing.T) {
	tests := []struct {
		hits, peers int
		want        string
	}{
		{4, 2, "4 arrivals from 2 machines"},
		{1, 1, "1 arrival from 1 machine"},
		{0, 0, "0 arrivals from 0 machines"},
	}
	for _, tt := range tests {
		if got := arrivalLine(tt.hits, tt.peers); got != tt.want {
			t.Errorf("arrivalLine(%d,%d) = %q, want %q", tt.hits, tt.peers, got, tt.want)
		}
	}
}

func TestSummaryFor(t *testing.T) {
	s := summaryFor(8730, ProtoBoth, 3, 2, 2, 0)

	for _, key := range []string{"port", "protocol", "tcp", "udp", "peers", "dropped", "note"} {
		if _, ok := s[key]; !ok {
			t.Errorf("summary is missing key %q", key)
		}
	}
	if s["port"] != 8730 || s["protocol"] != ProtoBoth {
		t.Errorf("port/protocol wrong: %v %v", s["port"], s["protocol"])
	}
	if s["tcp"] != 3 || s["udp"] != 2 || s["peers"] != 2 {
		t.Errorf("tallies wrong: %v", s)
	}
	if s["note"] != "" {
		t.Errorf("note should be empty when arrivals happened and nothing dropped, got %q", s["note"])
	}
}

func TestSummaryCountsPeersNotArrivals(t *testing.T) {
	sess := newSession(options{port: 8730, protocol: ProtoTCP}, Sink{})
	sess.record(Hit{Protocol: ProtoTCP, Peer: "10.0.0.7"})
	sess.record(Hit{Protocol: ProtoTCP, Peer: "10.0.0.7"})
	sess.record(Hit{Protocol: ProtoTCP, Peer: "10.0.0.9"})

	s := sess.summary()
	if s["tcp"] != 3 {
		t.Errorf("tcp = %v, want 3", s["tcp"])
	}
	if s["peers"] != 2 {
		t.Errorf("peers = %v, want 2 distinct machines", s["peers"])
	}
}

func TestNoSliceIsNil(t *testing.T) {
	data, err := json.Marshal(summaryFor(8730, ProtoTCP, 0, 0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Fatalf("summary marshals a null: %s", data)
	}
}

// recorder collects what a session emitted. The accept loop runs on its own
// goroutine, so a plain slice read straight after a dial passes about nine runs
// in ten. internal/tools/filedrop shipped exactly that flake once.
type recorder struct {
	mu   sync.Mutex
	hits []Hit
}

func (r *recorder) sink() Sink {
	return Sink{
		Emit: func(h Hit) {
			r.mu.Lock()
			r.hits = append(r.hits, h)
			r.mu.Unlock()
		},
		Progress: func(string) {},
	}
}

func (r *recorder) wait(t *testing.T, want int) []Hit {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		n := len(r.hits)
		r.mu.Unlock()
		if n >= want {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Hit, len(r.hits))
	copy(out, r.hits)
	return out
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.hits)
}

// startLoopback binds the requested protocols on 127.0.0.1 with a port the
// operating system chooses, so nothing in the suite needs a fixed port, a LAN or
// elevation.
func startLoopback(t *testing.T, protocol string) (*session, *recorder, string, context.CancelFunc, chan error) {
	t.Helper()

	var (
		tcpLn   net.Listener
		udpConn net.PacketConn
		address string
		err     error
	)
	if protocol == ProtoTCP || protocol == ProtoBoth {
		tcpLn, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address = tcpLn.Addr().String()
	}
	if protocol == ProtoUDP || protocol == ProtoBoth {
		bind := "127.0.0.1:0"
		if address != "" {
			bind = address
		}
		udpConn, err = net.ListenPacket("udp", bind)
		if err != nil {
			t.Fatal(err)
		}
		if address == "" {
			address = udpConn.LocalAddr().String()
		}
	}

	rec := &recorder{}
	sess := newSession(options{port: 1024, protocol: protocol}, rec.sink())

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- Serve(ctx, tcpLn, udpConn, sess) }()

	return sess, rec, address, cancel, errc
}

func TestServeTCPRecordsAConnection(t *testing.T) {
	_, rec, address, cancel, errc := startLoopback(t, ProtoTCP)
	defer func() { cancel(); <-errc }()

	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	banner := make([]byte, 128)
	n, _ := conn.Read(banner)
	if !strings.Contains(string(banner[:n]), "CHIT port listener") {
		t.Fatalf("no banner came back, got %q", banner[:n])
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	hits := rec.wait(t, 1)
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want exactly 1", len(hits))
	}
	got := hits[0]
	if got.Protocol != ProtoTCP {
		t.Errorf("protocol = %q, want tcp", got.Protocol)
	}
	if got.Peer != "127.0.0.1" {
		t.Errorf("peer = %q, want 127.0.0.1", got.Peer)
	}
	if got.PeerPort == 0 {
		t.Errorf("peer port was not recorded")
	}
	if got.Bytes != 5 {
		t.Errorf("bytes = %d, want 5", got.Bytes)
	}
	if got.Preview != "hello" {
		t.Errorf("preview = %q, want %q", got.Preview, "hello")
	}
	if got.Time == "" {
		t.Errorf("time was not stamped")
	}
}

func TestServeTCPRecordsAScannerThatSendsNothing(t *testing.T) {
	_, rec, address, cancel, errc := startLoopback(t, ProtoTCP)
	defer func() { cancel(); <-errc }()

	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	hits := rec.wait(t, 1)
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want exactly 1", len(hits))
	}
	if hits[0].Bytes != 0 || hits[0].Preview != "" {
		t.Errorf("a silent connection should record 0 bytes and no preview, got %+v", hits[0])
	}
}

func TestServeUDPRecordsAndEchoes(t *testing.T) {
	_, rec, address, cancel, errc := startLoopback(t, ProtoUDP)
	defer func() { cancel(); <-errc }()

	conn, err := net.Dial("udp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	echo := make([]byte, 32)
	n, err := conn.Read(echo)
	if err != nil {
		t.Fatalf("no echo came back: %v", err)
	}
	if string(echo[:n]) != "ping" {
		t.Errorf("echo = %q, want %q", echo[:n], "ping")
	}

	hits := rec.wait(t, 1)
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want exactly 1", len(hits))
	}
	if hits[0].Protocol != ProtoUDP || hits[0].Bytes != 4 || hits[0].Preview != "ping" {
		t.Errorf("hit = %+v", hits[0])
	}
}

func TestServeStopsOnCancelAndReleasesThePort(t *testing.T) {
	_, _, address, cancel, errc := startLoopback(t, ProtoBoth)

	cancel()
	select {
	case err := <-errc:
		if err != context.Canceled {
			t.Fatalf("Serve returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return within 2 seconds of cancel")
	}

	// The assertion that actually proves the port was released: bind it again.
	again, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("the TCP port was still held after Stop: %v", err)
	}
	again.Close()

	pc, err := net.ListenPacket("udp", address)
	if err != nil {
		t.Fatalf("the UDP port was still held after Stop: %v", err)
	}
	pc.Close()
}

func TestServeCapsHits(t *testing.T) {
	sess, rec, address, cancel, errc := startLoopback(t, ProtoUDP)
	defer func() { cancel(); <-errc }()

	conn, err := net.Dial("udp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 505 datagrams: 500 make it to the table (the literal, not the constant)
	// and 5 are counted only in the totals.
	const sent = 505
	for i := 0; i < sent; i++ {
		if _, err := conn.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		// UDP on loopback can still drop under a tight loop; a short pause keeps
		// the test about the cap rather than about the kernel's socket buffer.
		time.Sleep(200 * time.Microsecond)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := sess.summary(); s["udp"].(int) >= sent {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	s := sess.summary()
	if s["udp"] != sent {
		t.Fatalf("udp tally = %v, want %d (the kernel dropped datagrams, rerun)", s["udp"], sent)
	}
	if got := rec.count(); got != 500 {
		t.Errorf("emitted %d rows, want exactly 500", got)
	}
	if s["dropped"] != 5 {
		t.Errorf("dropped = %v, want 5", s["dropped"])
	}
	note, _ := s["note"].(string)
	if !strings.Contains(note, "5 more arrived") {
		t.Errorf("note does not mention the dropped rows: %q", note)
	}
}

func TestStartListenRejectsBeforeBinding(t *testing.T) {
	svc := New(nil)
	if _, err := svc.StartListen(Params{Port: 80}); err == nil {
		t.Fatal("a privileged port should be refused before anything binds")
	}
	if _, err := svc.StartListen(Params{Port: 9000, Protocol: "sctp"}); err == nil {
		t.Fatal("an unknown protocol should be refused before anything binds")
	}
}

func TestBannerNamesThePeer(t *testing.T) {
	got := bannerFor("10.0.0.7:51234")
	if !strings.Contains(got, "10.0.0.7:51234") {
		t.Errorf("banner does not name the peer: %q", got)
	}
	if !strings.HasSuffix(got, "\r\n") {
		t.Errorf("banner should end with CRLF so telnet renders it: %q", got)
	}
}

// TestServeWaitsForASlowClient pins the read deadline's lower bound. A client on
// a real network does not send the instant it connects, and a deadline short
// enough to cut it off would silently record 0 bytes for a connection that did
// carry data. Found by mutation: tcpDeadline could be dropped to a millisecond
// with every other test still green.
func TestServeWaitsForASlowClient(t *testing.T) {
	_, rec, address, cancel, errc := startLoopback(t, ProtoTCP)
	defer func() { cancel(); <-errc }()

	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	// Well inside the 2 second deadline and far outside a millisecond one.
	time.Sleep(300 * time.Millisecond)
	if _, err := conn.Write([]byte("late")); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	hits := rec.wait(t, 1)
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want exactly 1", len(hits))
	}
	if hits[0].Bytes != 4 || hits[0].Preview != "late" {
		t.Errorf("a client that paused before speaking lost its bytes: %+v", hits[0])
	}
}

// TestServeDoesNotWaitForeverOnASilentClient pins the deadline's upper bound: a
// port scanner that connects and never speaks or closes must not hold a
// goroutine indefinitely, and its connection must still be recorded.
func TestServeDoesNotWaitForeverOnASilentClient(t *testing.T) {
	_, rec, address, cancel, errc := startLoopback(t, ProtoTCP)
	defer func() { cancel(); <-errc }()

	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 2 seconds is the deadline, so 5 is a generous bound that still fails if the
	// deadline is ever raised to something like a minute.
	started := time.Now()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && rec.count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if rec.count() != 1 {
		t.Fatalf("a silent connection was not recorded within 5 seconds")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("took %v to give up on a silent client", elapsed)
	}
}
