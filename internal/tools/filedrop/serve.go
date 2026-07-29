package filedrop

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"chit/internal/core"
)

// Sink is where a session reports to. The job wires it to jc.Emit; a test wires
// it to a slice, which is the only way to read what a session emitted without a
// Wails runtime.
type Sink struct {
	Emit func(Transfer)
}

// session is one running drop. It holds no state a request can reach except the
// share list resolved before the listener opened.
type session struct {
	opts  options
	token string
	out   Sink

	mu        sync.Mutex
	downloads int
	uploads   int
	bytesOut  int64
	bytesIn   int64
}

// StartDrop puts the chosen files on a small web server and returns the job id
// at once. The listener is bound here rather than inside the job, so "that port
// is already in use" rejects the call and the tech sees a field error instead of
// a job that fails a moment later.
func (s *Service) StartDrop(p Params, primaryIP string) (string, error) {
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
	sess := &session{opts: opts, token: token}

	// core.JobContext does not expose its own id, so the id is handed to the
	// job goroutine down a buffered channel once Start has returned it. The
	// deferred read blocks until it arrives, which is immediately.
	idFor := make(chan string, 1)

	jobID := s.jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		defer func() { s.forget(<-idFor) }()
		sess.out = Sink{Emit: func(t Transfer) { jc.Emit(KindTransfer, t) }}
		jc.Progress(0, 0, "Sharing "+strconv.Itoa(len(opts.shares))+" files at "+url)

		err := Serve(jc.Ctx(), listener, sess)
		// The summary is set on the way out whatever happened, because Stop is
		// the normal way a drop ends and the tech still wants the tally.
		jc.SetSummary(sess.summary(url))
		return err
	})

	idFor <- jobID

	s.remember(jobID, Session{
		Token: token, Port: opts.port, URL: url, Files: len(opts.shares),
	})
	return jobID, nil
}

func (s *session) summary(url string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return summaryFor(url, s.opts.port, len(s.opts.shares), s.downloads, s.uploads, s.bytesOut, s.bytesIn)
}

// Serve runs the HTTP server on the listener until the context is cancelled,
// which is the normal way a drop ends. It is named rather than an anonymous
// closure so a test can drive it against a real listener on 127.0.0.1 and read
// what it emits.
func Serve(ctx context.Context, listener net.Listener, s *session) error {
	server := &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
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
				"Sharing stopped unexpectedly. Check that nothing else took the port, and start again.")
		}
		return nil
	case <-ctx.Done():
		// A transfer in flight gets a moment to finish before the listener goes.
		grace, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = server.Shutdown(grace)
		<-done
		return ctx.Err()
	}
}

// handler has exactly four routes and none of them takes a filesystem path from
// the request. A file is served by its index into a slice resolved before the
// listener opened, so path traversal is not defended against, it is
// unrepresentable.
//
// This is a bare HandlerFunc rather than an http.ServeMux on purpose. ServeMux
// canonicalises the path and answers "/d/tok/f/../../etc/passwd" with a 307
// redirect to the cleaned path, which is harmless here (the redirect lands on
// another 404) but means a security-critical surface has a second, invisible
// behaviour in front of it. Everything unknown should be one flat 404.
func (s *session) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.EscapedPath(), "/d/")
		if !ok {
			notFound(w)
			return
		}
		token, tail, _ := strings.Cut(rest, "/")
		if !tokenMatches(s.token, token) {
			notFound(w)
			return
		}

		switch {
		case tail == "":
			s.serveIndex(w)
		case strings.HasPrefix(tail, "f/"):
			s.serveFile(w, r, strings.TrimPrefix(tail, "f/"))
		case tail == "up":
			s.receive(w, r)
		default:
			notFound(w)
		}
	})
}

// notFound is the one answer to every unknown path and every wrong token, so
// the server never confirms that a session exists.
func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	io.WriteString(w, "Nothing here.\n")
}

func (s *session) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, IndexHTML(s.token, s.opts.shares, s.opts.allowUpload))
}

// serveFile takes a decimal index, never a name and never a path.
func (s *session) serveFile(w http.ResponseWriter, r *http.Request, index string) {
	at, ok := shareIndex(index, len(s.opts.shares))
	if !ok {
		notFound(w)
		return
	}
	share := s.opts.shares[at]

	file, err := os.Open(share.Path)
	if err != nil {
		s.record(Transfer{
			Direction: "download", Name: share.Name, Peer: peerOf(r), Status: "failed",
			Message: "That file could not be read any more. It may have been moved or deleted since sharing started.",
		})
		notFound(w)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(share.Bytes, 10))
	// The name is quoted and stripped of anything that could break the header.
	w.Header().Set("Content-Disposition", `attachment; filename="`+headerSafe(share.Name)+`"`)

	sent, err := io.Copy(w, file)
	if err != nil || sent != share.Bytes {
		s.record(Transfer{
			Direction: "download", Name: share.Name, Bytes: sent, Peer: peerOf(r),
			Status: "failed", Message: "The other machine stopped the download part way through.",
		})
		return
	}
	s.record(Transfer{
		Direction: "download", Name: share.Name, Bytes: sent, Peer: peerOf(r), Status: "ok",
	})
}

// receive streams one uploaded part straight to disk. r.ParseMultipartForm is
// deliberately not used: it buffers the whole body into the operating system's
// temp folder first, which for a 4 GB file is a second copy nobody asked for.
func (s *session) receive(w http.ResponseWriter, r *http.Request) {
	if !s.opts.allowUpload || r.Method != http.MethodPost {
		notFound(w)
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		s.failUpload(w, r, "", "The upload did not contain a file.")
		return
	}

	for {
		part, err := reader.NextPart()
		if err != nil {
			s.failUpload(w, r, "", "The upload did not contain a file.")
			return
		}
		if part.FileName() == "" {
			part.Close()
			continue
		}

		name := uniqueName(s.opts.receiveDir, sanitizeName(part.FileName()))
		path := filepath.Join(s.opts.receiveDir, name)

		file, err := os.Create(path)
		if err != nil {
			part.Close()
			s.failUpload(w, r, name, "CHIT could not write that file into the folder you chose.")
			return
		}

		written, tooBig, copyErr := copyLimited(file, part, s.opts.uploadLimit)
		file.Close()
		part.Close()

		if tooBig {
			os.Remove(path)
			s.failUpload(w, r, name,
				"That file is larger than "+strconv.FormatInt(s.opts.uploadLimit>>30, 10)+" GB, which is the most CHIT will accept in one go.")
			return
		}
		if copyErr != nil {
			os.Remove(path)
			s.failUpload(w, r, name, "The upload stopped part way through, so nothing was kept.")
			return
		}

		s.record(Transfer{
			Direction: "upload", Name: name, Bytes: written, Peer: peerOf(r), Status: "ok",
		})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "Sent. You can close this page.\n")
		return
	}
}

func (s *session) failUpload(w http.ResponseWriter, r *http.Request, name, message string) {
	s.record(Transfer{
		Direction: "upload", Name: name, Peer: peerOf(r), Status: "failed", Message: message,
	})
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	io.WriteString(w, message+"\n")
}

// record tallies a transfer and streams it to the page.
func (s *session) record(t Transfer) {
	t.Time = time.Now().UTC().Format(time.RFC3339)

	s.mu.Lock()
	if t.Status == "ok" {
		if t.Direction == "download" {
			s.downloads++
			s.bytesOut += t.Bytes
		} else {
			s.uploads++
			s.bytesIn += t.Bytes
		}
	}
	s.mu.Unlock()

	if s.out.Emit != nil {
		s.out.Emit(t)
	}
}

// shareIndex accepts only plain decimal digits. strconv.Atoi on its own is
// lenient enough to take "+0" and " 0", which can only ever resolve to a valid
// index and so is harmless, but a route that quietly accepts more shapes than
// it documents is one somebody has to reason about later.
func shareIndex(text string, count int) (int, bool) {
	if text == "" || len(text) > 4 {
		return 0, false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, false
		}
	}
	at, err := strconv.Atoi(text)
	if err != nil || at < 0 || at >= count {
		return 0, false
	}
	return at, true
}

func peerOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// headerSafe strips the characters that would end the quoted string early or
// inject another header.
func headerSafe(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == '"' || r == '\\' || r == 0x7f {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
