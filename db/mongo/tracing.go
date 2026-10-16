package mongo

import (
	"context"
	"sync"

	"go.mongodb.org/mongo-driver/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/klass-lk/ginboot/db/mongo"

// untracedCommands are the driver's own housekeeping, which would only add
// noise to a request's trace.
var untracedCommands = map[string]bool{
	"hello": true, "isMaster": true, "ismaster": true, "ping": true, "saslStart": true,
	"saslContinue": true, "authenticate": true, "endSessions": true, "buildInfo": true,
	"getnonce": true,
}

// TracingMonitor returns a command monitor that records each command as an
// OpenTelemetry client span, named "<command> <collection>" and parented by
// the span in the context the command ran with. A span carries the
// database, collection, command and server, and a failed command marks it
// as an error. Command bodies are never recorded: they hold user data.
//
// Monitors chain, so it combines with the guards:
//
//	opts.SetMonitor(mongo.TracingMonitor(mongo.ReadOnlyMonitor(nil)))
//
// next, if not nil, receives every event after the span is started or
// ended.
func TracingMonitor(next *event.CommandMonitor) *event.CommandMonitor {
	return tracingMonitor(otel.GetTracerProvider().Tracer(tracerName), next)
}

func tracingMonitor(tracer trace.Tracer, next *event.CommandMonitor) *event.CommandMonitor {
	var spans sync.Map // request id -> trace.Span
	finish := func(id int64, err string) {
		s, ok := spans.LoadAndDelete(id)
		if !ok {
			return
		}
		span := s.(trace.Span)
		if err != "" {
			span.SetStatus(codes.Error, err)
		}
		span.End()
	}
	return &event.CommandMonitor{
		Started: func(ctx context.Context, e *event.CommandStartedEvent) {
			if !untracedCommands[e.CommandName] {
				coll := commandCollection(e)
				name := e.CommandName
				if coll != "" {
					name += " " + coll
				}
				_, span := tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient),
					trace.WithAttributes(
						attribute.String("db.system", "mongodb"),
						attribute.String("db.namespace", e.DatabaseName),
						attribute.String("db.collection.name", coll),
						attribute.String("db.operation.name", e.CommandName),
						attribute.String("db.mongodb.connection_id", e.ConnectionID),
					))
				spans.Store(e.RequestID, span)
			}
			if next != nil && next.Started != nil {
				next.Started(ctx, e)
			}
		},
		Succeeded: func(ctx context.Context, e *event.CommandSucceededEvent) {
			finish(e.RequestID, "")
			if next != nil && next.Succeeded != nil {
				next.Succeeded(ctx, e)
			}
		},
		Failed: func(ctx context.Context, e *event.CommandFailedEvent) {
			msg := e.Failure
			if msg == "" {
				msg = "command failed"
			}
			finish(e.RequestID, msg)
			if next != nil && next.Failed != nil {
				next.Failed(ctx, e)
			}
		},
	}
}

// commandCollection is the collection a command names: the command's own
// value for most commands, and the "collection" field for getMore.
func commandCollection(e *event.CommandStartedEvent) string {
	if coll, ok := e.Command.Lookup(e.CommandName).StringValueOK(); ok {
		return coll
	}
	if coll, ok := e.Command.Lookup("collection").StringValueOK(); ok {
		return coll
	}
	return ""
}
