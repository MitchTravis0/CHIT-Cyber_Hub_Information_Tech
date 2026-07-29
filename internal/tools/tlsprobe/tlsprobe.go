// Package tlsprobe tries one TLS handshake per protocol version against a
// server and reports which versions it accepts. It is the service layer the
// TLS Handshake Prober page talks to.
package tlsprobe

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"time"

	"chit/internal/core"
)

const (
	// DefaultTimeoutMS is the budget for one handshake, not for the whole run.
	DefaultTimeoutMS = 5000
	minTimeoutMS     = 500
	maxTimeoutMS     = 30000
	// workers is the number of testable versions, so all four are tried at once
	// and a dead host costs one timeout rather than four.
	workers = 4
)

// Level values, shared with the other Phase 8 shape B tools.
const (
	LevelOK    = "ok"
	LevelWarn  = "warn"
	LevelError = "error"
)

// notTestableMessage is why SSL 3.0 has no answer. Saying nothing at all would
// let a tech assume it was tested and refused.
const notTestableMessage = "CHIT cannot test SSL 3.0. Go's TLS library does not implement it, so no answer here is better than a guessed one."

type Params struct {
	// Target is host, host:port, or https://host[:port]. Port defaults to 443.
	Target    string `json:"target"`
	TimeoutMS int    `json:"timeoutMs"`
}

// Attempt is one protocol version tried against the server.
type Attempt struct {
	Version string `json:"version"`
	// Testable is false only for SSL 3.0, which Go cannot speak.
	Testable bool `json:"testable"`
	// Accepted is true when the handshake completed at exactly this version.
	Accepted bool   `json:"accepted"`
	Cipher   string `json:"cipher"`
	// ALPN is the agreed application protocol ("h2", "http/1.1"), "" when none.
	ALPN string `json:"alpn"`
	// Message is one sentence, always set.
	Message string `json:"message"`
	// HandshakeMS is how long the attempt took, 0 when not testable.
	HandshakeMS float64 `json:"handshakeMs"`
}

type Report struct {
	Target   string    `json:"target"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	IP       string    `json:"ip"`
	Attempts []Attempt `json:"attempts"`
	Level    string    `json:"level"`
	Headline string    `json:"headline"`
	// Advice is the next step, "" when there is nothing to do.
	Advice    string `json:"advice"`
	CheckedAt string `json:"checkedAt"`
}

// version is one row of the table, in the order the page shows them: oldest
// first, because the question is nearly always "how old can a client be".
type version struct {
	name string
	// id is 0 for a version Go cannot speak.
	id uint16
}

func versions() []version {
	return []version{
		{name: "SSL 3.0", id: 0},
		{name: "TLS 1.0", id: tls.VersionTLS10},
		{name: "TLS 1.1", id: tls.VersionTLS11},
		{name: "TLS 1.2", id: tls.VersionTLS12},
		{name: "TLS 1.3", id: tls.VersionTLS13},
	}
}

func normalizeTimeout(ms int) (int, error) {
	if ms == 0 {
		return DefaultTimeoutMS, nil
	}
	if ms < minTimeoutMS || ms > maxTimeoutMS {
		return 0, core.Errorf(core.CodeInvalidInput,
			"The wait per handshake must be between 0.5 and 30 seconds. %d ms is outside that.", ms)
	}
	return ms, nil
}

// Probe tries every TLS version against p.Target. A server that refuses
// everything is a successful probe with a bad answer, so it comes back as a
// Report with Level "error"; only bad parameters produce a Go error.
func Probe(ctx context.Context, p Params) (Report, error) {
	tgt, err := parseTarget(p.Target)
	if err != nil {
		return Report{}, err
	}
	timeoutMS, err := normalizeTimeout(p.TimeoutMS)
	if err != nil {
		return Report{}, err
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond

	// Resolving up front turns "the name is wrong" into a field error rather
	// than five identical handshake failures.
	ips, err := net.DefaultResolver.LookupHost(ctx, tgt.host)
	if err != nil {
		return Report{}, core.Errorf(core.CodeNotFound,
			"Could not find a computer called %s. Check the spelling, or use its IP address.", tgt.host)
	}

	all := versions()
	attempts := make([]Attempt, len(all))
	type indexed struct {
		i int
		a Attempt
	}
	for r := range core.Pool(ctx, indexRange(len(all)), workers, func(c context.Context, i int) (indexed, bool) {
		return indexed{i: i, a: try(c, tgt, all[i], timeout, timeoutMS)}, true
	}) {
		attempts[r.i] = r.a
	}
	if ctx.Err() != nil {
		return Report{}, core.Errorf(core.CodeInternal,
			"CHIT was closing down, so the probe did not finish. Try it again.")
	}

	report := Report{
		Target:    p.Target,
		Host:      tgt.host,
		Port:      tgt.port,
		IP:        ips[0],
		Attempts:  attempts,
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	report.Level, report.Headline, report.Advice = Classify(tgt.host, tgt.port, attempts)
	return report, nil
}

// indexRange exists because core.Pool works over a slice and the rows must come
// back in version order, not in the order the handshakes finished.
func indexRange(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// try is one handshake pinned to exactly one version.
func try(ctx context.Context, tgt target, v version, timeout time.Duration, timeoutMS int) Attempt {
	out := Attempt{Version: v.name, Testable: v.id != 0}
	if !out.Testable {
		out.Message = notTestableMessage
		return out
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cfg := &tls.Config{
		ServerName: tgt.host,
		MinVersion: v.id,
		MaxVersion: v.id,
		// The certificate is deliberately not verified: this tool reports
		// protocol versions, and refusing a self-signed certificate would hide
		// the answer on exactly the equipment it exists to diagnose.
		InsecureSkipVerify: true,
		// Every suite, including the ones Go considers insecure. A server that
		// only offers 3DES on TLS 1.0 still accepts TLS 1.0, and reporting it as
		// refused would be wrong.
		CipherSuites: allCipherSuites(),
		NextProtos:   []string{"h2", "http/1.1"},
	}

	started := time.Now()
	d := &tls.Dialer{NetDialer: &net.Dialer{}, Config: cfg}
	conn, err := d.DialContext(ctx, "tcp", tgt.address())
	elapsed := milliseconds(time.Since(started))
	if err != nil {
		out.Message = failureMessage(v.name, err, timeoutMS)
		return out
	}
	defer conn.Close()

	state := conn.(*tls.Conn).ConnectionState()
	out.Accepted = true
	out.HandshakeMS = elapsed
	out.Cipher = tls.CipherSuiteName(state.CipherSuite)
	out.ALPN = state.NegotiatedProtocol
	out.Message = fmt.Sprintf("The server accepted %s.", v.name)
	return out
}

// allCipherSuites is every suite Go can offer, secure and insecure together.
// Go's default list drops the old ones, which would make an old server read as
// refusing a version it actually accepts.
func allCipherSuites() []uint16 {
	secure, insecure := tls.CipherSuites(), tls.InsecureCipherSuites()
	out := make([]uint16, 0, len(secure)+len(insecure))
	for _, s := range secure {
		out = append(out, s.ID)
	}
	for _, s := range insecure {
		out = append(out, s.ID)
	}
	return out
}

// failureMessage turns a handshake error into a sentence. No stdlib TLS wording
// ever reaches the screen through here.
func failureMessage(name string, err error, timeoutMS int) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), isTimeout(err):
		return fmt.Sprintf(
			"%s got no answer within %d ms, while another version did answer. Treat this as refused.",
			name, timeoutMS)
	case errors.Is(err, io.EOF), isReset(err):
		return fmt.Sprintf(
			"The server closed the connection during the %s handshake, which is how some servers refuse a version.",
			name)
	default:
		return fmt.Sprintf("The server refused %s.", name)
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func isReset(err error) bool {
	// Checked as text because the reset arrives wrapped differently on each OS
	// and the exact syscall error is not worth three build-tagged files here.
	return strings.Contains(err.Error(), "connection reset")
}

// Classify turns the five rows into the one line at the top of the page.
func Classify(host string, port int, attempts []Attempt) (level, headline, advice string) {
	accepted := make([]string, 0, len(attempts))
	old, modern := false, false
	for _, a := range attempts {
		if !a.Accepted {
			continue
		}
		accepted = append(accepted, a.Version)
		if a.Version == "TLS 1.0" || a.Version == "TLS 1.1" {
			old = true
		} else {
			modern = true
		}
	}

	switch {
	case len(accepted) == 0:
		return LevelError,
			fmt.Sprintf("Nothing answered on %s port %d, so no TLS version could be tested.", host, port),
			"Check the port number and whether a firewall is in the way. This is a connection problem, not a TLS one."
	case old && modern:
		return LevelWarn,
			fmt.Sprintf("This server still accepts %s, which most modern clients now refuse.", accepted[0]),
			"Old equipment will keep working. Plan to turn the old versions off once it is replaced."
	case old:
		return LevelError,
			fmt.Sprintf("This server accepts only %s. Modern browsers and operating systems refuse both TLS 1.0 and TLS 1.1.", joinVersions(accepted)),
			"The server needs TLS 1.2 enabled before current clients will connect."
	default:
		return LevelOK,
			fmt.Sprintf("This server accepts %s.", joinVersions(accepted)),
			"Anything that can only speak TLS 1.0 or 1.1 will not connect to it."
	}
}

// joinVersions writes a list the way a person says it: "A", "A and B",
// "A, B and C".
func joinVersions(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

func milliseconds(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}
