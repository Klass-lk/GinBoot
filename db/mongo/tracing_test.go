package mongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestTracingMonitorRecordsCommandSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("test")
	var forwarded []string
	next := &event.CommandMonitor{
		Started:   func(context.Context, *event.CommandStartedEvent) { forwarded = append(forwarded, "started") },
		Succeeded: func(context.Context, *event.CommandSucceededEvent) { forwarded = append(forwarded, "succeeded") },
		Failed:    func(context.Context, *event.CommandFailedEvent) { forwarded = append(forwarded, "failed") },
	}
	m := tracingMonitor(tracer, next)

	parentCtx, parent := tracer.Start(context.Background(), "request")
	find := started(t, "find", bson.D{{Key: "find", Value: "users"}, {Key: "filter", Value: bson.D{{Key: "password", Value: "secret"}}}})
	find.RequestID = 1
	m.Started(parentCtx, find)
	m.Succeeded(parentCtx, &event.CommandSucceededEvent{CommandFinishedEvent: event.CommandFinishedEvent{RequestID: 1}})

	more := started(t, "getMore", bson.D{{Key: "getMore", Value: int64(7)}, {Key: "collection", Value: "users"}})
	more.RequestID = 2
	m.Started(parentCtx, more)
	m.Failed(parentCtx, &event.CommandFailedEvent{CommandFinishedEvent: event.CommandFinishedEvent{RequestID: 2}, Failure: "cursor killed"})

	m.Started(parentCtx, started(t, "hello", bson.D{{Key: "hello", Value: 1}}))
	parent.End()

	spans := recorder.Ended()
	require.Len(t, spans, 3) // find, getMore, request; no span for hello
	assert.Equal(t, "find users", spans[0].Name())
	assert.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent().SpanID())
	assert.Contains(t, spans[0].Attributes(), attribute.String("db.collection.name", "users"))
	assert.Contains(t, spans[0].Attributes(), attribute.String("db.namespace", "app"))
	for _, a := range spans[0].Attributes() {
		assert.NotContains(t, a.Value.Emit(), "secret", "command bodies are never recorded")
	}
	assert.Equal(t, codes.Unset, spans[0].Status().Code)
	assert.Equal(t, "getMore users", spans[1].Name())
	assert.Equal(t, codes.Error, spans[1].Status().Code)
	assert.Equal(t, "cursor killed", spans[1].Status().Description)
	assert.Equal(t, []string{"started", "succeeded", "started", "failed", "started"}, forwarded)
}

// BenchmarkTracingMonitor is one command's Started+Succeeded: what the
// monitor adds to a query, which itself takes ~0.3-5ms on the network.
func BenchmarkTracingMonitor(b *testing.B) {
	raw, _ := bson.Marshal(bson.D{{Key: "find", Value: "sessions"}, {Key: "filter", Value: bson.D{{Key: "moduleId", Value: "m"}}}})
	run := func(b *testing.B, tracer trace.Tracer, parent context.Context) {
		m := tracingMonitor(tracer, nil)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			e := &event.CommandStartedEvent{CommandName: "find", DatabaseName: "app", Command: raw, RequestID: int64(i)}
			m.Started(parent, e)
			m.Succeeded(parent, &event.CommandSucceededEvent{CommandFinishedEvent: event.CommandFinishedEvent{RequestID: int64(i)}})
		}
	}
	b.Run("telemetry off (no-op provider)", func(b *testing.B) {
		run(b, noop.NewTracerProvider().Tracer("x"), context.Background())
	})
	b.Run("sampled out", func(b *testing.B) {
		tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
		run(b, tp.Tracer("x"), context.Background())
	})
	b.Run("recorded, batch exporter", func(b *testing.B) {
		tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(discard{}))
		defer tp.Shutdown(context.Background())
		ctx, parent := tp.Tracer("x").Start(context.Background(), "request")
		defer parent.End()
		run(b, tp.Tracer("x"), ctx)
	})
}

type discard struct{}

func (discard) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }
func (discard) Shutdown(context.Context) error                             { return nil }
