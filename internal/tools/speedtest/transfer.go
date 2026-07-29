package speedtest

import (
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"chit/internal/core"
)

// countingReader adds every byte that passes through it to a shared total, so
// the sampler can read how far a phase has got without touching the workers.
type countingReader struct {
	r io.Reader
	n *int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		atomic.AddInt64(c.n, int64(n))
	}
	return n, err
}

// zeroSource is an endless stream of zero bytes, so an upload never needs a
// 25 MiB buffer in memory.
type zeroSource struct{}

func (zeroSource) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// transferOpts is one phase: which way the bytes go, where to, and for how long.
type transferOpts struct {
	phase    string
	url      string
	message  string
	client   *http.Client
	duration time.Duration
	streams  int
	// doneSec is how many seconds of the progress bar earlier phases used up.
	doneSec  int
	totalSec int
	upload   bool
}

// phaseResult is what a phase measured. The phase returns it as well as
// emitting samples, which is what makes it testable without reading events.
type phaseResult struct {
	Bytes    int64
	Mbps     float64
	Samples  int
	Failures int
}

// runTransfer moves as much as it can for the length of the phase and reports
// the settled rate.
func runTransfer(jc *core.JobContext, o transferOpts) (phaseResult, error) {
	phaseCtx, cancel := context.WithTimeout(jc.Ctx(), o.duration)
	defer cancel()

	var moved, failures int64

	// core.Pool maps a fixed list of items; these workers are long-lived
	// streams bounded by the phase clock instead, so a WaitGroup is the fit.
	var wg sync.WaitGroup
	for range o.streams {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for phaseCtx.Err() == nil {
				if err := o.once(phaseCtx, &moved); err != nil && phaseCtx.Err() == nil {
					atomic.AddInt64(&failures, 1)
					select {
					case <-time.After(retryMS * time.Millisecond):
					case <-phaseCtx.Done():
					}
				}
			}
		}()
	}

	started := time.Now()
	ticker := time.NewTicker(sampleMS * time.Millisecond)
	defer ticker.Stop()

	var r ramp
	var prevBytes int64
	var prevElapsed time.Duration
	samples := 0

sampling:
	for {
		select {
		case <-phaseCtx.Done():
			break sampling
		case <-ticker.C:
			elapsed := time.Since(started)
			bytes := atomic.LoadInt64(&moved)
			r.mark(bytes, elapsed)
			samples++
			jc.Emit(KindSample, Sample{
				Phase:     o.phase,
				ElapsedMS: elapsed.Milliseconds(),
				Bytes:     bytes,
				Mbps:      mbps(bytes-prevBytes, elapsed-prevElapsed),
				AvgMbps:   r.headline(bytes, elapsed),
			})
			prevBytes, prevElapsed = bytes, elapsed
			jc.Progress(o.doneSec+int(elapsed.Seconds()), o.totalSec, o.message)
		}
	}

	wg.Wait()
	elapsed := time.Since(started)
	bytes := atomic.LoadInt64(&moved)
	headline := r.headline(bytes, elapsed)
	samples++
	jc.Emit(KindSample, Sample{
		Phase:     o.phase,
		ElapsedMS: elapsed.Milliseconds(),
		Bytes:     bytes,
		Mbps:      headline,
		AvgMbps:   headline,
		Final:     true,
	})

	out := phaseResult{Bytes: bytes, Mbps: headline, Samples: samples, Failures: int(failures)}
	if err := jc.Ctx().Err(); err != nil {
		return out, err
	}
	if bytes == 0 && failures > 0 {
		return out, core.Errorf(core.CodeNetwork,
			"The connection dropped during the test, so there is no result to show. Try again.")
	}
	return out, nil
}

// once runs a single request. What an upload counts is what was handed to the
// socket, so a connection that stalls at the far end can read high for the last
// fraction of a second, which is part of why the result is approximate.
func (o transferOpts) once(ctx context.Context, moved *int64) error {
	method := http.MethodGet
	var body io.Reader
	if o.upload {
		method = http.MethodPost
		body = &countingReader{r: io.LimitReader(zeroSource{}, chunkBytes), n: moved}
	}

	req, err := newRequest(ctx, method, o.url, body)
	if err != nil {
		return err
	}
	if o.upload {
		req.ContentLength = chunkBytes
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if o.upload {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	_, err = io.Copy(io.Discard, &countingReader{r: resp.Body, n: moved})
	return err
}
