package telemetry

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// slowFlusher stands in for a provider whose export is a round trip to a distant
// collector.
type slowFlusher struct {
	name     string
	duration time.Duration

	attempted atomic.Bool
	succeeded atomic.Bool
}

func (s *slowFlusher) ForceFlush(ctx context.Context) error {
	s.attempted.Store(true)
	select {
	case <-time.After(s.duration):
		s.succeeded.Store(true)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func withTargets(t *testing.T, targets ...flusher) {
	t.Helper()

	flushMu.Lock()
	previous := make([]flusher, len(flushTargets))
	copy(previous, flushTargets)
	flushMu.Unlock()

	setFlushTargets(targets...)

	t.Cleanup(func() {
		flushMu.Lock()
		flushTargets = previous
		flushMu.Unlock()
	})
}

// Traces, metrics and logs share the drain's deadline but not its work. Drained
// one after another they compete for that deadline: the first spends what a
// round trip costs, and the last is never attempted at all — which is why a
// service could show traces arriving while its logs never did.
func TestFlushGivesEveryTargetTheWholeDeadline(t *testing.T) {
	// Three exports of 300ms each. Sequentially that is 900ms and busts the
	// budget; concurrently it is 300ms and does not.
	traces := &slowFlusher{name: "traces", duration: 300 * time.Millisecond}
	metrics := &slowFlusher{name: "metrics", duration: 300 * time.Millisecond}
	logs := &slowFlusher{name: "logs", duration: 300 * time.Millisecond}
	withTargets(t, traces, metrics, logs)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := Flush(ctx); err != nil {
		t.Fatalf("Flush() = %v, want every target drained within the deadline", err)
	}

	for _, target := range []*slowFlusher{traces, metrics, logs} {
		if !target.attempted.Load() {
			t.Errorf("%s was never even attempted", target.name)
		}
		if !target.succeeded.Load() {
			t.Errorf("%s did not finish inside the shared deadline", target.name)
		}
	}
}

// The drain is billed time on a runtime that freezes, so it should cost the
// slowest export rather than the sum of them.
func TestFlushCostsTheSlowestExportNotTheirSum(t *testing.T) {
	const each = 200 * time.Millisecond
	withTargets(t,
		&slowFlusher{name: "traces", duration: each},
		&slowFlusher{name: "metrics", duration: each},
		&slowFlusher{name: "logs", duration: each},
	)

	started := time.Now()
	if err := Flush(context.Background()); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
	elapsed := time.Since(started)

	// Generous headroom for scheduling; the point is that it is nowhere near
	// the 600ms three sequential exports would take.
	if elapsed > 2*each {
		t.Errorf("drain took %v for three %v exports, which means they ran one after another", elapsed, each)
	}
}

// One unreachable collector must not hide the others, nor stop them draining.
func TestOneFailingTargetDoesNotStopTheRest(t *testing.T) {
	fast := &slowFlusher{name: "traces", duration: 10 * time.Millisecond}
	slow := &slowFlusher{name: "metrics", duration: 5 * time.Second}
	withTargets(t, slow, fast)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := Flush(ctx)

	if err == nil {
		t.Error("a target that could not be drained should be reported")
	}
	if !fast.succeeded.Load() {
		t.Error("a reachable collector was not drained because another one hung")
	}
}

func TestFlushWithNothingRegistered(t *testing.T) {
	withTargets(t)
	if err := Flush(context.Background()); err != nil {
		t.Errorf("Flush() = %v, want nil when there is nothing to drain", err)
	}
}
