package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolProcessesEveryItem(t *testing.T) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}

	sum := 0
	for r := range Pool(context.Background(), items, 8, func(_ context.Context, n int) (int, bool) {
		return n, true
	}) {
		sum += r
	}
	if sum != 4950 {
		t.Fatalf("sum = %d, want 4950", sum)
	}
}

func TestPoolDropsUnwantedResults(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6}

	count := 0
	for range Pool(context.Background(), items, 3, func(_ context.Context, n int) (int, bool) {
		return n, n%2 == 0
	}) {
		count++
	}
	if count != 3 {
		t.Fatalf("kept %d results, want 3", count)
	}
}

func TestPoolRespectsWorkerLimit(t *testing.T) {
	items := make([]int, 50)
	var inFlight, peak int64
	var mu sync.Mutex

	for range Pool(context.Background(), items, 4, func(_ context.Context, _ int) (int, bool) {
		n := atomic.AddInt64(&inFlight, 1)
		mu.Lock()
		if n > peak {
			peak = n
		}
		mu.Unlock()
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		return 0, true
	}) {
	}

	if peak > 4 {
		t.Fatalf("peak concurrency = %d, want at most 4", peak)
	}
}

func TestPoolStopsOnCancel(t *testing.T) {
	items := make([]int, 1000)
	ctx, cancel := context.WithCancel(context.Background())

	var processed int64
	out := Pool(ctx, items, 4, func(_ context.Context, _ int) (int, bool) {
		atomic.AddInt64(&processed, 1)
		time.Sleep(time.Millisecond)
		return 0, true
	})

	<-out
	cancel()

	drained := make(chan struct{})
	go func() {
		for range out {
		}
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(3 * time.Second):
		t.Fatal("pool did not shut down after cancel")
	}

	if atomic.LoadInt64(&processed) >= int64(len(items)) {
		t.Fatal("cancel did not stop the pool early")
	}
}

func TestPoolWithNoItems(t *testing.T) {
	count := 0
	for range Pool(context.Background(), []int{}, 8, func(_ context.Context, n int) (int, bool) {
		return n, true
	}) {
		count++
	}
	if count != 0 {
		t.Fatalf("got %d results from an empty item list", count)
	}
}
