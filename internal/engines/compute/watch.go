package compute

import (
	"context"
	"runtime/metrics"
	"time"

	"go.starlark.net/starlark"
)

const heapMetric = "/memory/classes/heap/objects:bytes"

// watch cancels a thread when its context ends or when the heap grows by
// more than limit bytes while it runs. It returns a function that stops
// watching. The heap is the whole process's, so the limit is generous: it
// exists to stop a runaway script before it takes the engine down.
func watch(ctx context.Context, thread *starlark.Thread, limit uint64) func() {
	done := make(chan struct{})
	sample := []metrics.Sample{{Name: heapMetric}}
	metrics.Read(sample)
	start := sample[0].Value.Uint64()
	go func() {
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				thread.Cancel(ctx.Err().Error())
				return
			case <-tick.C:
				metrics.Read(sample)
				if now := sample[0].Value.Uint64(); now > start && now-start > limit {
					thread.Cancel("memory limit")
					return
				}
			}
		}
	}()
	return func() { close(done) }
}
