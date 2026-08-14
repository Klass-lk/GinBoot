package ginboot

import (
	"context"
	"sync"
)

// Flusher drains buffered telemetry, returning once it has been handed to its
// exporters or ctx expires.
//
// It is registered by an instrumentation package, in the same way and for the
// same reason as Instrumenter: this package must not import the OpenTelemetry
// SDK, and a runtime that needs to drain telemetry — runtime/lambda — must not
// have to either.
type Flusher func(context.Context) error

var (
	flusherMu sync.RWMutex
	flusher   Flusher
)

// RegisterFlusher records how to drain buffered telemetry. Called from the init
// of a package such as github.com/klass-lk/ginboot/telemetry, not by
// application code.
func RegisterFlusher(f Flusher) {
	flusherMu.Lock()
	defer flusherMu.Unlock()
	flusher = f
}

// TelemetryBuffered reports whether anything is registered that could need
// draining.
//
// A runtime uses this to decide whether draining is worth arranging at all.
// Arranging it is not free — on AWS Lambda it means holding the execution
// environment open past the response — so a process with no instrumentation
// compiled in should not pay for the machinery.
func TelemetryBuffered() bool {
	flusherMu.RLock()
	defer flusherMu.RUnlock()
	return flusher != nil
}

// FlushTelemetry drains buffered telemetry, or does nothing when no
// instrumentation is registered. Safe to call from any goroutine.
func FlushTelemetry(ctx context.Context) error {
	flusherMu.RLock()
	f := flusher
	flusherMu.RUnlock()

	if f == nil {
		return nil
	}
	return f(ctx)
}
