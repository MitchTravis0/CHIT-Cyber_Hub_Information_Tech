// Package speedtest measures this machine's path to Cloudflare: round trip
// time, then download, then upload. It is the service layer the Speed Test page
// talks to, and every number it produces is approximate on purpose.
package speedtest

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"chit/internal/core"
)

// JobKind is the job manager kind for a speed test.
const JobKind = "speedtest"

// KindSample is the result kind used for every Sample emitted by a test.
const KindSample = "sample"

const (
	baseURL       = "https://speed.cloudflare.com"
	chunkBytes    = 26214400 // 25 MiB per request, looped until the phase ends
	latencyProbes = 10
	rampMS        = 1000 // the first second of a transfer is ramp-up, not speed
	sampleMS      = 200
	userAgent     = "CHIT"
)

const (
	minDurationSec     = 3
	maxDurationSec     = 30
	defaultDurationSec = 8
	maxStreams         = 8
	defaultStreams     = 4
	// latencySec is the fixed share of the progress bar the latency phase gets.
	latencySec = 3
	// retryMS is how long a worker waits before trying again after a request
	// failed while the phase was still live.
	retryMS = 200
	metaSec = 5
)

const (
	phaseLatency  = "latency"
	phaseDownload = "download"
	phaseUpload   = "upload"
)

// approximateNote is shown by the UI at the end of every test. It is the
// sentence that stops a tech quoting these numbers to an ISP as gospel.
const approximateNote = "These numbers are approximate. They measure this computer's path to Cloudflare right now, over whatever it is connected by, including Wi-Fi, a VPN and anything else the machine is downloading."

// Params is what the UI sends. Every zero value means "use the default", so an
// empty Params is a good test.
type Params struct {
	// DurationSec is how long each transfer phase runs. 0 means 8.
	DurationSec int `json:"durationSec"`
	// Streams is how many parallel connections each phase opens. 0 means 4.
	Streams int `json:"streams"`
	// SkipUpload runs download only, for a quick check on a metered link.
	SkipUpload bool `json:"skipUpload"`
}

// Sample is one 200 ms slice of a phase. Phase is "latency", "download" or
// "upload". Final marks the last sample of a phase, whose Mbps is the headline
// figure for that phase.
type Sample struct {
	Phase     string  `json:"phase"`
	ElapsedMS int64   `json:"elapsedMs"`
	Bytes     int64   `json:"bytes"`
	Mbps      float64 `json:"mbps"`
	AvgMbps   float64 `json:"avgMbps"`
	// LatencyMS is set on latency samples only, and is 0 elsewhere.
	LatencyMS float64 `json:"latencyMs"`
	Final     bool    `json:"final"`
}

// validate catches everything a user can get wrong, so a bad request rejects
// the StartTest call instead of starting a job that immediately fails.
func (p Params) validate() (Params, error) {
	if p.DurationSec < 0 || p.DurationSec > maxDurationSec ||
		(p.DurationSec > 0 && p.DurationSec < minDurationSec) {
		return p, core.Errorf(core.CodeInvalidInput,
			"Each part of the test can run for %d to %d seconds. %d is outside that.",
			minDurationSec, maxDurationSec, p.DurationSec)
	}
	if p.Streams < 0 || p.Streams > maxStreams {
		return p, core.Errorf(core.CodeInvalidInput,
			"Use between 1 and %d connections at a time. %d is outside that.", maxStreams, p.Streams)
	}

	if p.DurationSec == 0 {
		p.DurationSec = defaultDurationSec
	}
	if p.Streams == 0 {
		p.Streams = defaultStreams
	}
	return p, nil
}

// Service owns the speed test entry points. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
	// base is a field rather than a constant so the tests can point the whole
	// service at an httptest server instead of the internet.
	base   string
	client *http.Client
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs, base: baseURL, client: newClient()}
}

// newClient keeps its own transport so a speed test cannot be throttled by
// idle connection limits meant for small requests. There is no client timeout:
// the job context governs how long everything runs.
func newClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			ForceAttemptHTTP2:   true,
			MaxIdleConnsPerHost: 16,
		},
	}
}

// StartTest begins a test and returns the job id at once. Samples arrive as
// "sample" items on job:result, about five a second per phase.
func (s *Service) StartTest(p Params) (string, error) {
	p, err := p.validate()
	if err != nil {
		return "", err
	}
	total := latencySec + p.DurationSec
	if !p.SkipUpload {
		total += p.DurationSec
	}
	return s.jobs.Start(JobKind, total, func(jc *core.JobContext) error {
		return s.run(jc, p, total)
	}), nil
}

// measured is everything the test learned, gathered as it goes so a cancelled
// run still reports the phases that finished.
type measured struct {
	streams     int
	durationSec int
	server      string
	latency     latencyResult
	download    phaseResult
	upload      phaseResult
}

func (m measured) summary() map[string]any {
	return map[string]any{
		"downloadMbps":  m.download.Mbps,
		"uploadMbps":    m.upload.Mbps,
		"latencyMs":     m.latency.Best,
		"avgLatencyMs":  m.latency.Avg,
		"jitterMs":      m.latency.Jitter,
		"downloadBytes": m.download.Bytes,
		"uploadBytes":   m.upload.Bytes,
		"streams":       m.streams,
		"durationSec":   m.durationSec,
		"server":        m.server,
		"approximate":   true,
		"note":          approximateNote,
	}
}

func (s *Service) run(jc *core.JobContext, p Params, totalSec int) error {
	m := measured{streams: p.Streams, durationSec: p.DurationSec}
	// The summary is written even when the test is cancelled or a phase fails,
	// so whatever was measured stays on screen.
	defer func() { jc.SetSummary(m.summary()) }()

	m.server = serverName(jc.Ctx(), s.client, s.base)

	lat, err := latencyPhase(jc, s.client, s.base)
	m.latency = lat
	if err != nil {
		return err
	}
	jc.Progress(latencySec, totalSec, "Measured latency")

	duration := time.Duration(p.DurationSec) * time.Second
	m.download, err = runTransfer(jc, transferOpts{
		phase:    phaseDownload,
		url:      s.base + "/__down?bytes=" + strconv.Itoa(chunkBytes),
		message:  "Measuring download speed",
		client:   s.client,
		duration: duration,
		streams:  p.Streams,
		doneSec:  latencySec,
		totalSec: totalSec,
	})
	if err != nil {
		return err
	}

	if p.SkipUpload {
		return nil
	}
	m.upload, err = runTransfer(jc, transferOpts{
		phase:    phaseUpload,
		url:      s.base + "/__up",
		message:  "Measuring upload speed",
		client:   s.client,
		duration: duration,
		streams:  p.Streams,
		doneSec:  latencySec + p.DurationSec,
		totalSec: totalSec,
		upload:   true,
	})
	return err
}

// latencyResult carries the three numbers a tech reads off the latency phase.
type latencyResult struct {
	Best   float64
	Avg    float64
	Jitter float64
}

// latencyPhase times a handful of empty downloads. Throughput is not worth
// measuring against a host that will not answer at all, so a total failure here
// stops the test.
func latencyPhase(jc *core.JobContext, client *http.Client, base string) (latencyResult, error) {
	url := base + "/__down?bytes=0"
	started := time.Now()
	kept := make([]float64, 0, latencyProbes)

	for i := range latencyProbes {
		if jc.Ctx().Err() != nil {
			return latencyResult{}, jc.Ctx().Err()
		}
		ms, err := probe(jc.Ctx(), client, url)
		if err != nil {
			continue
		}
		// The first probe pays for the TCP and TLS handshake, so it is timed
		// but never counted.
		if i == 0 {
			continue
		}
		kept = append(kept, ms)
		jc.Emit(KindSample, Sample{
			Phase:     phaseLatency,
			ElapsedMS: time.Since(started).Milliseconds(),
			LatencyMS: ms,
			AvgMbps:   meanF(kept),
		})
	}

	if len(kept) == 0 {
		return latencyResult{}, core.Errorf(core.CodeNetwork,
			"Could not reach the speed test service. Check that this machine has internet access, and that a proxy or firewall is not blocking it.")
	}

	out := latencyResult{Best: minF(kept), Avg: meanF(kept), Jitter: jitter(kept)}
	jc.Emit(KindSample, Sample{
		Phase:     phaseLatency,
		ElapsedMS: time.Since(started).Milliseconds(),
		LatencyMS: out.Best,
		AvgMbps:   out.Avg,
		Final:     true,
	})
	return out, nil
}

// probe times one empty download, body included, because the time a tech cares
// about is the whole round trip and not just the headers.
func probe(ctx context.Context, client *http.Client, url string) (float64, error) {
	req, err := newRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, err = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if err != nil {
		return 0, err
	}
	return float64(time.Since(started).Microseconds()) / 1000, nil
}

func newRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Cache-Control", "no-cache")
	return req, nil
}

// serverName reads which Cloudflare data centre answered, so the result can say
// where it was measured to. Best effort: a failure just leaves it blank.
func serverName(ctx context.Context, client *http.Client, base string) string {
	ctx, cancel := context.WithTimeout(ctx, metaSec*time.Second)
	defer cancel()

	req, err := newRequest(ctx, http.MethodGet, base+"/meta", nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return ""
	}
	return parseMeta(body)
}

// parseMeta pulls the data centre code out of /meta. The client IP is in there
// too and is deliberately ignored: the Public IP tool owns that.
func parseMeta(body []byte) string {
	var meta struct {
		Colo string `json:"colo"`
	}
	if err := json.Unmarshal(body, &meta); err != nil || meta.Colo == "" {
		return ""
	}
	return "Cloudflare " + meta.Colo
}
