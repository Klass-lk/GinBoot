package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
)

// TestLoggingDoesNotBlockOnSlowCollector is the regression test for a cold start
// that timed out.
//
// Telemetry used to be exported inline on a serverless runtime, so every log
// record was a round trip to the collector before the next line of application
// code ran. With the collector in another region that was several hundred
// milliseconds each, which pushed initialisation past the platform's limit and
// added the same cost to every request.
//
// The collector here is deliberately slower than any real one. If export is on
// the caller's path again, this test takes seconds rather than milliseconds.
func TestLoggingDoesNotBlockOnSlowCollector(t *testing.T) {
	const collectorDelay = 2 * time.Second

	var requests atomic.Int64
	released := make(chan struct{})
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		// Slow, but released at cleanup so the test does not wait on it: an
		// httptest server's Close blocks until its handlers return.
		select {
		case <-time.After(collectorDelay):
		case <-released:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() {
		close(released)
		collector.Close()
	})

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	// The runtime is identified by this variable, and the bug only appeared there.
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-function")

	shutdown, err := Setup(context.Background(), "test-service", "v0.0.1")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = shutdown(ctx)
	}()

	logger := slog.New(otelHandlerForTest("test-service"))

	start := time.Now()
	for i := 0; i < 5; i++ {
		logger.Info("request completed", slog.Int("attempt", i))
	}
	elapsed := time.Since(start)

	// Five records against a two-second collector is ten seconds if each one
	// waits for its own export.
	if elapsed > collectorDelay {
		t.Errorf("emitting 5 log records took %v; export is on the caller's path", elapsed)
	}
}

// TestFlushWithoutTelemetryConfigured keeps the drain hook safe to call
// unconditionally, including before Setup has ever run.
func TestFlushWithoutTelemetryConfigured(t *testing.T) {
	setFlushTargets()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := Flush(ctx); err != nil {
		t.Errorf("Flush with nothing configured returned %v", err)
	}
}

// TestFlushIsBoundedWhenCollectorHangs keeps the drain hook from becoming the
// stall it exists to prevent.
func TestFlushIsBoundedWhenCollectorHangs(t *testing.T) {
	released := make(chan struct{})
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-released:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(released)
		collector.Close()
	})

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	shutdown, err := Setup(context.Background(), "test-service", "v0.0.1")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = shutdown(ctx)
	}()

	logger := slog.New(otelHandlerForTest("test-service"))
	logger.Info("a record to make the flush do work")

	start := time.Now()
	// No deadline supplied: Flush must impose its own.
	_ = Flush(context.Background())
	elapsed := time.Since(start)

	// The self-imposed bound is a small number of seconds; anything approaching
	// the collector's indefinite hang means the bound is not being applied.
	if elapsed > 8*time.Second {
		t.Errorf("Flush took %v against a hanging collector; it must bound itself", elapsed)
	}
	t.Logf("Flush returned after %v against a hanging collector", elapsed)
}

// otelHandlerForTest builds the same slog handler Instrument installs, so these
// tests exercise the path application logging actually takes.
func otelHandlerForTest(service string) slog.Handler {
	return otelslog.NewHandler(service)
}
