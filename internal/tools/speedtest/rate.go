package speedtest

import "time"

// mbps converts bytes over a duration into megabits per second, decimal
// megabits, which is how every ISP quotes a line.
func mbps(bytes int64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(bytes) * 8 / 1e6 / d.Seconds()
}

// jitter is the mean absolute difference between consecutive probes.
func jitter(ms []float64) float64 {
	if len(ms) < 2 {
		return 0
	}
	total := 0.0
	for i := 1; i < len(ms); i++ {
		d := ms[i] - ms[i-1]
		if d < 0 {
			d = -d
		}
		total += d
	}
	return total / float64(len(ms)-1)
}

func meanF(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	total := 0.0
	for _, x := range v {
		total += x
	}
	return total / float64(len(v))
}

func minF(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	out := v[0]
	for _, x := range v[1:] {
		if x < out {
			out = x
		}
	}
	return out
}

// ramp remembers where the transfer stopped being TCP slow start, so the
// headline figure is the speed the line settled at rather than an average
// dragged down by the first second.
type ramp struct {
	set     bool
	bytes   int64
	elapsed time.Duration
}

// mark records the first tick at or after the ramp point and ignores the rest.
func (r *ramp) mark(bytes int64, elapsed time.Duration) {
	if !r.set && elapsed >= rampMS*time.Millisecond {
		r.set, r.bytes, r.elapsed = true, bytes, elapsed
	}
}

// headline is the rate measured since the ramp point. A phase that ended before
// the ramp point (a 3 second run that stalled, or a test stopped early) falls
// back to the whole phase, which is better than no number at all.
func (r ramp) headline(bytes int64, elapsed time.Duration) float64 {
	if !r.set || elapsed <= r.elapsed {
		return mbps(bytes, elapsed)
	}
	return mbps(bytes-r.bytes, elapsed-r.elapsed)
}
