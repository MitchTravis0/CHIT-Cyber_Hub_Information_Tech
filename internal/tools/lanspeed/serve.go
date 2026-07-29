package lanspeed

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"chit/internal/core"
)

const (
	readHeaderTimeout = 10 * time.Second
	// shutdownGrace is how long Stop waits for a pull in flight before closing
	// the listener anyway. It matches LAN File Drop.
	shutdownGrace = 2 * time.Second
	// progressInterval throttles the progress line. core already coalesces
	// events to about ten a second, so anything shorter is thrown away.
	progressInterval = 200 * time.Millisecond
)

// Sink is where a session reports to. The job wires it to jc.Emit and
// jc.Progress; a test wires it to a slice, which is the only way to read what a
// session emitted without a Wails runtime.
type Sink struct {
	Emit     func(Pull)
	Progress func(sent, total int64, message string)
}

// session is one running test. It holds no state a request can reach: the only
// thing a request influences is a counter and how many bytes go back out.
type session struct {
	opts  options
	token string
	url   string
	out   Sink
	// local is every address a pull could arrive on that means "this same
	// computer", so a mistaken self-test is labelled rather than reported.
	local map[string]bool

	mu       sync.Mutex
	pulls    int
	localHit int
	emitted  int
	best     float64
	bytesOut int64
}

func newSession(opts options, token, url string, local map[string]bool, out Sink) *session {
	if local == nil {
		local = map[string]bool{}
	}
	return &session{opts: opts, token: token, url: url, local: local, out: out}
}

// StartSpeed puts the generated stream on a small web server and returns the job
// id at once. The listener is bound here rather than inside the job, so "that
// port is already in use" rejects the call and the tech sees a field error
// instead of a job that fails a moment later.
func (s *Service) StartSpeed(p Params, primaryIP string) (string, error) {
	opts, err := p.validate()
	if err != nil {
		return "", err
	}

	addresses, err := Addresses(primaryIP)
	if err != nil {
		return "", err
	}
	if len(addresses) == 0 {
		return "", core.Errorf(core.CodeNetwork,
			"This computer has no network address, so there is nowhere for the other machine to connect to. Check that it is on wifi or has a cable plugged in.")
	}

	token, err := newToken()
	if err != nil {
		return "", err
	}

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(opts.port))
	if err != nil {
		return "", core.Errorf(core.CodeNetwork,
			"Something else on this computer is already using port %d. Change the port and start again.", opts.port)
	}

	url := URLFor(addresses[0].IP, opts.port, token)
	sess := newSession(opts, token, url, localSet(addresses), Sink{})

	// core.JobContext does not expose its own id, so the id is handed to the job
	// goroutine down a buffered channel once Start has returned it. The deferred
	// read blocks until it arrives, which is immediately.
	idFor := make(chan string, 1)

	jobID := s.jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		defer func() { s.forget(<-idFor) }()
		return runSpeed(jc, listener, sess)
	})

	idFor <- jobID

	s.remember(jobID, Session{Token: token, Port: opts.port, URL: url, SizeMB: opts.sizeMB})
	return jobID, nil
}

// runSpeed is the body of the job, named so it is not stranded at 0% coverage
// inside an anonymous closure.
func runSpeed(jc *core.JobContext, listener net.Listener, sess *session) error {
	sess.out = Sink{
		Emit: func(p Pull) { jc.Emit(KindPull, p) },
		Progress: func(sent, total int64, message string) {
			jc.Progress(int(sent>>20), int(total>>20), message)
		},
	}
	err := Serve(jc.Ctx(), listener, sess)
	// The summary is set on the way out whatever happened, because Stop is the
	// normal way a test ends and the tech still wants the tally.
	jc.SetSummary(sess.summary())
	return err
}

// Serve runs the HTTP server on the listener until the context is cancelled,
// which is the normal way a test ends. It is named rather than an anonymous
// closure so a test can drive it against a real listener on 127.0.0.1 and read
// what it emits.
func Serve(ctx context.Context, listener net.Listener, s *session) error {
	s.progress(0, 0, "Waiting for the other machine at "+s.url)

	server := &http.Server{
		Handler:           s.handler(ctx),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	done := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return core.Errorf(core.CodeNetwork,
				"The test stopped unexpectedly. Check that nothing else took the port, and start again.")
		}
		return nil
	case <-ctx.Done():
		// A pull in flight gets a moment to finish before the listener goes.
		grace, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = server.Shutdown(grace)
		<-done
		return ctx.Err()
	}
}

// handler has exactly two routes and neither takes anything from the request
// except a token compared in constant time. It is a bare HandlerFunc rather than
// an http.ServeMux for the reason filedrop documents: ServeMux canonicalises the
// path and answers an odd one with a redirect, which puts a second, invisible
// behaviour in front of a surface that should be one flat 404.
func (s *session) handler(jobCtx context.Context) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.EscapedPath(), "/t/")
		if !ok {
			notFound(w)
			return
		}
		token, tail, _ := strings.Cut(rest, "/")
		if !tokenMatches(s.token, token) {
			notFound(w)
			return
		}

		switch tail {
		case "":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, IndexHTML(s.token, s.opts.sizeMB, s.url))
		case "dl":
			s.stream(jobCtx, w, r)
		default:
			notFound(w)
		}
	})
}

// notFound is the one answer to every unknown path and every wrong token, so the
// server never confirms that a session exists.
func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	io.WriteString(w, "Nothing here.\n")
}

// stream writes the generated data and times it. The clock starts after the
// headers are flushed, so the figure is about the link rather than about how
// long the browser took to ask.
func (s *session) stream(jobCtx context.Context, w http.ResponseWriter, r *http.Request) {
	total := s.opts.totalBytes()
	peer := peerOf(r)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(total, 10))
	w.Header().Set("Content-Disposition", `attachment; filename="chit-speedtest.bin"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	started := time.Now()
	lastProgress := started
	var sent int64

	for sent < total {
		if jobCtx.Err() != nil || r.Context().Err() != nil {
			break
		}
		chunk := block
		if remaining := total - sent; remaining < int64(len(chunk)) {
			chunk = chunk[:remaining]
		}
		n, err := w.Write(chunk)
		sent += int64(n)
		if err != nil {
			break
		}
		if now := time.Now(); now.Sub(lastProgress) >= progressInterval {
			lastProgress = now
			s.progress(sent, total, progressLine(peer, sent, total, Mbps(sent, now.Sub(started).Seconds())))
		}
	}

	seconds := time.Since(started).Seconds()
	status := "stopped"
	if sent == total {
		status = "ok"
	}
	mbps := Mbps(sent, seconds)

	reading := Reading(mbps)
	if s.local[peer] {
		reading = SameMachineReading
	}

	s.record(Pull{
		Peer: peer, Bytes: sent, Seconds: seconds, Mbps: mbps,
		Status: status, Reading: reading,
	})
}

// record tallies a pull and streams it to the page, up to the cap.
func (s *session) record(p Pull) {
	p.Time = time.Now().UTC().Format(time.RFC3339)

	s.mu.Lock()
	s.pulls++
	if p.Reading == SameMachineReading {
		s.localHit++
	}
	s.bytesOut += p.Bytes
	if p.Mbps > s.best {
		s.best = p.Mbps
	}
	send := s.emitted < maxPulls
	if send {
		s.emitted++
	}
	s.mu.Unlock()

	if send && s.out.Emit != nil {
		s.out.Emit(p)
	}
	s.progress(0, 0, "Waiting for the other machine at "+s.url)
}

func (s *session) progress(sent, total int64, message string) {
	if s.out.Progress != nil {
		s.out.Progress(sent, total, message)
	}
}

func (s *session) summary() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return summaryFor(s.url, s.opts.port, s.opts.sizeMB, s.pulls, s.localHit, s.best, s.bytesOut)
}

func peerOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
