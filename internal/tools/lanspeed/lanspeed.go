// Package lanspeed measures the transfer speed between this machine and another
// one on the same network. The shipped Speed Test measures the internet, which
// is a different link and a different question: this one answers "copying from
// the server is slow".
//
// CHIT serves a generated stream over HTTP on a token-scoped path and times the
// bytes it wrote to the socket. The other machine needs nothing installed but a
// browser or curl, and nothing is written to disk at either end.
package lanspeed

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"

	"chit/internal/core"
	// filedrop.URLFor and filedrop.Addresses are imported rather than copied, so
	// there is only one address enumerator and one link builder in the app. The
	// repo owner approved this Go cross-tool import before the spec was written,
	// on the terms in SPECS/CONVENTIONS.md 2.3.1. That package is never edited.
	"chit/internal/tools/filedrop"
)

// JobKind is the job manager kind for a throughput session.
const JobKind = "lanspeed"

// KindPull is the result kind of every completed or abandoned pull.
const KindPull = "pull"

const (
	// defaultPort is above 1024 (so no elevation) and distinct from LAN File
	// Drop's 8722 and Port Listener's 8730, so all three can run at once.
	defaultPort = 8740
	minPort     = 1024
	maxPort     = 65535

	defaultSizeMB = 200
	minSizeMB     = 10
	maxSizeMB     = 4096

	// blockBytes is the buffer written repeatedly. 1 MiB is large enough that
	// the per-write syscall is noise and small enough to notice a cancel.
	blockBytes = 1 << 20

	// maxPulls caps the rows emitted, so a page left on a refresh loop cannot
	// fill the table.
	maxPulls = 100

	// tokenBytes is the random path segment, 8 bytes as 16 hex characters.
	tokenBytes = 8
)

// Params is what the UI sends. Every zero value means "use the default".
type Params struct {
	// Port of zero means the default.
	Port int `json:"port"`
	// SizeMB is how much data one pull transfers. Zero means the default.
	SizeMB int `json:"sizeMb"`
}

// Session is what the page needs in order to show the link.
type Session struct {
	Token  string `json:"token"`
	Port   int    `json:"port"`
	URL    string `json:"url"`
	SizeMB int    `json:"sizeMb"`
}

// Pull is one transfer the server handled.
type Pull struct {
	Time  string `json:"time"`
	Peer  string `json:"peer"`
	Bytes int64  `json:"bytes"`
	// Seconds is how long the bytes took to leave this machine.
	Seconds float64 `json:"seconds"`
	Mbps    float64 `json:"mbps"`
	// Status is "ok" when the whole stream went out, "stopped" when the other
	// machine closed the connection part way through.
	Status string `json:"status"`
	// Reading is the plain sentence beside the number.
	Reading string `json:"reading"`
}

// options is the validated form of Params, produced before anything binds.
type options struct {
	port   int
	sizeMB int
}

func (o options) totalBytes() int64 { return int64(o.sizeMB) << 20 }

// validate catches everything a user can get wrong, so bad input rejects the
// StartSpeed call instead of binding a port that immediately fails.
func (p Params) validate() (options, error) {
	opts := options{port: p.Port, sizeMB: p.SizeMB}

	if opts.port == 0 {
		opts.port = defaultPort
	}
	if opts.port < minPort || opts.port > maxPort {
		return opts, core.Errorf(core.CodeInvalidInput,
			"The port must be between %d and %d. Below %d needs administrator rights, which CHIT never asks for.",
			minPort, maxPort, minPort)
	}

	if opts.sizeMB == 0 {
		opts.sizeMB = defaultSizeMB
	}
	if opts.sizeMB < minSizeMB || opts.sizeMB > maxSizeMB {
		return opts, core.Errorf(core.CodeInvalidInput,
			"The test size must be between %d MB and %d MB. %d MB is enough to tell a gigabit link from a 100 Mbps one.",
			minSizeMB, maxSizeMB, defaultSizeMB)
	}
	return opts, nil
}

// newToken is the random path segment that scopes a session.
func newToken() (string, error) {
	var b [tokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", core.Errorf(core.CodeInternal,
			"CHIT could not create a private link for this test. Try again.")
	}
	return hex.EncodeToString(b[:]), nil
}

// tokenMatches compares in constant time, and a wrong token gets the same 404 as
// a wrong path, so the server never confirms a session exists.
func tokenMatches(want, got string) bool {
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// block is the buffer written over and over. It is filled with a repeating
// pattern rather than zeroes because a run of zeroes is exactly what a WAN
// accelerator or a proxy would collapse, and the measurement would then be of
// the middlebox. It is deliberately not random: this is a throughput test, not a
// cryptographic one, and a fixed pattern keeps the suite deterministic.
var block = func() []byte {
	b := make([]byte, blockBytes)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}()

// Mbps is megabits per second: bytes to bits, then to millions, then per second.
// Megabits are decimal, which is why this is 1e6 and not a shift.
func Mbps(bytes int64, seconds float64) float64 {
	if seconds <= 0 || bytes <= 0 {
		return 0
	}
	return float64(bytes) * 8 / 1e6 / seconds
}

// Reading is the plain sentence beside the number. Its thresholds are pinned by
// testdata/lanspeed-readings.json, which the TypeScript side reads too.
func Reading(mbps float64) string {
	switch {
	case mbps >= 700:
		return "Gigabit or better."
	case mbps >= 200:
		return "Fast, but short of a full gigabit link."
	case mbps >= 70:
		return "About 100 Mbps. Check for a 100 Mbps switch port or a damaged cable."
	case mbps >= 20:
		return "Wi-Fi speeds, or a busy link."
	case mbps > 0:
		return "Very slow for a local network. Check the cable, the port and what else is using the link."
	}
	return "Too little was transferred to measure."
}

// SameMachineReading replaces the speed reading when the pull came from this
// computer rather than from another one. Found by running the tool: pulling the
// link on the machine that is serving it reports tens of thousands of Mbps and
// reads "Gigabit or better", which is memory speed and not a network figure at
// all. A number that looks like a pass and means nothing is worse than no
// number.
const SameMachineReading = "That was this same computer, so this is memory speed and not a network figure. Open the link on the other machine."

// sessionNote explains a session in which nothing happened, one in which only
// this machine ever pulled, and labels every number as approximate otherwise.
func sessionNote(pulls, local int) string {
	if pulls == 0 {
		return "Nothing pulled the test file. Check that both machines are on the same network, that you typed the address exactly, and that this computer's firewall let CHIT through."
	}
	if local == pulls {
		return "Every pull came from this same computer, so none of these numbers describes the network. Open the link on the machine at the other end."
	}
	return "These numbers are approximate. They measure what this computer could push down the link while it was also doing everything else, and they describe the direction from this machine to the other one only."
}

// summaryFor is the job:done summary. Its keys are a contract with the page and
// are checked nowhere by the compiler, in either language, so they are built in
// one named place that a test can pin.
func summaryFor(url string, port, sizeMB, pulls, local int, best float64, out int64) map[string]any {
	return map[string]any{
		"url":      url,
		"port":     port,
		"sizeMb":   sizeMB,
		"pulls":    pulls,
		"bestMbps": best,
		"bytesOut": out,
		"note":     sessionNote(pulls, local),
	}
}

// localSet is every address a pull could arrive on that would mean "this same
// computer", so a mistaken self-test is labelled rather than reported as a
// result.
func localSet(addresses []filedrop.Address) map[string]bool {
	out := map[string]bool{"127.0.0.1": true, "::1": true}
	for _, a := range addresses {
		out[a.IP] = true
	}
	return out
}

// progressLine is the text under the bar while a pull is in flight.
func progressLine(peer string, sent, total int64, mbps float64) string {
	return "Sending to " + peer + ": " +
		strconv.FormatInt(sent>>20, 10) + " MB of " + strconv.FormatInt(total>>20, 10) +
		" MB at " + strconv.FormatFloat(mbps, 'f', 0, 64) + " Mbps"
}

// Service owns the throughput entry points and holds the live sessions, so the
// page can ask for the link of the job it just started. app.go is frozen and
// cannot carry a field, so the service is reached through a sync.Once accessor,
// exactly as totp and filedrop do.
type Service struct {
	jobs *core.JobManager

	mu   sync.Mutex
	live map[string]Session
}

var (
	sharedOnce sync.Once
	shared     *Service
)

// Shared returns the one Service instance. A new one per bound call would
// forget every session as soon as it was created.
func Shared(jobs *core.JobManager) *Service {
	sharedOnce.Do(func() {
		shared = &Service{jobs: jobs, live: map[string]Session{}}
	})
	return shared
}

// SessionFor returns the session a job id is running, or an empty Session when
// that job is not a throughput test or is no longer running.
func (s *Service) SessionFor(jobID string) Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live[jobID]
}

func (s *Service) remember(jobID string, session Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[jobID] = session
}

func (s *Service) forget(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.live, jobID)
}

// curlFor is the command printed on the served page, so the tech can run the
// test without a browser.
func curlFor(url string) string {
	return "curl -o " + nullDevice + " " + url + "/dl"
}

// nullDevice is written for the machine at the other end, which is far more
// likely to be a Linux or macOS box with a terminal open than a Windows one.
const nullDevice = "/dev/null"

// Addresses lists the addresses another machine could reach this one on. It
// forwards to the shipped enumerator rather than being a second one: that
// function walks every interface and puts the routed address first, and two
// copies of it would be free to disagree about which address to offer.
func Addresses(primaryIP string) ([]filedrop.Address, error) {
	return filedrop.Addresses(primaryIP)
}

// URLFor builds the link to read out. filedrop.URLFor is deliberately not
// reused here even though the shape is the same: its path segment is "/d/" and
// rewriting the string it returns would be worse than the four lines below.
// Bracketing an IPv6 literal is not the kind of thing SPECS/CONVENTIONS.md
// 2.3.1 means by "copying it would be a real fork".
func URLFor(ip string, port int, token string) string {
	host := ip
	if strings.Contains(ip, ":") {
		host = "[" + ip + "]"
	}
	return "http://" + host + ":" + strconv.Itoa(port) + "/t/" + token
}
