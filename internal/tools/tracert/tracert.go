// Package tracert follows the IPv4 path to a host by driving the traceroute
// command that ships with the operating system, streaming one result per hop.
// It is the service layer the Traceroute page talks to.
package tracert

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"runtime"
	"strings"

	"chit/internal/core"
)

// JobKind is the job manager kind for a trace.
const JobKind = "traceroute"

// KindHop is the result kind every Hop is emitted under. A job emits this kind
// and no other: useJob ignores the batch kind, so a second kind would be mixed
// into the same results array.
const KindHop = "hop"

const (
	defaultMaxHops   = 30
	minMaxHops       = 1
	maxMaxHops       = 64
	defaultQueries   = 3
	minQueries       = 1
	maxQueries       = 5
	defaultTimeoutMS = 2000
	minTimeoutMS     = 200
	maxTimeoutMS     = 10000
	maxHostLength    = 253
)

// Params is what the UI sends. Every zero value means "use the default".
type Params struct {
	Host    string `json:"host"`
	MaxHops int    `json:"maxHops"`
	// Queries is probes per hop. Ignored on Windows, where tracert always
	// sends three.
	Queries   int  `json:"queries"`
	TimeoutMS int  `json:"timeoutMs"`
	NoNames   bool `json:"noNames"`
}

// Hop is one router on the path, or one hop that nothing answered on.
type Hop struct {
	Number   int    `json:"number"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	// TimesMS holds one entry per probe that came back. Never nil: an empty
	// slice marshals to [] so the frontend never sees null.
	TimesMS []float64 `json:"timesMs"`
	Lost    int       `json:"lost"`
	BestMS  float64   `json:"bestMs"`
	AvgMS   float64   `json:"avgMs"`
	WorstMS float64   `json:"worstMs"`
	// AlsoSeen holds extra addresses when the probes for one hop came back from
	// different routers, which happens on load-balanced paths. Never nil.
	AlsoSeen []string `json:"alsoSeen"`
	// Note is the plain-English form of an unreachable annotation such as !H,
	// or "" when the router said nothing unusual.
	Note string `json:"note"`
	// Final is true for the hop that is the destination.
	Final bool `json:"final"`
}

// settings is Params after validation, with every default filled in.
type settings struct {
	Host      string
	MaxHops   int
	Queries   int
	TimeoutMS int
	NoNames   bool
}

// normalize catches everything a user can get wrong, so a bad request rejects
// the StartTrace call instead of starting a job that immediately fails.
func (p Params) normalize() (settings, error) {
	s := settings{
		Host:      strings.TrimSpace(p.Host),
		MaxHops:   p.MaxHops,
		Queries:   p.Queries,
		TimeoutMS: p.TimeoutMS,
		NoNames:   p.NoNames,
	}
	if s.Host == "" {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"Enter a host name or IP address to trace, for example 8.8.8.8 or google.com.")
	}
	if !validHost(s.Host) {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"%q is not a host name or an IP address. Try 8.8.8.8 or google.com.", s.Host)
	}
	if s.MaxHops == 0 {
		s.MaxHops = defaultMaxHops
	} else if s.MaxHops < minMaxHops || s.MaxHops > maxMaxHops {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"Follow between %d and %d hops. %d is outside that.", minMaxHops, maxMaxHops, s.MaxHops)
	}
	if s.Queries == 0 {
		s.Queries = defaultQueries
	} else if s.Queries < minQueries || s.Queries > maxQueries {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"Send between %d and %d probes per hop. %d is outside that.", minQueries, maxQueries, s.Queries)
	}
	if s.TimeoutMS == 0 {
		s.TimeoutMS = defaultTimeoutMS
	} else if s.TimeoutMS < minTimeoutMS || s.TimeoutMS > maxTimeoutMS {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"The wait per probe must be between %.1f and %d seconds. %d ms is outside that.",
			float64(minTimeoutMS)/1000, maxTimeoutMS/1000, s.TimeoutMS)
	}
	return s, nil
}

// validHost accepts an address literal or a host name. It never resolves: a
// name that does not exist is a lookup failure inside the job, not a typo the
// form can catch.
func validHost(host string) bool {
	if len(host) > maxHostLength || strings.ContainsAny(host, " \t") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
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

// Service owns the trace entry points. App forwards its bound methods here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartTrace begins a trace and returns the job id at once. Hops arrive as
// "hop" items on job:result, one for every line the system tool prints.
func (s *Service) StartTrace(p Params) (string, error) {
	set, err := p.normalize()
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, set.MaxHops, func(jc *core.JobContext) error {
		return trace(jc, set)
	}), nil
}

func trace(jc *core.JobContext, s settings) error {
	ip, err := resolveIPv4(jc.Ctx(), s.Host)
	if err != nil {
		if jc.Ctx().Err() != nil {
			return jc.Ctx().Err()
		}
		return err
	}

	tool, err := lookupTool(runtime.GOOS)
	if err != nil {
		return err
	}

	parse := parseUnixHop
	if runtime.GOOS == "windows" {
		parse = parseTracertHop
	}

	message := fmt.Sprintf("Following the path to %s (%s)", s.Host, ip)
	hops, answered := 0, 0
	reached := false
	onHop := func(hop Hop) {
		hop.Final = finalHop(hop, ip)
		if hop.Final {
			reached = true
		}
		hops++
		if len(hop.TimesMS) > 0 {
			answered++
		}
		jc.Emit(KindHop, hop)
		jc.Progress(hop.Number, s.MaxHops, message)
	}

	if err := runTool(jc, tool, buildArgs(runtime.GOOS, ip, s), parse, onHop); err != nil {
		return err
	}

	jc.SetSummary(map[string]any{
		"host":    s.Host,
		"ip":      ip,
		"hops":    hops,
		"reached": reached,
		"tool":    filepath.Base(tool),
		"note":    summaryNote(s.Host, hops, answered, reached),
	})
	return nil
}

// finalHop reports whether this hop is the destination itself, which is what
// tells a tech the path completed.
func finalHop(hop Hop, destination string) bool {
	return hop.IP != "" && hop.IP == destination
}

// summaryNote explains a path that stopped short or answered nothing at all,
// because both look like a broken tool to somebody who has not seen them
// before.
func summaryNote(host string, hops, answered int, reached bool) string {
	if hops == 0 || answered == 0 {
		return "No router on the path answered. Some networks block traceroute completely, so this does not always mean the connection is broken."
	}
	if !reached {
		return fmt.Sprintf("The path stopped after %d hops without reaching %s. The last router that answered is the place to start looking.", hops, host)
	}
	return ""
}

// resolveIPv4 turns whatever the user typed into one IPv4 literal. The system
// tool is handed that literal and never the text the user typed, so no user
// input ever reaches a command line.
func resolveIPv4(ctx context.Context, host string) (string, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Is4() {
			return addr.String(), nil
		}
		return "", core.Errorf(core.CodeInvalidInput,
			"%s only has an IPv6 address. CHIT follows IPv4 paths only.", host)
	}

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	if ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host); err == nil && len(ips) > 0 {
		return ips[0].String(), nil
	}
	if ips, err := net.DefaultResolver.LookupIP(ctx, "ip6", host); err == nil && len(ips) > 0 {
		return "", core.Errorf(core.CodeInvalidInput,
			"%s only has an IPv6 address. CHIT follows IPv4 paths only.", host)
	}
	return "", core.Errorf(core.CodeNotFound,
		"CHIT could not find %s. Check the spelling, or enter the IP address instead.", host)
}
