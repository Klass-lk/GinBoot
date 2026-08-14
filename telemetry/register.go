package telemetry

import (
	"context"
	"os"
	"strings"

	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/config"
)

// Importing this package registers it as ginboot's instrumentation, so that
// ginboot.yml's telemetry block can switch it on:
//
//	import _ "github.com/klass-lk/ginboot/telemetry"
//
// ginboot.New then reads telemetry.enabled and calls the function below. See
// Instrumenter in the ginboot package for why the wiring runs in this
// direction. Calling Setup and Instrument directly still works and still is the
// way to control what each request records — see InstrumentWithOptions.
func init() {
	ginboot.RegisterInstrumenter(setupFromConfig)

	// Telemetry is batched, so something has to drain it on a runtime that gets
	// suspended between requests rather than running continuously. Registering
	// the drain here lets runtime/lambda arrange it without importing the
	// OpenTelemetry SDK — see Flush, and ginboot.RegisterFlusher.
	ginboot.RegisterFlusher(Flush)
}

func setupFromConfig(ctx context.Context, s *ginboot.Server, cfg config.TelemetryConfig) (func(context.Context) error, error) {
	// Refuse to install the middleware a second time rather than double every
	// span and log line, which is what an application that also calls Instrument
	// itself would otherwise get.
	if !s.ClaimInstrumentation() {
		return nil, nil
	}

	applyConfigToEnv(cfg)

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = os.Getenv("OTEL_SERVICE_NAME")
	}
	if serviceName == "" {
		serviceName = "ginboot-app"
	}

	version := cfg.ServiceVersion
	if version == "" {
		version = "v0.0.0"
	}

	shutdown, err := Setup(ctx, serviceName, version)
	if err != nil {
		return nil, err
	}

	instrument(s, serviceName, nil, DefaultCaptureOptions())
	return shutdown, nil
}

// applyConfigToEnv publishes the configured values as the environment variables
// the OpenTelemetry SDK reads, which is the only way it accepts settings.
//
// A value already in the environment is left alone. The deployment's own
// settings are what the environment carries — the platform running the
// application injects them — and they have to outrank a file committed to the
// repository, or an application could not be pointed at a different collector
// without a rebuild. config.LoadConfig applies the same precedence when it
// populates cfg, so in practice this writes only what the file alone supplied.
func applyConfigToEnv(cfg config.TelemetryConfig) {
	setIfAbsent("OTEL_SERVICE_NAME", cfg.ServiceName)
	setIfAbsent("OTEL_EXPORTER_OTLP_ENDPOINT", cfg.Endpoint)
	setIfAbsent("OTEL_EXPORTER_OTLP_HEADERS", cfg.Headers)
	setIfAbsent("OTEL_EXPORTER_OTLP_PROTOCOL", cfg.Protocol)

	attributes := cfg.ResourceAttributes
	if cfg.Environment != "" && !strings.Contains(attributes, "deployment.environment") {
		if attributes != "" {
			attributes += ","
		}
		attributes += "deployment.environment=" + cfg.Environment
	}
	setIfAbsent("OTEL_RESOURCE_ATTRIBUTES", attributes)
}

func setIfAbsent(key, value string) {
	if value == "" {
		return
	}
	if _, ok := os.LookupEnv(key); ok {
		return
	}
	_ = os.Setenv(key, value)
}
