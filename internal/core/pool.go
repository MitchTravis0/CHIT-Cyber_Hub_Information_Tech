package core

import (
	"context"
	"sync"
)

// Pool runs fn over items with at most workers goroutines and streams the
// results back as they finish. fn returns (result, keep): keep=false drops the
// item (e.g. a host that did not answer). The channel is closed when all items
// are processed or ctx is cancelled, so callers can simply range over it.
func Pool[T any, R any](ctx context.Context, items []T, workers int, fn func(context.Context, T) (R, bool)) <-chan R {
	if workers < 1 {
		workers = 1
	}
	if workers > len(items) {
		workers = len(items)
	}

	in := make(chan T)
	out := make(chan R, workers)

	go func() {
		defer close(in)
		for _, item := range items {
			select {
			case in <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for item := range in {
				if ctx.Err() != nil {
					return
				}
				result, keep := fn(ctx, item)
				if !keep {
					continue
				}
				select {
				case out <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
