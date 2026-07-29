// Package ipscan drives a netscan sweep as a cancellable job and names the
// vendors behind the MAC addresses it finds. It is the service layer the IP
// Range Scanner page talks to.
package ipscan

import (
	"time"

	"chit/internal/core"
	"chit/internal/netinfo"
	"chit/internal/netscan"
	"chit/internal/ouidb"
)

// JobKind is the job manager kind for a range scan.
const JobKind = "ipscan"

const (
	maxTimeoutMS = 30000
	maxWorkers   = 1024
)

// ScanParams is what the UI sends. Every zero value means "use the default",
// so a scan with nothing but a range filled in is a good scan.
type ScanParams struct {
	Range     string `json:"range"`
	TimeoutMS int    `json:"timeoutMs"`
	Workers   int    `json:"workers"`
	Ports     []int  `json:"ports"`
	// SkipTCP turns off the TCP fallback used on devices that ignore ping.
	SkipTCP bool `json:"skipTcp"`
	// SkipHostnames turns off reverse DNS, which is the slow part on a network
	// with no local DNS server.
	SkipHostnames bool `json:"skipHostnames"`
}

// options validates the parameters and turns them into a netscan sweep.
// Everything a user can get wrong is caught here rather than inside the job,
// so a bad range rejects the StartScan call instead of starting a scan that
// immediately fails.
func (p ScanParams) options() (netscan.Options, error) {
	r, err := netscan.ParseRange(p.Range)
	if err != nil {
		return netscan.Options{}, err
	}
	if p.TimeoutMS < 0 || p.TimeoutMS > maxTimeoutMS {
		return netscan.Options{}, core.Errorf(core.CodeInvalidInput,
			"The wait per device must be between 0 and %d seconds. %d ms is outside that.",
			maxTimeoutMS/1000, p.TimeoutMS)
	}
	if p.Workers < 0 || p.Workers > maxWorkers {
		return netscan.Options{}, core.Errorf(core.CodeInvalidInput,
			"Check between 1 and %d addresses at a time. %d is outside that.", maxWorkers, p.Workers)
	}
	for _, port := range p.Ports {
		if port < 1 || port > 65535 {
			return netscan.Options{}, core.Errorf(core.CodeInvalidInput,
				"%d is not a port number. Ports run from 1 to 65535.", port)
		}
	}

	return netscan.Options{
		Range:         r,
		Workers:       p.Workers,
		Timeout:       time.Duration(p.TimeoutMS) * time.Millisecond,
		Ports:         p.Ports,
		SkipTCP:       p.SkipTCP,
		SkipHostnames: p.SkipHostnames,
		VendorLookup:  vendorName,
	}, nil
}

// Service owns the scanning entry points. The App struct embeds one and
// forwards its bound methods to it.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartScan begins a sweep and returns the job id at once. Hosts arrive as
// "host" items on job:result, one for every address in the range, so the UI can
// paint the whole block free or occupied.
func (s *Service) StartScan(p ScanParams) (string, error) {
	opts, err := p.options()
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, opts.Range.Count, func(jc *core.JobContext) error {
		_, err := sweep(jc, opts)
		return err
	}), nil
}

// sweep runs the discovery and records the tally carried by job:done.
func sweep(jc *core.JobContext, opts netscan.Options) (netscan.Summary, error) {
	sum, err := netscan.Sweep(jc, opts)
	if err != nil {
		return sum, err
	}
	m := sum.Map()
	m["range"] = opts.Range.Text
	jc.SetSummary(m)
	return sum, nil
}

// vendorName is handed to netscan so MAC addresses read out of the ARP cache
// reach the UI already named. An unregistered prefix (a phone using a
// randomized address, say) simply has no vendor.
func vendorName(mac string) string {
	vendor, _ := ouidb.Lookup(mac)
	return vendor
}

// Defaults is the range the UI offers before the user types anything.
type Defaults struct {
	Range   string `json:"range"`
	Adapter string `json:"adapter"`
	IP      string `json:"ip"`
	Gateway string `json:"gateway"`
}

// SuggestedRange describes the network this machine is on, which is the range a
// tech wants to scan nearly every time.
func SuggestedRange() (Defaults, error) {
	s, err := netinfo.PrimarySubnet()
	if err != nil {
		return Defaults{}, err
	}
	return Defaults{Range: s.CIDR, Adapter: s.Adapter, IP: s.IP, Gateway: s.Gateway}, nil
}
