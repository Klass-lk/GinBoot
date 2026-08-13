package ginboot

import (
	"context"
	"fmt"
	"sync"

	"github.com/klass-lk/ginboot/config"
)

// Instrumenter installs observability on a server and returns a function that
// drains whatever it buffered.
//
// It exists so that ginboot.yml's telemetry block can actually switch telemetry
// on, which the obvious approach cannot do. Instrumentation lives in
// github.com/klass-lk/ginboot/telemetry, a separate module that imports this
// one; for New to call it directly this package would have to import it back,
// which is a package cycle. Inverting the pair instead — moving telemetry under
// this package — would put the whole OpenTelemetry SDK in the dependency graph
// of every ginboot application, including the ones that will never emit a span.
//
// So the dependency stays pointing this way and the instrumentation registers
// itself, in the shape database/sql uses for its drivers: the application takes
// a blank import of the telemetry package, whose init calls
// RegisterInstrumenter, and configuration decides the rest.
type Instrumenter func(ctx context.Context, s *Server, cfg config.TelemetryConfig) (shutdown func(context.Context) error, err error)

var (
	instrumenterMu sync.RWMutex
	instrumenter   Instrumenter
)

// RegisterInstrumenter records the instrumentation an application has imported.
// It is called from the init of a package such as
// github.com/klass-lk/ginboot/telemetry, not by application code.
//
// The last registration wins, so an application can supply its own in place of
// the bundled one.
func RegisterInstrumenter(f Instrumenter) {
	instrumenterMu.Lock()
	defer instrumenterMu.Unlock()
	instrumenter = f
}

func registeredInstrumenter() Instrumenter {
	instrumenterMu.RLock()
	defer instrumenterMu.RUnlock()
	return instrumenter
}

// ClaimInstrumentation reports whether the caller is the first to install
// observability middleware on this server, and records that it has.
//
// Instrumentation is middleware, and gin has no notion of registering the same
// middleware twice — a second pass silently doubles every span, log line and
// metric for the rest of the process. An application that both imports the
// telemetry package and calls Instrument itself would do exactly that, so the
// second caller is told to do nothing rather than left to corrupt the output.
//
// Called by instrumentation packages, not by application code.
func (s *Server) ClaimInstrumentation() bool {
	s.instrumentedMu.Lock()
	defer s.instrumentedMu.Unlock()
	if s.instrumented {
		return false
	}
	s.instrumented = true
	return true
}

// instrumentFromConfig turns telemetry on when ginboot.yml asks for it.
//
// Called from New so that the middleware is in place before any route is
// registered: gin applies middleware to the routes declared after it, so
// instrumenting later would quietly leave the application's own endpoints — the
// ones worth observing — uninstrumented.
//
// Nothing here reaches the network. Setting up exporters builds clients without
// dialing, and every record they take afterwards leaves on a background
// goroutine, so no request ever waits on telemetry.
func (s *Server) instrumentFromConfig() {
	if s.config == nil || !s.config.Ginboot.Telemetry.Enabled {
		return
	}

	instrument := registeredInstrumenter()
	if instrument == nil {
		// Config asked for something the binary cannot do. Saying so is the
		// whole point: the alternative is an application that looks configured
		// for telemetry, reports none, and gives no hint that one import is
		// missing.
		fmt.Println("[ginboot] telemetry.enabled is set but no instrumentation is registered; " +
			"add: import _ \"github.com/klass-lk/ginboot/telemetry\"")
		return
	}

	shutdown, err := instrument(context.Background(), s, s.config.Ginboot.Telemetry)
	if err != nil {
		// Telemetry is never a reason to refuse to serve.
		fmt.Printf("[ginboot] telemetry setup failed, continuing without it: %v\n", err)
		return
	}
	s.telemetryShutdown = shutdown
}

// Shutdown drains anything instrumentation is holding, and returns once it has
// been handed to its exporters or ctx expires.
//
// Worth calling where a process ends on purpose. It is not useful on a runtime
// that is suspended rather than stopped, such as AWS Lambda, which never gets
// far enough to run it.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.telemetryShutdown == nil {
		return nil
	}
	return s.telemetryShutdown(ctx)
}
