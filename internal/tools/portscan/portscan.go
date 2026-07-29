// Package portscan checks which TCP ports on one host accept a connection. It
// is the service layer the Port Scanner page talks to.
package portscan

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"chit/internal/core"
)

// JobKind is the job manager kind for a port scan.
const JobKind = "portscan"

// KindPort is the result kind every Result is emitted under.
const KindPort = "port"

// Port states. Closed means the host refused the connection, which proves it is
// there; filtered means nothing came back at all.
const (
	StateOpen     = "open"
	StateClosed   = "closed"
	StateFiltered = "filtered"
)

const (
	defaultTimeoutMS = 1000
	minTimeoutMS     = 100
	maxTimeoutMS     = 30000

	defaultWorkers = 64
	maxWorkers     = 512

	maxHostLength = 253

	// resolveTimeout keeps a dead DNS server from stalling the whole scan.
	resolveTimeout = 5 * time.Second
)

// Params is what the UI sends. Every zero value means "use the default", so a
// scan with nothing but a host and a port spec filled in is a good scan.
type Params struct {
	Host string `json:"host"`
	// Ports is the spec as typed: "443", "80,443", "1-1024", "80, 8000-8100".
	Ports       string `json:"ports"`
	TimeoutMS   int    `json:"timeoutMs"`
	Workers     int    `json:"workers"`
	GrabBanners bool   `json:"grabBanners"`
}

// Result is one port, decided. Every port in the spec produces exactly one.
type Result struct {
	Port      int     `json:"port"`
	State     string  `json:"state"`
	Service   string  `json:"service"`
	Banner    string  `json:"banner"`
	LatencyMS float64 `json:"latencyMs"`
}

// settings is the validated form of Params, with the defaults filled in.
type settings struct {
	host    string
	ports   []int
	timeout time.Duration
	workers int
	banners bool
}

// normalize catches everything a user can get wrong, so a bad request rejects
// the StartScan call instead of starting a job that immediately fails.
func (p Params) normalize() (settings, error) {
	host := strings.TrimSpace(p.Host)
	if host == "" || len(host) > maxHostLength {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"Enter a host name or IP address to check, for example 192.168.1.50 or printer-3.")
	}

	ports, err := parsePorts(p.Ports)
	if err != nil {
		return settings{}, err
	}

	if p.TimeoutMS != 0 && (p.TimeoutMS < minTimeoutMS || p.TimeoutMS > maxTimeoutMS) {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"The wait per port must be between 0.1 and 30 seconds. %d ms is outside that.", p.TimeoutMS)
	}
	if p.Workers < 0 || p.Workers > maxWorkers {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"Check between 1 and %d ports at a time. %d is outside that.", maxWorkers, p.Workers)
	}

	timeoutMS := p.TimeoutMS
	if timeoutMS == 0 {
		timeoutMS = defaultTimeoutMS
	}
	workers := p.Workers
	if workers == 0 {
		workers = defaultWorkers
	}
	if workers > len(ports) {
		workers = len(ports)
	}

	return settings{
		host:    host,
		ports:   ports,
		timeout: time.Duration(timeoutMS) * time.Millisecond,
		workers: workers,
		banners: p.GrabBanners,
	}, nil
}

// Service owns the scanning entry points. The App struct forwards its bound
// methods to it.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartScan begins a scan and returns the job id at once. Ports arrive as
// "port" items on job:result, one for every port in the spec.
func (s *Service) StartScan(p Params) (string, error) {
	set, err := p.normalize()
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, len(set.ports), func(jc *core.JobContext) error {
		_, err := scan(jc, set)
		return err
	}), nil
}

// scan probes every port once and records the tally carried by job:done.
func scan(jc *core.JobContext, s settings) (map[string]any, error) {
	ctx := jc.Ctx()
	total := len(s.ports)
	jc.SetTotal(total)
	jc.Progress(0, total, fmt.Sprintf("Checking %d ports on %s", total, s.host))

	ip, err := resolveHost(ctx, s.host)
	if err != nil {
		return nil, err
	}
	message := fmt.Sprintf("Checking %d ports on %s (%s)", total, s.host, ip)
	jc.Progress(0, total, message)

	check := func(c context.Context, port int) (Result, bool) {
		return probe(c, ip, port, s.timeout, s.banners), true
	}

	var open, closed, filtered, done int
	for r := range core.Pool(ctx, s.ports, s.workers, check) {
		switch r.State {
		case StateOpen:
			open++
		case StateClosed:
			closed++
		default:
			filtered++
		}
		done++
		jc.Emit(KindPort, r)
		jc.Progress(done, total, message)
	}
	// The pool closes its channel on cancellation too, so the loop ends either
	// way and the context is what tells the two apart.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	summary := map[string]any{
		"host":     s.host,
		"ip":       ip,
		"total":    total,
		"open":     open,
		"closed":   closed,
		"filtered": filtered,
		"note":     summaryNote(open, closed, filtered),
	}
	jc.SetSummary(summary)
	return summary, nil
}

// resolveHost turns what the tech typed into one address to knock on. IPv4 wins
// because the gear a tech scans is nearly always v4.
func resolveHost(ctx context.Context, host string) (string, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(lookupCtx, "ip", host)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil || len(ips) == 0 {
		return "", core.Errorf(core.CodeNotFound,
			"CHIT could not find %s. Check the spelling, or enter the IP address instead.", host)
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String(), nil
		}
	}
	return ips[0].String(), nil
}

// summaryNote explains a result that a tech would otherwise misread: nothing
// open is a different fault depending on whether the host answered at all.
func summaryNote(open, closed, filtered int) string {
	switch {
	case open == 0 && closed == 0:
		return "Nothing answered on any of those ports. The host may be switched off, or a firewall is dropping the checks without replying."
	case open == 0 && closed > 0:
		return "The host is there and answering, but nothing is listening on the ports you checked. The service is probably stopped, or it is running on a different port."
	case open > 0 && filtered > 0:
		return fmt.Sprintf("%d ports gave no answer at all. That usually means a firewall is dropping the traffic rather than the port being closed.", filtered)
	default:
		return ""
	}
}
