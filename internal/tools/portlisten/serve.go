package portlisten

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"chit/internal/core"
)

// tcpDeadline is how long one accepted connection may hold a goroutine. A
// scanner that connects and never speaks would otherwise pin one for ever.
const tcpDeadline = 2 * time.Second

// Sink is where a session reports to. The job wires it to jc.Emit and
// jc.Progress; a test wires it to a slice, which is the only way to read what a
// session emitted without a Wails runtime.
type Sink struct {
	Emit     func(Hit)
	Progress func(string)
}

// session is one running listener. Everything a request can influence is a
// counter: nothing it sends is ever used to open a file, build a path or run a
// command.
type session struct {
	opts options
	out  Sink

	mu      sync.Mutex
	tcp     int
	udp     int
	emitted int
	dropped int
	peers   map[string]struct{}
}

func newSession(opts options, out Sink) *session {
	return &session{opts: opts, out: out, peers: map[string]struct{}{}}
}

// Service owns the listening entry point. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartListen binds the port and returns the job id at once. Binding happens
// here rather than inside the job, so "that port is already in use" rejects the
// call and the tech sees a field error instead of a job that fails a moment
// later.
func (s *Service) StartListen(p Params) (string, error) {
	opts, err := p.validate()
	if err != nil {
		return "", err
	}

	address := ":" + strconv.Itoa(opts.port)

	var tcpLn net.Listener
	if opts.wantTCP() {
		tcpLn, err = net.Listen("tcp", address)
		if err != nil {
			return "", inUse("TCP", opts.port)
		}
	}

	var udpConn net.PacketConn
	if opts.wantUDP() {
		udpConn, err = net.ListenPacket("udp", address)
		if err != nil {
			// The TCP half must not be left bound when the UDP half refuses.
			if tcpLn != nil {
				tcpLn.Close()
			}
			return "", inUse("UDP", opts.port)
		}
	}

	return s.jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		return runListen(jc, opts, tcpLn, udpConn)
	}), nil
}

func inUse(protocol string, port int) error {
	return core.Errorf(core.CodeNetwork,
		"Something else on this computer is already using %s port %d. Pick another port, or use the Listening Ports tool to see what has it.",
		protocol, port)
}

// runListen is the body of the job, named so it is not stranded at 0% coverage
// inside an anonymous closure.
func runListen(jc *core.JobContext, opts options, tcpLn net.Listener, udpConn net.PacketConn) error {
	sess := newSession(opts, Sink{
		Emit:     func(h Hit) { jc.Emit(KindHit, h) },
		Progress: func(msg string) { jc.Progress(0, 0, msg) },
	})
	err := Serve(jc.Ctx(), tcpLn, udpConn, sess)
	// The summary is set on the way out whatever happened, because Stop is the
	// normal way a listening session ends and the tech still wants the tally.
	jc.SetSummary(sess.summary())
	return err
}

// Serve runs the listeners until the context is cancelled, which is the normal
// way a session ends. It is named rather than an anonymous closure so a test can
// drive it against real loopback sockets and read what it emits.
//
// Either listener may be nil, meaning that protocol was not asked for.
func Serve(ctx context.Context, tcpLn net.Listener, udpConn net.PacketConn, s *session) error {
	s.out.Progress("Listening on " + protocolWords(s.opts.protocol) + " port " + strconv.Itoa(s.opts.port))

	var wg sync.WaitGroup
	if tcpLn != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.acceptTCP(tcpLn)
		}()
	}
	if udpConn != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.readUDP(udpConn)
		}()
	}

	loops := make(chan struct{})
	go func() {
		wg.Wait()
		close(loops)
	}()

	closeAll := func() {
		if tcpLn != nil {
			tcpLn.Close()
		}
		if udpConn != nil {
			udpConn.Close()
		}
	}

	select {
	case <-ctx.Done():
		closeAll()
		<-loops
		return ctx.Err()
	case <-loops:
		// Both loops ended without anyone asking them to, which means the
		// operating system took the socket away.
		closeAll()
		return core.Errorf(core.CodeNetwork,
			"Listening stopped unexpectedly. Check that nothing else took port %d, and start again.", s.opts.port)
	}
}

func (s *session) acceptTCP(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleTCP(conn)
	}
}

// handleTCP greets the client, reads whatever it says, and closes. The greeting
// goes first so a client that connects and immediately closes still gets one,
// and so a scanner's silent connection is still recorded.
func (s *session) handleTCP(conn net.Conn) {
	defer conn.Close()

	peer, port := splitPeer(conn.RemoteAddr().String())
	conn.SetDeadline(time.Now().Add(tcpDeadline))
	io.WriteString(conn, bannerFor(conn.RemoteAddr().String()))

	// ReadAll rather than ReadFull: a client that sends five bytes and closes
	// must be recorded as five bytes, not as a failed read of a full buffer.
	// The deadline set above is what ends a client that connects and says
	// nothing at all, which is the common case for a port scanner.
	sent, _ := io.ReadAll(io.LimitReader(conn, readLimit))

	s.record(Hit{
		Protocol: ProtoTCP, Peer: peer, PeerPort: port,
		Bytes: len(sent), Preview: preview(sent),
	})
}

// readUDP echoes each datagram back to whoever sent it, for the same reason the
// TCP half sends a banner: the person testing from the other machine needs to
// see something on their own screen.
func (s *session) readUDP(pc net.PacketConn) {
	buf := make([]byte, udpDatagram)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		pc.WriteTo(buf[:n], addr)

		peer, port := splitPeer(addr.String())
		s.record(Hit{
			Protocol: ProtoUDP, Peer: peer, PeerPort: port,
			Bytes: n, Preview: preview(buf[:n]),
		})
	}
}

// record tallies an arrival and streams it to the page, up to the cap.
func (s *session) record(h Hit) {
	h.Time = time.Now().UTC().Format(time.RFC3339)

	s.mu.Lock()
	if h.Protocol == ProtoUDP {
		s.udp++
	} else {
		s.tcp++
	}
	s.peers[h.Peer] = struct{}{}
	send := s.emitted < maxHits
	if send {
		s.emitted++
	} else {
		s.dropped++
	}
	tcp, udp, peers := s.tcp, s.udp, len(s.peers)
	s.mu.Unlock()

	if send && s.out.Emit != nil {
		s.out.Emit(h)
	}
	if s.out.Progress != nil {
		s.out.Progress(arrivalLine(tcp+udp, peers))
	}
}

// arrivalLine is the progress text once something has arrived.
func arrivalLine(hits, peers int) string {
	return plural(hits, "arrival", "arrivals") + " from " + plural(peers, "machine", "machines")
}

func plural(n int, singular, many string) string {
	word := many
	if n == 1 {
		word = singular
	}
	return strconv.Itoa(n) + " " + word
}

func (s *session) summary() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return summaryFor(s.opts.port, s.opts.protocol, s.tcp, s.udp, len(s.peers), s.dropped)
}

// splitPeer separates an address from its port, keeping the whole string as the
// address when it has no port rather than dropping it.
func splitPeer(remote string) (string, int) {
	host, port, err := net.SplitHostPort(remote)
	if err != nil {
		return remote, 0
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return host, 0
	}
	return host, n
}
