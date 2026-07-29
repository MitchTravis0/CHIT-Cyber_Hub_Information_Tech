// Package triage runs the whole "is the internet down" checklist in order and
// reports where it breaks. It is the service layer the Internet Triage page
// talks to.
package triage

import (
	"context"
	"fmt"
	"time"

	"chit/internal/core"
)

// JobKind is the job manager kind for a triage run.
const JobKind = "triage"

// KindRung is the result kind every Rung is emitted under.
const KindRung = "rung"

// Rung statuses. A warn does not stop the ladder; a fail does.
const (
	StatusOK      = "ok"
	StatusWarn    = "warn"
	StatusFail    = "fail"
	StatusSkipped = "skipped"
)

// Rung ids, in ladder order.
const (
	RungAdapter  = "adapter"
	RungGateway  = "gateway"
	RungDNS      = "dns"
	RungInternet = "internet"
	RungHTTPS    = "https"
	RungPortal   = "portal"
)

// Steps is how many rungs the ladder always has. Every run emits all of them,
// so the screen shows the whole chain and where it broke.
const Steps = 6

const (
	// DefaultTimeoutMS is the budget for one rung, not for the whole ladder.
	DefaultTimeoutMS = 4000
	minTimeoutMS     = 500
	maxTimeoutMS     = 20000
)

// The fixed targets. They are named on the page before the button is pressed,
// because this tool reaches the internet the moment it runs and a tech on a
// locked-down network should be able to see what it will touch.
const (
	// DNSTestName is resolved by the DNS rung. example.com is IANA's reserved
	// example domain, which will not move and will not be blocked as an ad.
	DNSTestName = "example.com"
	// FallbackResolver is asked only when the system resolver fails, to tell a
	// broken DNS server apart from a dead internet.
	FallbackResolver = "1.1.1.1"
	// HTTPSTestURL must answer 204 with an empty body.
	HTTPSTestURL = "https://www.google.com/generate_204"
	// PortalTestURL is plain http and must answer 200 with exactly
	// PortalTestBody. A redirect or different text means a login page is in the
	// way.
	PortalTestURL  = "http://detectportal.firefox.com/success.txt"
	PortalTestBody = "success"
)

// InternetTargets are dialled by the internet rung: two operators, so one being
// down does not read as "no internet".
var InternetTargets = []string{"1.1.1.1:443", "8.8.8.8:443"}

// GatewayPorts are tried in order when the gateway ignores ping.
var GatewayPorts = []int{80, 443, 53}

type Params struct {
	// TimeoutMS is the budget for one rung, not for the whole ladder.
	TimeoutMS int `json:"timeoutMs"`
}

// Rung is one step of the ladder.
type Rung struct {
	ID string `json:"id"`
	// Step is 1 to 6, so the UI can number them without knowing the order.
	Step int `json:"step"`
	// Name is the heading a tech reads.
	Name   string `json:"name"`
	Status string `json:"status"`
	// Detail is what was measured.
	Detail string `json:"detail"`
	// Advice says what to do. It is set for every status including ok, where it
	// says what the step proves.
	Advice string `json:"advice"`
	// Target is what was checked, so the page can name it.
	Target string `json:"target"`
	// MS is how long the rung took, 0 for a skipped rung.
	MS float64 `json:"ms"`
}

// Sink is where a run reports to. The job wires it to jc.Emit and jc.Progress;
// a test wires it to a slice, which is the only way to read what a run emitted
// without a Wails runtime.
type Sink struct {
	Emit     func(Rung)
	Progress func(done int, message string)
}

// Service owns the triage entry point. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

func (p Params) normalize() (time.Duration, error) {
	ms := p.TimeoutMS
	if ms == 0 {
		ms = DefaultTimeoutMS
	}
	if ms < minTimeoutMS || ms > maxTimeoutMS {
		return 0, core.Errorf(core.CodeInvalidInput,
			"The wait per step must be between 0.5 and 20 seconds. %d ms is outside that.", ms)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// StartTriage begins a run and returns the job id at once. Rungs arrive as
// "rung" items on job:result, always six of them, in ladder order.
func (s *Service) StartTriage(p Params) (string, error) {
	timeout, err := p.normalize()
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, Steps, func(jc *core.JobContext) error {
		return runTriage(jc, timeout)
	}), nil
}

// runTriage is the body of the job, named so it is not stranded at 0% coverage
// inside an anonymous closure.
func runTriage(jc *core.JobContext, timeout time.Duration) error {
	summary, err := Run(jc.Ctx(), timeout, DefaultProbes(), Sink{
		Emit:     func(r Rung) { jc.Emit(KindRung, r) },
		Progress: func(done int, message string) { jc.Progress(done, Steps, message) },
	})
	if err != nil {
		return err
	}
	jc.SetSummary(summary)
	return nil
}

// step is one rung's definition: its id, its heading, and the check it runs.
type step struct {
	id    string
	name  string
	check func(context.Context, Probes, time.Duration) Rung
}

func ladder() []step {
	return []step{
		{RungAdapter, "Network adapter", adapterRung},
		{RungGateway, "Gateway", gatewayRung},
		{RungDNS, "DNS", dnsRung},
		{RungInternet, "The internet", internetRung},
		{RungHTTPS, "HTTPS", httpsRung},
		{RungPortal, "Captive portal", portalRung},
	}
}

// Run walks the ladder in order, stopping at the first failure and marking
// everything after it as not checked. It returns the job:done summary.
//
// A cancelled context comes back as ctx.Err() so the job ends as job:done with
// cancelled=true rather than looking like a failure the user did not cause.
func Run(ctx context.Context, timeout time.Duration, probes Probes, out Sink) (map[string]any, error) {
	steps := ladder()
	tally := map[string]int{StatusOK: 0, StatusWarn: 0, StatusFail: 0, StatusSkipped: 0}
	firstFailure, headline := "", ""
	stopped := false

	for i, s := range steps {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		out.Progress(i, "Checking "+lowerFirst(s.name))

		var rung Rung
		if stopped {
			rung = Rung{
				ID:     s.id,
				Name:   s.name,
				Status: StatusSkipped,
				Detail: "not checked",
				Advice: "This was not checked because an earlier step failed. Fix that one first.",
			}
		} else {
			started := time.Now()
			rung = s.check(ctx, probes, timeout)
			rung.ID, rung.Name = s.id, s.name
			rung.MS = milliseconds(time.Since(started))
		}
		rung.Step = i + 1

		if rung.Status == StatusFail && !stopped {
			stopped = true
			firstFailure = s.id
			headline = s.name + ": " + firstSentence(rung.Advice)
		}
		tally[rung.Status]++
		out.Emit(rung)
		out.Progress(i+1, "Checking "+lowerFirst(s.name))
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if headline == "" {
		headline = "Everything passed. This computer can reach the internet."
	}
	return map[string]any{
		"ok":           tally[StatusOK],
		"warn":         tally[StatusWarn],
		"failed":       tally[StatusFail],
		"skipped":      tally[StatusSkipped],
		"firstFailure": firstFailure,
		"headline":     headline,
	}, nil
}

// firstSentence keeps the headline to one line: the advice is often two or
// three sentences and only the first belongs at the top of the page.
func firstSentence(advice string) string {
	for i := 0; i < len(advice)-1; i++ {
		if advice[i] == '.' && advice[i+1] == ' ' {
			return advice[:i+1]
		}
	}
	return advice
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'A' && s[0] <= 'Z' && !allCaps(s) {
		return string(s[0]+('a'-'A')) + s[1:]
	}
	return s
}

// allCaps keeps "DNS" and "HTTPS" as they are: "checking dNS" reads as a typo.
func allCaps(s string) bool {
	for _, c := range s {
		if c >= 'a' && c <= 'z' {
			return false
		}
	}
	return true
}

func milliseconds(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func fmtMS(ms float64) string { return fmt.Sprintf("%.0f ms", ms) }
