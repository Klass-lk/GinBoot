package ginboot

import (
	"context"
	"fmt"
	"os"
	"strconv"
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
	if !telemetryRequested(s.config) {
		return
	}

	instrument := registeredInstrumenter()
	if instrument == nil {
		// Telemetry was asked for and the binary cannot provide it. Saying so is
		// the whole point: the alternative is an application that looks
		// configured for telemetry, reports none, and gives no hint that one
		// import is missing.
		fmt.Println("[ginboot] telemetry was requested (ginboot.yml or OTEL_EXPORTER_OTLP_ENDPOINT) " +
			"but no instrumentation is registered; add: import _ \"github.com/klass-lk/ginboot/telemetry\"")
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

// telemetryRequested reports whether this process should be instrumented.
//
// Configuration is one way to ask, and an environment carrying somewhere to
// send telemetry is the other. The second exists because the file does not
// always survive the trip: a deployment often ships a compiled binary and
// nothing else, so ginboot.yml is absent at runtime and telemetry.enabled reads
// as false no matter what the repository says. A platform that has gone to the
// trouble of pointing an application at a collector has expressed the intent
// plainly enough.
//
// This cannot switch telemetry on by itself. The instrumentation still has to
// be compiled in, which takes a deliberate import, so an application that never
// wants the OpenTelemetry SDK never carries it whatever the environment says.
func telemetryRequested(cfg *config.Config) bool {
	// OTEL_SDK_DISABLED is OpenTelemetry's own off switch, and it is the way out
	// for a service that has an endpoint in its environment and still wants
	// nothing to do with it — the one case the endpoint rule would otherwise get
	// wrong. Checked first so it beats both of the ways of asking.
	if disabled, err := strconv.ParseBool(os.Getenv("OTEL_SDK_DISABLED")); err == nil && disabled {
		return false
	}

	if cfg != nil && cfg.Ginboot.Telemetry.Enabled {
		return true
	}

	return otlpEndpointConfigured()
}

// otlpEndpointConfigured reports whether the environment names somewhere to send
// telemetry. The signal-specific variables count: an application exporting only
// traces sets one of those and no general endpoint at all.
func otlpEndpointConfigured() bool {
	for _, key := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
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
