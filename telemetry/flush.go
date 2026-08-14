package telemetry

import (
	"context"
	"errors"
	"sync"
	"time"
)

// flusher is implemented by all three SDK providers.
type flusher interface {
	ForceFlush(context.Context) error
}

var (
	flushMu      sync.RWMutex
	flushTargets []flusher
)

func setFlushTargets(targets ...flusher) {
	flushMu.Lock()
	defer flushMu.Unlock()

	flushTargets = flushTargets[:0]
	for _, target := range targets {
		// A nil *TracerProvider arrives as a non-nil interface, so the concrete
		// value is what has to be checked before calling through it.
		if target == nil {
			continue
		}
		flushTargets = append(flushTargets, target)
	}
}

// Flush drains buffered telemetry, returning once it has been handed to the
// exporters or the context expires.
//
// Telemetry is batched rather than exported as it is produced, because exporting
// inline puts a network round trip on the path of every log line and every span:
// with a collector in another region that is several hundred milliseconds each
// time, enough to push a cold start past the platform's initialisation limit and
// to add that cost to every request. The trade is that records sit in a buffer
// for a moment, which is invisible on a long-lived server but matters on a
// runtime that can be suspended between requests — AWS Lambda freezes the
// execution environment, and a frozen process exports nothing.
//
// Call this at the end of an invocation on such a runtime. It is safe to call
// when telemetry was never configured, and safe to call concurrently.
func Flush(ctx context.Context) error {
	flushMu.RLock()
	targets := make([]flusher, len(flushTargets))
	copy(targets, flushTargets)
	flushMu.RUnlock()

	if len(targets) == 0 {
		return nil
	}

	// A flush that hangs must not become the stall it was meant to prevent.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
	}

	// Concurrently, because the deadline is shared and the targets are not:
	// traces, metrics and logs go to three different endpoints over three
	// different connections, and nothing about one export informs another.
	//
	// Run one after another they compete for the same budget instead of using
	// it: the first takes what a round trip costs, the second gets the
	// remainder, and the third — reliably the logs — gets none and fails
	// without ever being attempted. The symptom is telemetry that arrives
	// partially and always in the same order, which reads like a backend
	// problem rather than a scheduling one.
	//
	// Running them together also shortens the drain to the slowest single
	// export rather than the sum of all three, and on a runtime where this time
	// is billed that is the difference worth having.
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	wg.Add(len(targets))

	for i, target := range targets {
		go func(i int, target flusher) {
			defer wg.Done()
			errs[i] = target.ForceFlush(ctx)
		}(i, target)
	}
	wg.Wait()

	return errors.Join(errs...)
}
