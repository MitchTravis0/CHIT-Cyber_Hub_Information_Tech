package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu     sync.Mutex
	events []recorded
}

type recorded struct {
	name    string
	payload any
}

func newTestManager() (*JobManager, *recorder) {
	m := NewJobManager()
	r := &recorder{}
	m.emit = func(_ context.Context, name string, data ...any) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.events = append(r.events, recorded{name: name, payload: data[0]})
	}
	return m, r
}

func (r *recorder) of(name string) []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []any
	for _, e := range r.events {
		if e.name == name {
			out = append(out, e.payload)
		}
	}
	return out
}

func (r *recorder) waitFor(t *testing.T, name string) any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := r.of(name); len(got) > 0 {
			return got[len(got)-1]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s event", name)
	return nil
}

func TestJobLifecycleEmitsResultsThenDone(t *testing.T) {
	m, r := newTestManager()

	id := m.Start("test", 3, func(jc *JobContext) error {
		for i := 0; i < 3; i++ {
			jc.Emit("host", i)
			jc.Progress(i+1, 3, "scanning")
		}
		jc.SetSummary(map[string]any{"found": 3})
		return nil
	})

	done := r.waitFor(t, EventDone).(JobDone)
	if done.JobID != id {
		t.Fatalf("done jobId = %q, want %q", done.JobID, id)
	}
	if done.Cancelled {
		t.Fatal("job reported as cancelled")
	}
	if done.Summary["found"] != 3 {
		t.Fatalf("summary = %v, want found=3", done.Summary)
	}

	var items []any
	for _, p := range r.of(EventResult) {
		batch := p.(ResultBatch)
		if batch.JobID != id || batch.Kind != "host" {
			t.Fatalf("unexpected batch %+v", batch)
		}
		items = append(items, batch.Items...)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}

	progress := r.of(EventProgress)
	if len(progress) == 0 {
		t.Fatal("no progress event emitted")
	}
	last := progress[len(progress)-1].(Progress)
	if last.Done != 3 || last.Total != 3 || last.Message != "scanning" {
		t.Fatalf("last progress = %+v", last)
	}
	if m.Running() != 0 {
		t.Fatalf("job still registered after completion")
	}
}

func TestEmitsAreBatched(t *testing.T) {
	m, r := newTestManager()

	m.Start("test", 500, func(jc *JobContext) error {
		for i := 0; i < 500; i++ {
			jc.Emit("host", i)
		}
		return nil
	})
	r.waitFor(t, EventDone)

	batches := r.of(EventResult)
	if len(batches) > 5 {
		t.Fatalf("500 fast emits produced %d batches, want them coalesced", len(batches))
	}
	items := 0
	for _, b := range batches {
		items += len(b.(ResultBatch).Items)
	}
	if items != 500 {
		t.Fatalf("batches carried %d items, want 500", items)
	}
}

func TestEmitFlushesWhenKindChanges(t *testing.T) {
	m, r := newTestManager()

	m.Start("test", 2, func(jc *JobContext) error {
		jc.Emit("host", "a")
		jc.Emit("note", "b")
		return nil
	})
	r.waitFor(t, EventDone)

	batches := r.of(EventResult)
	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2 (one per kind)", len(batches))
	}
	if k := batches[0].(ResultBatch).Kind; k != "host" {
		t.Fatalf("first batch kind = %q, want host", k)
	}
	if k := batches[1].(ResultBatch).Kind; k != "note" {
		t.Fatalf("second batch kind = %q, want note", k)
	}
}

func TestCancelStopsJobAndReportsCancelled(t *testing.T) {
	m, r := newTestManager()
	started := make(chan struct{})

	id := m.Start("test", 0, func(jc *JobContext) error {
		close(started)
		<-jc.Ctx().Done()
		return jc.Ctx().Err()
	})
	<-started

	if err := m.Cancel(id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	done := r.waitFor(t, EventDone).(JobDone)
	if !done.Cancelled {
		t.Fatal("done.Cancelled = false, want true")
	}
	if len(r.of(EventError)) != 0 {
		t.Fatal("cancellation must not emit a job:error event")
	}
}

func TestCancelUnknownJob(t *testing.T) {
	m, _ := newTestManager()
	err := m.Cancel("job-nope")
	if err == nil {
		t.Fatal("expected an error for an unknown job id")
	}
	if CodeOf(err) != CodeNotFound {
		t.Fatalf("code = %q, want %q", CodeOf(err), CodeNotFound)
	}
}

func TestJobErrorCarriesFriendlyMessage(t *testing.T) {
	m, r := newTestManager()

	id := m.Start("test", 0, func(jc *JobContext) error {
		return Errorf(CodeNetwork, "No response from 192.168.1.50. The host may be off or blocking ping.")
	})

	jobErr := r.waitFor(t, EventError).(JobError)
	if jobErr.JobID != id || jobErr.Code != CodeNetwork {
		t.Fatalf("unexpected error event %+v", jobErr)
	}
	if jobErr.Message != "No response from 192.168.1.50. The host may be off or blocking ping." {
		t.Fatalf("message = %q", jobErr.Message)
	}
	if len(r.of(EventDone)) != 0 {
		t.Fatal("a failed job must not also emit job:done")
	}
}

func TestPlainErrorIsNotLeakedToTheUser(t *testing.T) {
	m, r := newTestManager()

	m.Start("test", 0, func(jc *JobContext) error {
		return errors.New("dial udp 10.0.0.1:0: i/o timeout")
	})

	jobErr := r.waitFor(t, EventError).(JobError)
	if jobErr.Code != CodeInternal {
		t.Fatalf("code = %q, want %q", jobErr.Code, CodeInternal)
	}
	if jobErr.Message == "dial udp 10.0.0.1:0: i/o timeout" {
		t.Fatal("raw error text was shown to the user")
	}
}

func TestSetTotalIsReported(t *testing.T) {
	m, r := newTestManager()

	m.Start("test", 0, func(jc *JobContext) error {
		jc.SetTotal(254)
		jc.Progress(1, 254, "probing")
		return nil
	})
	r.waitFor(t, EventDone)

	progress := r.of(EventProgress)
	if len(progress) == 0 {
		t.Fatal("no progress emitted")
	}
	if total := progress[len(progress)-1].(Progress).Total; total != 254 {
		t.Fatalf("total = %d, want 254", total)
	}
}
