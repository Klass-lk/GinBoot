package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/klass-lk/ginboot"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/runtime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Setup initializes the OpenTelemetry SDK with OTLP exporters.
// It returns a shutdown function that should be called when the application exits.
func Setup(ctx context.Context, serviceName, version string) (func(context.Context) error, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"", // Empty schema URL prevents conflicts with resource.Default()
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Set up propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Enable debug logging for OpenTelemetry to catch export errors
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		fmt.Printf("[OpenTelemetry Error] %v\n", err)
	}))

	// Determine if OTLP endpoint is configured in environment
	hasOTLPConfig := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") != ""

	isLambda := os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""

	// Use a 1-second timeout context for OTLP exporter initialization to avoid blocking Lambda INIT
	exporterCtx, cancelExporterCtx := context.WithTimeout(ctx, 1*time.Second)
	defer cancelExporterCtx()

	// Set up trace provider
	var tracerProvider *trace.TracerProvider
	if hasOTLPConfig {
		traceExporter, err := otlptracehttp.New(exporterCtx, otlptracehttp.WithTimeout(500*time.Millisecond))
		if err != nil {
			fmt.Printf("[Telemetry Warning] Failed to create trace exporter: %v. Falling back to default provider.\n", err)
			tracerProvider = trace.NewTracerProvider(trace.WithResource(res))
		} else {
			var spanProcessor trace.SpanProcessor
			if isLambda {
				spanProcessor = trace.NewSimpleSpanProcessor(traceExporter)
			} else {
				spanProcessor = trace.NewBatchSpanProcessor(traceExporter)
			}
			tracerProvider = trace.NewTracerProvider(
				trace.WithSampler(trace.AlwaysSample()),
				trace.WithResource(res),
				trace.WithSpanProcessor(spanProcessor),
			)
		}
	} else {
		tracerProvider = trace.NewTracerProvider(
			trace.WithResource(res),
		)
	}
	otel.SetTracerProvider(tracerProvider)

	// Set up metric provider
	var meterProvider *metric.MeterProvider
	if hasOTLPConfig {
		metricExporter, err := otlpmetrichttp.New(exporterCtx, otlpmetrichttp.WithTimeout(500*time.Millisecond))
		if err != nil {
			fmt.Printf("[Telemetry Warning] Failed to create metric exporter: %v. Falling back to default provider.\n", err)
			meterProvider = metric.NewMeterProvider(metric.WithResource(res))
		} else {
			meterProvider = metric.NewMeterProvider(
				metric.WithResource(res),
				metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(15*time.Second))),
			)
		}
	} else {
		meterProvider = metric.NewMeterProvider(
			metric.WithResource(res),
		)
	}
	otel.SetMeterProvider(meterProvider)

	// Set up log provider
	var loggerProvider *log.LoggerProvider
	if hasOTLPConfig {
		logExporter, err := otlploghttp.New(exporterCtx, otlploghttp.WithTimeout(500*time.Millisecond))
		if err != nil {
			fmt.Printf("[Telemetry Warning] Failed to create log exporter: %v. Falling back to default provider.\n", err)
			loggerProvider = log.NewLoggerProvider(log.WithResource(res))
		} else {
			var logProcessor log.Processor
			if isLambda {
				logProcessor = log.NewSimpleProcessor(logExporter)
			} else {
				logProcessor = log.NewBatchProcessor(logExporter)
			}
			loggerProvider = log.NewLoggerProvider(
				log.WithResource(res),
				log.WithProcessor(logProcessor),
			)
		}
		}
	} else {
		loggerProvider = log.NewLoggerProvider(
			log.WithResource(res),
		)
	}
	global.SetLoggerProvider(loggerProvider)

	// Start collecting Go runtime metrics
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second * 15)); err != nil {
		fmt.Printf("[OpenTelemetry Error] failed to start runtime metrics: %v\n", err)
	}

	// Return a shutdown function
	return func(shutdownCtx context.Context) error {
		var errs []error
		if tracerProvider != nil {
			if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
				errs = append(errs, fmt.Errorf("failed to shutdown tracer provider: %w", err))
			}
		}
		if meterProvider != nil {
			if err := meterProvider.Shutdown(shutdownCtx); err != nil {
				errs = append(errs, fmt.Errorf("failed to shutdown meter provider: %w", err))
			}
		}
		if loggerProvider != nil {
			if err := loggerProvider.Shutdown(shutdownCtx); err != nil {
				errs = append(errs, fmt.Errorf("failed to shutdown logger provider: %w", err))
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("errors during shutdown: %v", errs)
		}
		return nil
	}, nil
}

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// Instrument enables OpenTelemetry tracing, metrics, request IDs, and structured logging for the server.
func Instrument(s *ginboot.Server, serviceName string, logger *slog.Logger) {
	// Add Request ID middleware for X-Request-ID headers & OTel correlation
	s.Engine().Use(RequestIDMiddleware())

	// Add otelgin middleware for tracing
	s.Engine().Use(otelgin.Middleware(serviceName))

	// Add custom metrics middleware
	s.Engine().Use(MetricsMiddleware(nil))

	// Add logging middleware that extracts trace IDs and request IDs
	if logger == nil {
		// Log to both console and OpenTelemetry
		consoleHandler := slog.NewTextHandler(os.Stdout, nil)
		otelHandler := otelslog.NewHandler(serviceName)
		logger = slog.New(&multiHandler{handlers: []slog.Handler{consoleHandler, otelHandler}})
		slog.SetDefault(logger)
	}
	s.SetLogger(ginboot.NewSlogLogger(logger))
	s.Engine().Use(LoggingMiddleware(logger))
}
