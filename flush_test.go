package ginboot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withFlusher(t *testing.T, f Flusher) {
	t.Helper()

	flusherMu.Lock()
	previous := flusher
	flusherMu.Unlock()

	RegisterFlusher(f)

	t.Cleanup(func() {
		flusherMu.Lock()
		flusher = previous
		flusherMu.Unlock()
	})
}

func TestFlushTelemetryDrainsWhatIsRegistered(t *testing.T) {
	drained := false
	withFlusher(t, func(context.Context) error {
		drained = true
		return nil
	})

	require.NoError(t, FlushTelemetry(context.Background()))
	assert.True(t, drained)
}

func TestFlushTelemetryReportsWhatItCouldNotDrain(t *testing.T) {
	failure := errors.New("collector unreachable")
	withFlusher(t, func(context.Context) error { return failure })

	assert.ErrorIs(t, FlushTelemetry(context.Background()), failure)
}

func TestFlushTelemetryIsSafeWithoutInstrumentation(t *testing.T) {
	// The path every application that never enables telemetry takes, including
	// on a runtime that drains after each invocation.
	withFlusher(t, nil)

	assert.NoError(t, FlushTelemetry(context.Background()))
	assert.False(t, TelemetryBuffered())
}

func TestTelemetryBufferedReportsWhetherThereIsAnythingToDrain(t *testing.T) {
	withFlusher(t, nil)
	assert.False(t, TelemetryBuffered(), "nothing registered means nothing to drain")

	withFlusher(t, func(context.Context) error { return nil })
	assert.True(t, TelemetryBuffered())
}

// Draining is a serverless concern: it exists because AWS Lambda freezes the
// execution environment between invocations, and it costs billed time to hold
// that environment open. A long-running HTTP server has no such problem — its
// exporters run continuously in the background — so nothing on the request path
// may drain anything.
//
// This guards the boundary from the framework side. The drain is arranged
// entirely within runtime/lambda, a separate module an HTTP-only application
// never compiles in, and it is inert there unless AWS_LAMBDA_RUNTIME_API is set.
func TestServingHTTPNeverDrainsTelemetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var drains atomic.Int32
	withFlusher(t, func(context.Context) error {
		drains.Add(1)
		return nil
	})

	server := New()
	server.Engine().GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 25; i++ {
		recorder := httptest.NewRecorder()
		server.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
	}

	assert.Zero(t, drains.Load(),
		"an HTTP request must never wait on telemetry; draining belongs to runtimes that get frozen")
}
