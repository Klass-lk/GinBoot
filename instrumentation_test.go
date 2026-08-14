package ginboot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The registry is package-level state, so a test that installs one has to put
// back what it found. None of these may run in parallel for the same reason.
func withInstrumenter(t *testing.T, f Instrumenter) {
	t.Helper()

	instrumenterMu.Lock()
	previous := instrumenter
	instrumenterMu.Unlock()

	RegisterInstrumenter(f)

	t.Cleanup(func() {
		instrumenterMu.Lock()
		instrumenter = previous
		instrumenterMu.Unlock()
	})
}

// writeConfig puts a ginboot.yml where New will find it, which is the working
// directory. The directory is the test's own, so the file goes away with it.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ginboot.yml"), []byte(body), 0o600))
	t.Chdir(dir)
}

const telemetryEnabled = `ginboot:
  telemetry:
    enabled: true
    service-name: probe-app
    service-version: v9.9.9
`

const telemetryDisabled = `ginboot:
  telemetry:
    enabled: false
    service-name: probe-app
`

func TestNewInstrumentsWhenTheConfigAsksForIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writeConfig(t, telemetryEnabled)

	var (
		called bool
		got    config.TelemetryConfig
	)
	withInstrumenter(t, func(_ context.Context, _ *Server, cfg config.TelemetryConfig) (func(context.Context) error, error) {
		called = true
		got = cfg
		return nil, nil
	})

	New()

	assert.True(t, called, "telemetry.enabled is set, so the registered instrumenter should run")
	// The instrumenter is handed the configuration rather than left to read the
	// file again, which is the whole reason the block is worth parsing.
	assert.Equal(t, "probe-app", got.ServiceName)
	assert.Equal(t, "v9.9.9", got.ServiceVersion)
}

func TestNewLeavesInstrumentationOffUnlessAskedTo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name string
		body string
	}{
		{"disabled explicitly", telemetryDisabled},
		{"no telemetry block at all", "ginboot:\n  server:\n    port: 8080\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writeConfig(t, tt.body)

			called := false
			withInstrumenter(t, func(_ context.Context, _ *Server, _ config.TelemetryConfig) (func(context.Context) error, error) {
				called = true
				return nil, nil
			})

			New()

			assert.False(t, called, "instrumentation must be opt-in: importing the package cannot be enough")
		})
	}
}

// The ordering that makes this worth doing in New at all. Middleware reaches the
// routes declared after it, so instrumenting any later would leave the
// application's own endpoints — the ones worth observing — uninstrumented.
func TestInstrumentationIsInstalledBeforeAnyRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writeConfig(t, telemetryEnabled)

	withInstrumenter(t, func(_ context.Context, s *Server, _ config.TelemetryConfig) (func(context.Context) error, error) {
		s.Engine().Use(func(c *gin.Context) {
			c.Header("X-Instrumented", "yes")
			c.Next()
		})
		return nil, nil
	})

	server := New()

	// Registered after New returns, exactly as an application would.
	server.Engine().GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "yes", recorder.Header().Get("X-Instrumented"),
		"a route declared after New must still be covered")
}

// Telemetry is never a reason to refuse to serve. Both ways it can be
// unavailable leave a server that works.
func TestTheServerStartsWhenInstrumentationCannot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("nothing registered", func(t *testing.T) {
		writeConfig(t, telemetryEnabled)
		withInstrumenter(t, nil)

		server := New()

		require.NotNil(t, server)
		assert.Nil(t, server.telemetryShutdown)
		assertServes(t, server)
	})

	t.Run("registered but failing", func(t *testing.T) {
		writeConfig(t, telemetryEnabled)
		withInstrumenter(t, func(_ context.Context, _ *Server, _ config.TelemetryConfig) (func(context.Context) error, error) {
			return nil, errors.New("collector unreachable")
		})

		server := New()

		require.NotNil(t, server)
		// A failed setup must not leave a shutdown behind to be called later.
		assert.Nil(t, server.telemetryShutdown)
		assertServes(t, server)
	})
}

func assertServes(t *testing.T, server *Server) {
	t.Helper()
	server.Engine().GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestLastRegistrationWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writeConfig(t, telemetryEnabled)

	first, second := false, false
	withInstrumenter(t, func(_ context.Context, _ *Server, _ config.TelemetryConfig) (func(context.Context) error, error) {
		first = true
		return nil, nil
	})
	RegisterInstrumenter(func(_ context.Context, _ *Server, _ config.TelemetryConfig) (func(context.Context) error, error) {
		second = true
		return nil, nil
	})

	New()

	assert.False(t, first, "an application supplying its own instrumentation should displace the bundled one")
	assert.True(t, second)
}

// Gin has no notion of registering the same middleware once. Without this, an
// application that both imports the telemetry package and calls Instrument
// itself doubles every span, log line and metric for the life of the process.
func TestClaimInstrumentationSucceedsOnlyOnce(t *testing.T) {
	server := &Server{}

	assert.True(t, server.ClaimInstrumentation(), "the first caller installs the middleware")
	assert.False(t, server.ClaimInstrumentation(), "the second must be told to do nothing")
	assert.False(t, server.ClaimInstrumentation())
}

func TestClaimInstrumentationIsSafeUnderConcurrency(t *testing.T) {
	server := &Server{}

	const racers = 64
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		claims int
	)
	wg.Add(racers)
	for range racers {
		go func() {
			defer wg.Done()
			if server.ClaimInstrumentation() {
				mu.Lock()
				claims++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, claims, "exactly one caller may install the middleware")
}

func TestClaimsAreNotSharedBetweenServers(t *testing.T) {
	// The flag belongs to a server, not to the process: a test suite or a host
	// standing up several servers must be able to instrument each of them.
	first, second := &Server{}, &Server{}

	assert.True(t, first.ClaimInstrumentation())
	assert.True(t, second.ClaimInstrumentation())
}

func TestShutdownRunsWhatInstrumentationLeftBehind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writeConfig(t, telemetryEnabled)

	drained := false
	withInstrumenter(t, func(_ context.Context, _ *Server, _ config.TelemetryConfig) (func(context.Context) error, error) {
		return func(context.Context) error {
			drained = true
			return nil
		}, nil
	})

	server := New()
	require.NoError(t, server.Shutdown(context.Background()))
	assert.True(t, drained, "Shutdown must drain what the instrumenter buffered")
}

func TestShutdownReportsWhatItCouldNotDrain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writeConfig(t, telemetryEnabled)

	failure := errors.New("flush timed out")
	withInstrumenter(t, func(_ context.Context, _ *Server, _ config.TelemetryConfig) (func(context.Context) error, error) {
		return func(context.Context) error { return failure }, nil
	})

	server := New()
	assert.ErrorIs(t, server.Shutdown(context.Background()), failure)
}

func TestShutdownIsSafeWithoutInstrumentation(t *testing.T) {
	// Every application that never enables telemetry takes this path, so it must
	// not be the thing that panics on the way out.
	assert.NoError(t, (&Server{}).Shutdown(context.Background()))
}
