// Package pingmon pings a handful of hosts on a repeating interval and streams
// one sample per host per round. It is the service layer the Ping Monitor page
// talks to.
package pingmon

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"chit/internal/core"
)

// JobKind is the job manager kind for a ping monitor run.
const JobKind = "pingmon"

// KindSample is the result kind every Sample is emitted under. A job emits this
// kind and no other: useJob ignores the batch kind, so a second kind would be
// mixed into the same results array.
const KindSample = "sample"

// How a reply was measured.
const (
	ViaICMP = "icmp"
	ViaTCP  = "tcp"
)

// MaxTargets is the number of hosts one run can watch. The chart has four
// distinct theme colours, so four is the limit.
const MaxTargets = 4

// MaxRounds stops a forgotten monitor after twelve hours at the default
// interval, so the results array cannot grow without bound.
const MaxRounds = 43200

const (
	defaultIntervalMS = 1000
	minIntervalMS     = 200
	maxIntervalMS     = 60000

	defaultTimeoutMS = 1000
	minTimeoutMS     = 100
	maxTimeoutMS     = 10000

	defaultTCPPort = 443

	maxTargetLength = 253
	maxLabelLength  = 63

	// resolveBudget bounds the one round of name lookups done at the start, so
	// a dead DNS server cannot hold the run up for minutes.
	resolveBudget = 5 * time.Second
)

// icmpFailureBudget is how many echo requests may fail outright before the
// monitor stops trying. A blocked ICMP socket fails instantly every time, and
// retrying it once a second for an hour tells nobody anything.
const icmpFailureBudget = 3

// Params is what the UI sends. Every zero value means "use the default".
type Params struct {
	Targets    []string `json:"targets"`
	IntervalMS int      `json:"intervalMs"`
	TimeoutMS  int      `json:"timeoutMs"`
	// Rounds is how many times to ping every target. 0 means keep going until
	// the user stops it.
	Rounds int `json:"rounds"`
	// TCPPort is the port used to measure a round trip when ICMP is blocked.
	TCPPort int `json:"tcpPort"`
	// SkipTCP turns that fallback off, leaving ICMP only.
	SkipTCP bool `json:"skipTcp"`
}

// Sample is one ping of one target in one round.
type Sample struct {
	Round     int     `json:"round"`
	Target    string  `json:"target"`
	IP        string  `json:"ip"`
	OK        bool    `json:"ok"`
	LatencyMS float64 `json:"latencyMs"`
	Via       string  `json:"via"`
	// At is the wall clock time the sample was taken, in unix milliseconds, so
	// the drop log can show when it happened.
	At int64 `json:"at"`
	// Reason is empty when OK, otherwise a sentence explaining the miss.
	Reason string `json:"reason"`
}

// settings is the validated form of Params.
type settings struct {
	Targets   []string
	Interval  time.Duration
	Timeout   time.Duration
	TimeoutMS int
	Rounds    int
	TCPPort   int
	SkipTCP   bool
}

// normalize validates the parameters and applies the defaults. Everything a
// user can get wrong is caught here rather than inside the job, so a bad host
// list rejects the StartMonitor call instead of starting a run that
// immediately fails.
func (p Params) normalize() (settings, error) {
	targets, err := cleanTargets(p.Targets)
	if err != nil {
		return settings{}, err
	}

	interval := p.IntervalMS
	if interval == 0 {
		interval = defaultIntervalMS
	}
	if interval < minIntervalMS || interval > maxIntervalMS {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"Ping every 0.2 to 60 seconds. %d ms is outside that.", p.IntervalMS)
	}

	timeout := p.TimeoutMS
	if timeout == 0 {
		timeout = defaultTimeoutMS
	}
	if timeout < minTimeoutMS || timeout > maxTimeoutMS {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"The wait for a reply must be between 0.1 and 10 seconds. %d ms is outside that.", p.TimeoutMS)
	}

	if p.Rounds < 0 || p.Rounds > MaxRounds {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"Stop after at most 43200 pings, which is twelve hours at one a second.")
	}

	port := p.TCPPort
	if port == 0 {
		port = defaultTCPPort
	}
	if port < 1 || port > 65535 {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"%d is not a port number. Ports run from 1 to 65535.", p.TCPPort)
	}

	return settings{
		Targets:   targets,
		Interval:  time.Duration(interval) * time.Millisecond,
		Timeout:   time.Duration(timeout) * time.Millisecond,
		TimeoutMS: timeout,
		Rounds:    p.Rounds,
		TCPPort:   port,
		SkipTCP:   p.SkipTCP,
	}, nil
}

// cleanTargets trims, de-duplicates and syntax checks the host list. Two
// spellings of the same name are one host to watch, not two.
func cleanTargets(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}

	if len(out) == 0 {
		return nil, core.Errorf(core.CodeInvalidInput,
			"Enter at least one host to ping, for example 192.168.1.1 or google.com.")
	}
	if len(out) > MaxTargets {
		return nil, core.Errorf(core.CodeInvalidInput,
			"Watch at most 4 hosts at a time. You gave %d.", len(out))
	}
	for _, t := range out {
		if !validTarget(t) {
			return nil, core.Errorf(core.CodeInvalidInput,
				"%q is not a host name or an IP address. Try 192.168.1.1 or google.com.", t)
		}
	}
	return out, nil
}

// validTarget accepts an address literal or something shaped like a host name.
// The name is not resolved here: that needs the network and would make
// StartPingMonitor block.
func validTarget(t string) bool {
	if t == "" || len(t) > maxTargetLength {
		return false
	}
	if _, err := netip.ParseAddr(t); err == nil {
		return true
	}
	for _, label := range strings.Split(t, ".") {
		if len(label) == 0 || len(label) > maxLabelLength {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
			default:
				return false
			}
		}
	}
	return true
}

// Service owns the monitoring entry points. The App struct forwards its bound
// methods to it.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartMonitor begins a run and returns the job id at once. Samples arrive as
// "sample" items on job:result, one per target per round.
func (s *Service) StartMonitor(p Params) (string, error) {
	cfg, err := p.normalize()
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, cfg.Rounds, func(jc *core.JobContext) error {
		return monitorRun(jc, cfg)
	}), nil
}

// target is one host being watched, with the address it resolved to once.
type target struct {
	name string
	ip   string
}

type monitor struct {
	cfg     settings
	targets []*target

	icmpFails atomic.Int32
	icmpOK    atomic.Bool

	rounds         int
	stoppedAtLimit bool
}

func newMonitor(cfg settings) *monitor {
	m := &monitor{cfg: cfg, targets: make([]*target, 0, len(cfg.Targets))}
	for _, name := range cfg.Targets {
		m.targets = append(m.targets, &target{name: name})
	}
	return m
}

func monitorRun(jc *core.JobContext, cfg settings) error {
	m := newMonitor(cfg)
	if err := m.run(jc); err != nil {
		return err
	}
	jc.SetSummary(m.summary())
	return nil
}

// run is separate from monitorRun so a test can hold the monitor and read the
// tally it built.
func (m *monitor) run(jc *core.JobContext) error {
	cfg := m.cfg
	ctx := jc.Ctx()
	if err := m.resolveTargets(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for round := 1; ; round++ {
		replies := 0
		for s := range core.Pool(ctx, m.targets, len(m.targets), m.probeRound(round)) {
			if s.OK {
				replies++
			}
			m.record(s)
			jc.Emit(KindSample, s)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		jc.Progress(round, cfg.Rounds, fmt.Sprintf("Round %d: %d of %d answered", round, replies, len(m.targets)))

		if cfg.Rounds > 0 && round >= cfg.Rounds {
			break
		}
		if round >= MaxRounds {
			m.stoppedAtLimit = true
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// resolveTargets looks every name up once, at the start, so a round is pure
// measurement. A name that cannot be looked up still produces a sample per
// round, which is the answer a tech needs when DNS is the broken thing.
func (m *monitor) resolveTargets(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, resolveBudget)
	defer cancel()

	found := 0
	for _, t := range m.targets {
		if addr, err := netip.ParseAddr(t.name); err == nil {
			t.ip = addr.String()
			found++
			continue
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", t.name)
		if err != nil || len(ips) == 0 {
			continue
		}
		t.ip = preferIPv4(ips)
		found++
	}
	if found == 0 {
		return core.Errorf(core.CodeNotFound,
			"None of those hosts could be found. Check the spelling, or enter IP addresses instead.")
	}
	return nil
}

// preferIPv4 picks the address a tech expects to see, since a host with both
// records is nearly always reached over IPv4 on a small network.
func preferIPv4(ips []net.IP) string {
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String()
		}
	}
	return ips[0].String()
}

func (m *monitor) probeRound(round int) func(context.Context, *target) (Sample, bool) {
	return func(ctx context.Context, t *target) (Sample, bool) {
		return m.probe(ctx, round, t), true
	}
}

func (m *monitor) probe(ctx context.Context, round int, t *target) Sample {
	s := Sample{Round: round, Target: t.name, IP: t.ip, At: time.Now().UnixMilli()}
	if t.ip == "" {
		s.Reason = missReason(false, false, m.cfg.SkipTCP, m.cfg.TimeoutMS)
		return s
	}

	if m.icmpUsable() {
		rtt, ok, err := pingOnce(ctx, t.ip, m.cfg.Timeout)
		switch {
		case ok:
			m.icmpOK.Store(true)
			s.OK, s.Via, s.LatencyMS = true, ViaICMP, milliseconds(rtt)
			return s
		case err != nil && ctx.Err() == nil:
			m.icmpFails.Add(1)
		}
	}

	if !m.cfg.SkipTCP && ctx.Err() == nil {
		if rtt, ok := tcpPing(ctx, t.ip, m.cfg.TCPPort, m.cfg.Timeout); ok {
			s.OK, s.Via, s.LatencyMS = true, ViaTCP, milliseconds(rtt)
			return s
		}
	}

	s.Reason = missReason(true, m.icmpBlocked(), m.cfg.SkipTCP, m.cfg.TimeoutMS)
	return s
}

// icmpUsable stops the ping stage once it is clear this OS will not let the
// process send echo requests.
func (m *monitor) icmpUsable() bool {
	return m.icmpOK.Load() || m.icmpFails.Load() < icmpFailureBudget
}

// icmpBlocked is the same rule the IP Range Scanner uses: a host that ignores
// ping says nothing about this machine's permissions, so only outright send
// failures count.
func (m *monitor) icmpBlocked() bool {
	return !m.icmpOK.Load() && m.icmpFails.Load() >= icmpFailureBudget
}

func (m *monitor) record(s Sample) {
	if s.Round > m.rounds {
		m.rounds = s.Round
	}
}

func (m *monitor) summary() map[string]any {
	blocked := m.icmpBlocked()
	return map[string]any{
		"rounds":         m.rounds,
		"targets":        len(m.targets),
		"icmpAvailable":  !blocked,
		"stoppedAtLimit": m.stoppedAtLimit,
		"note":           noteFor(blocked, m.cfg.SkipTCP, m.stoppedAtLimit, m.cfg.TCPPort),
	}
}

// missReason explains a ping that brought nothing back, in the words a junior
// tech can act on.
func missReason(resolved, icmpBlocked, skipTCP bool, timeoutMS int) string {
	switch {
	case !resolved:
		return "This name could not be looked up, so it could not be pinged."
	case icmpBlocked && skipTCP:
		return "This computer cannot send ping packets and the TCP fallback is off."
	default:
		return fmt.Sprintf("No reply within %d ms.", timeoutMS)
	}
}

// noteFor is the explanation shown under the summary when the run had to fall
// back or stopped on its own.
func noteFor(icmpBlocked, skipTCP, stoppedAtLimit bool, tcpPort int) string {
	var notes []string
	if icmpBlocked && !skipTCP {
		notes = append(notes, fmt.Sprintf("This computer is not allowed to send ping (ICMP) packets, so the times shown are how long a TCP connection to port %d took instead. They read a little higher than a real ping.", tcpPort))
	}
	if icmpBlocked && skipTCP {
		notes = append(notes, "This computer is not allowed to send ping (ICMP) packets and the TCP fallback is switched off, so nothing could be measured. Turn the TCP fallback back on in the options.")
	}
	if stoppedAtLimit {
		notes = append(notes, fmt.Sprintf("The monitor stopped on its own after %d pings, which is the twelve hour limit. Start it again to keep watching.", MaxRounds))
	}
	return strings.Join(notes, " ")
}
