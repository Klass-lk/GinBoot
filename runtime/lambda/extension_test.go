package lambda

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klass-lk/ginboot"
)

// fakeExtensionsAPI stands in for Lambda. It hands out one INVOKE per call to
// event/next and records how the extension behaves against it.
type fakeExtensionsAPI struct {
	server *httptest.Server

	registerCalls atomic.Int32
	nextCalls     atomic.Int32
	registeredFor atomic.Value // string: the events body it was asked to register for
	extensionName atomic.Value // string

	registerStatus int

	mu       sync.Mutex
	invokes  int // how many INVOKE events remain to hand out
	released chan struct{}
}

func newFakeExtensionsAPI(t *testing.T, invokes int) *fakeExtensionsAPI {
	t.Helper()
	api := &fakeExtensionsAPI{invokes: invokes, registerStatus: http.StatusOK, released: make(chan struct{}, invokes+1)}

	mux := http.NewServeMux()
	mux.HandleFunc("/2020-01-01/extension/register", func(w http.ResponseWriter, r *http.Request) {
		api.registerCalls.Add(1)
		api.extensionName.Store(r.Header.Get("Lambda-Extension-Name"))
		body, _ := io.ReadAll(r.Body)
		api.registeredFor.Store(string(body))

		if api.registerStatus != http.StatusOK {
			w.WriteHeader(api.registerStatus)
			return
		}
		w.Header().Set("Lambda-Extension-Identifier", "test-extension-id")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"functionName":"test"}`))
	})
	mux.HandleFunc("/2020-01-01/extension/event/next", func(w http.ResponseWriter, r *http.Request) {
		api.nextCalls.Add(1)
		// Every call here is the extension reporting it has finished the previous
		// invocation, which is the signal Lambda would use to freeze.
		api.released <- struct{}{}

		api.mu.Lock()
		remaining := api.invokes
		if remaining > 0 {
			api.invokes--
		}
		api.mu.Unlock()

		if remaining <= 0 {
			// Nothing more to do; keep the long poll open briefly then end it so
			// the goroutine can exit with the test.
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"eventType":"INVOKE","deadlineMs":1}`))
	})

	api.server = httptest.NewServer(mux)
	t.Cleanup(api.server.Close)
	return api
}

// host returns the value Lambda would put in AWS_LAMBDA_RUNTIME_API: host:port,
// no scheme.
func (a *fakeExtensionsAPI) host() string {
	return strings.TrimPrefix(a.server.URL, "http://")
}

// waitForNextCalls waits until the extension has reported completion n times.
func (a *fakeExtensionsAPI) waitForNextCalls(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-a.released:
		case <-time.After(3 * time.Second):
			t.Fatalf("extension reported completion %d times, wanted %d", i, n)
		}
	}
}

func withFlusher(t *testing.T, f ginboot.Flusher) {
	t.Helper()
	ginboot.RegisterFlusher(f)
	t.Cleanup(func() { ginboot.RegisterFlusher(nil) })
}

func TestExtensionDrainsAfterEachInvocation(t *testing.T) {
	api := newFakeExtensionsAPI(t, 2)
	t.Setenv("AWS_LAMBDA_RUNTIME_API", api.host())

	var flushes atomic.Int32
	withFlusher(t, func(context.Context) error {
		flushes.Add(1)
		return nil
	})

	extension := startTelemetryExtension()
	if extension == nil {
		t.Fatal("extension should have registered")
	}

	// The first next call is the extension announcing it is ready.
	api.waitForNextCalls(t, 1)

	for i := 0; i < 2; i++ {
		extension.invocationComplete()
		api.waitForNextCalls(t, 1)
	}

	if got := flushes.Load(); got != 2 {
		t.Errorf("drained %d times, want one per invocation (2)", got)
	}
}

func TestExtensionRegistersForInvokeOnly(t *testing.T) {
	// Internal extensions may not register for SHUTDOWN; Lambda rejects the
	// registration outright, which would leave telemetry undrained.
	api := newFakeExtensionsAPI(t, 1)
	t.Setenv("AWS_LAMBDA_RUNTIME_API", api.host())
	withFlusher(t, func(context.Context) error { return nil })

	if startTelemetryExtension() == nil {
		t.Fatal("extension should have registered")
	}
	api.waitForNextCalls(t, 1)

	body, _ := api.registeredFor.Load().(string)
	if strings.Contains(strings.ToUpper(body), "SHUTDOWN") {
		t.Errorf("registered for SHUTDOWN, which Lambda refuses for internal extensions: %s", body)
	}
	if !strings.Contains(strings.ToUpper(body), "INVOKE") {
		t.Errorf("did not register for INVOKE: %s", body)
	}
	if name, _ := api.extensionName.Load().(string); name == "" {
		t.Error("Lambda-Extension-Name is required on register")
	}
}

// The invocation stays open until the extension asks for the next event. Any
// path that skips that turns a telemetry problem into a function timeout.
func TestExtensionAlwaysReportsCompletion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		flusher ginboot.Flusher
	}{
		{"flush fails", func(context.Context) error { return errors.New("collector unreachable") }},
		{"flush panics", func(context.Context) error { panic("boom") }},
		{"flush hangs past its deadline", func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeExtensionsAPI(t, 1)
			t.Setenv("AWS_LAMBDA_RUNTIME_API", api.host())
			t.Setenv("GINBOOT_TELEMETRY_FLUSH_TIMEOUT", "100ms")
			withFlusher(t, tt.flusher)

			extension := startTelemetryExtension()
			if extension == nil {
				t.Fatal("extension should have registered")
			}
			api.waitForNextCalls(t, 1)

			extension.invocationComplete()

			// The second call is the extension reporting the invocation done.
			api.waitForNextCalls(t, 1)
		})
	}
}

// A handler faster than the extension's own event/next round trip is the normal
// case, not an edge case: the token has to survive being sent early.
func TestCompletionSignalledBeforeTheEventIsSeenIsNotLost(t *testing.T) {
	api := newFakeExtensionsAPI(t, 1)
	t.Setenv("AWS_LAMBDA_RUNTIME_API", api.host())

	flushed := make(chan struct{}, 1)
	withFlusher(t, func(context.Context) error {
		flushed <- struct{}{}
		return nil
	})

	extension := startTelemetryExtension()
	if extension == nil {
		t.Fatal("extension should have registered")
	}

	// Signal immediately, racing the extension's own loop.
	extension.invocationComplete()

	select {
	case <-flushed:
	case <-time.After(3 * time.Second):
		t.Fatal("a completion signalled before the invoke event was seen was dropped")
	}
}

func TestInvocationCompleteNeverBlocks(t *testing.T) {
	// Called from a deferred statement on the response path, so it must return
	// whether or not anything is listening — including on a nil extension, which
	// is what every non-Lambda process has.
	var absent *telemetryExtension
	absent.invocationComplete()

	present := &telemetryExtension{handlerDone: make(chan struct{}, 1)}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			present.invocationComplete()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("invocationComplete blocked; it is called on the response path")
	}
}

func TestExtensionIsNotStartedWhenItWouldHaveNothingToDo(t *testing.T) {
	t.Run("not running on Lambda", func(t *testing.T) {
		t.Setenv("AWS_LAMBDA_RUNTIME_API", "")
		withFlusher(t, func(context.Context) error { return nil })

		if startTelemetryExtension() != nil {
			t.Error("no Lambda runtime API means no extension")
		}
	})

	t.Run("no telemetry compiled in", func(t *testing.T) {
		api := newFakeExtensionsAPI(t, 1)
		t.Setenv("AWS_LAMBDA_RUNTIME_API", api.host())
		ginboot.RegisterFlusher(nil)

		if startTelemetryExtension() != nil {
			t.Error("nothing to drain, so the environment should not be held open")
		}
		if api.registerCalls.Load() != 0 {
			t.Error("registered an extension with nothing to drain, which is billed time for no telemetry")
		}
	})
}

// Registration can fail — a name Lambda rejects, a runtime that does not support
// extensions. The function must still serve.
func TestFailedRegistrationLeavesTheRuntimeAlone(t *testing.T) {
	api := newFakeExtensionsAPI(t, 1)
	api.registerStatus = http.StatusForbidden
	t.Setenv("AWS_LAMBDA_RUNTIME_API", api.host())
	withFlusher(t, func(context.Context) error { return nil })

	extension := startTelemetryExtension()
	if extension != nil {
		t.Fatal("a rejected registration must not leave a live extension")
	}
	// And the nil it returned is still safe to signal, since the runner does so
	// unconditionally.
	extension.invocationComplete()
}

func TestFlushTimeoutIsConfigurable(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  time.Duration
	}{
		{"", defaultFlushTimeout},
		{"500ms", 500 * time.Millisecond},
		{"3s", 3 * time.Second},
		{"250", 250 * time.Millisecond}, // bare number reads as milliseconds
		{"nonsense", defaultFlushTimeout},
		{"-1s", defaultFlushTimeout},
		{"0", defaultFlushTimeout},
	} {
		t.Run("GINBOOT_TELEMETRY_FLUSH_TIMEOUT="+tt.value, func(t *testing.T) {
			t.Setenv("GINBOOT_TELEMETRY_FLUSH_TIMEOUT", tt.value)
			if got := flushTimeout(); got != tt.want {
				t.Errorf("flushTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
