package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// flushInterval caps event traffic at ~10 emits per second per job, so a /24
// sweep does not drown the webview in messages.
const flushInterval = 100 * time.Millisecond

type JobFunc func(jc *JobContext) error

type emitFunc func(ctx context.Context, name string, data ...any)

type JobManager struct {
	mu   sync.Mutex
	ctx  context.Context
	jobs map[string]context.CancelFunc
	emit emitFunc
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs: make(map[string]context.CancelFunc),
		emit: func(ctx context.Context, name string, data ...any) {
			if ctx == nil {
				return
			}
			wruntime.EventsEmit(ctx, name, data...)
		},
	}
}

// SetContext stores the Wails startup context used to emit events.
func (m *JobManager) SetContext(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
}

// Start runs fn in a goroutine and returns the job id immediately.
func (m *JobManager) Start(kind string, total int, fn JobFunc) string {
	id := newJobID()

	m.mu.Lock()
	parent := m.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	m.jobs[id] = cancel
	m.mu.Unlock()

	jc := &JobContext{jobID: id, ctx: ctx, m: m, total: total, summary: map[string]any{}}

	go func() {
		defer cancel()
		started := time.Now()
		stopFlusher := jc.startFlusher()
		err := fn(jc)
		stopFlusher()
		m.forget(id)

		cancelled := ctx.Err() != nil || errors.Is(err, context.Canceled)
		if err != nil && !cancelled {
			m.emitEvent(EventError, JobError{JobID: id, Code: CodeOf(err), Message: MessageOf(err)})
			return
		}
		m.emitEvent(EventDone, JobDone{
			JobID:      id,
			Cancelled:  cancelled,
			DurationMS: time.Since(started).Milliseconds(),
			Summary:    jc.snapshotSummary(),
		})
	}()

	return id
}

// Cancel stops a running job. Its JobFunc is expected to return once its
// context is cancelled, at which point a JobDone with cancelled=true is sent.
func (m *JobManager) Cancel(jobID string) error {
	m.mu.Lock()
	cancel, ok := m.jobs[jobID]
	delete(m.jobs, jobID)
	m.mu.Unlock()

	if !ok {
		return Errorf(CodeNotFound, "That job is not running any more, so there was nothing to stop.")
	}
	cancel()
	return nil
}

// Running reports how many jobs are currently registered.
func (m *JobManager) Running() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.jobs)
}

func (m *JobManager) forget(jobID string) {
	m.mu.Lock()
	delete(m.jobs, jobID)
	m.mu.Unlock()
}

func (m *JobManager) emitEvent(name string, payload any) {
	m.mu.Lock()
	ctx, emit := m.ctx, m.emit
	m.mu.Unlock()
	emit(ctx, name, payload)
}

type JobContext struct {
	jobID string
	ctx   context.Context
	m     *JobManager

	mu       sync.Mutex
	buf      []any
	bufKind  string
	done     int
	total    int
	message  string
	progress bool
	summary  map[string]any
}

// Ctx is cancelled when the user cancels the job or the app shuts down.
// Every worker must honour it.
func (jc *JobContext) Ctx() context.Context { return jc.ctx }

// Emit queues one result item. Items are coalesced into ResultBatch events.
func (jc *JobContext) Emit(kind string, item any) {
	var ready *ResultBatch

	jc.mu.Lock()
	if len(jc.buf) > 0 && kind != jc.bufKind {
		ready = jc.takeBatchLocked()
	}
	jc.bufKind = kind
	jc.buf = append(jc.buf, item)
	jc.mu.Unlock()

	if ready != nil {
		jc.m.emitEvent(EventResult, *ready)
	}
}

// Progress records how far along the job is. The value is sent on the next
// flush, so calling it per item is cheap.
func (jc *JobContext) Progress(done, total int, msg string) {
	jc.mu.Lock()
	jc.done, jc.total, jc.message, jc.progress = done, total, msg, true
	jc.mu.Unlock()
}

// SetTotal updates the denominator once it is known (e.g. after parsing a range).
func (jc *JobContext) SetTotal(total int) {
	jc.mu.Lock()
	jc.total, jc.progress = total, true
	jc.mu.Unlock()
}

// SetSummary sets the key/value pairs reported in the JobDone event.
func (jc *JobContext) SetSummary(summary map[string]any) {
	jc.mu.Lock()
	jc.summary = summary
	jc.mu.Unlock()
}

func (jc *JobContext) snapshotSummary() map[string]any {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	out := make(map[string]any, len(jc.summary))
	for k, v := range jc.summary {
		out[k] = v
	}
	return out
}

func (jc *JobContext) takeBatchLocked() *ResultBatch {
	batch := &ResultBatch{JobID: jc.jobID, Kind: jc.bufKind, Items: jc.buf}
	jc.buf = nil
	return batch
}

func (jc *JobContext) flush() {
	jc.mu.Lock()
	var batch *ResultBatch
	if len(jc.buf) > 0 {
		batch = jc.takeBatchLocked()
	}
	var progress *Progress
	if jc.progress {
		progress = &Progress{JobID: jc.jobID, Done: jc.done, Total: jc.total, Message: jc.message}
		jc.progress = false
	}
	jc.mu.Unlock()

	if batch != nil {
		jc.m.emitEvent(EventResult, *batch)
	}
	if progress != nil {
		jc.m.emitEvent(EventProgress, *progress)
	}
}

// startFlusher ticks until the returned stop func is called, which also
// performs the final flush so nothing queued is lost.
func (jc *JobContext) startFlusher() (stop func()) {
	quit := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		t := time.NewTicker(flushInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				jc.flush()
			case <-quit:
				return
			}
		}
	}()

	return func() {
		close(quit)
		<-stopped
		jc.flush()
	}
}

func newJobID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "job-" + time.Now().Format("150405.000000")
	}
	return "job-" + hex.EncodeToString(b[:])
}
