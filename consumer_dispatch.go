package ginboot

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	consumerTracer = "github.com/klass-lk/ginboot/consumer"
	// consumerSpanName is stable and shared by every consumer, with the
	// consumer's own name carried as an attribute — for the reason workerSpanName
	// gives: a span named after the consumer makes "show me every event handled"
	// impossible to express as one query.
	consumerSpanName = "ginboot.consumer.handle"
)

// DispatchResult is what came of handing a batch to a consumer.
type DispatchResult struct {
	// FailedIDs are the records that did not succeed, in the order they were
	// given. The caller reports these to the source so that only they are
	// redelivered.
	FailedIDs []string
	// Errors is one entry per failed record, keyed by id, for logging.
	Errors map[string]error
}

// Dispatch hands a batch of records to a consumer, one at a time.
//
// One record at a time, and one span each, because the unit that succeeds or
// fails is the record: a batch of ten where the fourth message is malformed must
// redeliver the fourth and none of the others. Handing the whole batch to the
// handler would make that distinction impossible to draw, and the usual result
// is the other nine being processed twice.
//
// A record that panics is a failed record, not a failed invocation. Letting the
// panic escape would fail the whole batch — including records already handled
// successfully, which would then be redelivered and handled again.
func (r *ConsumerRegistry) Dispatch(ctx context.Context, c Consumer, events []Event) DispatchResult {
	result := DispatchResult{Errors: make(map[string]error)}

	for _, ev := range events {
		// A context that has been cancelled means the runtime is reclaiming this
		// invocation. Everything not yet attempted is left unacknowledged so the
		// source redelivers it, rather than being marked done by a handler that
		// had no time to run.
		if err := ctx.Err(); err != nil {
			result.FailedIDs = append(result.FailedIDs, ev.ID)
			result.Errors[ev.ID] = err
			continue
		}

		if err := r.handleSafe(ctx, c, ev); err != nil {
			result.FailedIDs = append(result.FailedIDs, ev.ID)
			result.Errors[ev.ID] = err
		}
	}

	return result
}

// handleSafe runs one record, converting a panic into an error and recording a
// span either way.
func (r *ConsumerRegistry) handleSafe(ctx context.Context, c Consumer, ev Event) (err error) {
	source := c.Source()

	// Registered before the recovery below so that it closes after it — defers
	// run last-registered-first — and therefore observes the error a panic
	// produced rather than reporting the record as handled.
	//
	// otel.Tracer resolves to a no-op unless an application imported
	// instrumentation, so this costs an interface call in the applications that
	// did not.
	ctx, span := otel.Tracer(consumerTracer).Start(ctx, consumerSpanName,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("ginboot.consumer", c.Name()),
			attribute.String("ginboot.consumer.kind", string(source.Kind)),
			attribute.String("ginboot.consumer.source", source.Ref),
			attribute.String("ginboot.consumer.record_id", ev.ID),
		))
	start := time.Now()
	defer func() {
		span.SetAttributes(
			attribute.String("ginboot.consumer.outcome", outcomeOf(err)),
			attribute.Int64("ginboot.consumer.duration_ms", time.Since(start).Milliseconds()),
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}()

	defer func() {
		if rec := recover(); rec != nil {
			stack := string(debug.Stack())
			err = fmt.Errorf("consumer %q panicked handling %s: %v", c.Name(), ev.ID, rec)
			r.logger.Error(fmt.Sprintf("[Consumers] %v\n%s", err, stack))
		}
	}()

	err = c.Handle(ctx, ev)
	if err != nil {
		r.logger.Error(fmt.Sprintf("[Consumers] consumer %q failed on %s after %v: %v",
			c.Name(), ev.ID, time.Since(start), err))
	}
	return err
}
