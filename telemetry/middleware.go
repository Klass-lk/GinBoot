package telemetry

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// MetricsMiddleware records standard HTTP metrics.
func MetricsMiddleware(meter metric.Meter) gin.HandlerFunc {
	if meter == nil {
		meter = otel.GetMeterProvider().Meter("github.com/klass-lk/ginboot")
	}

	requestDuration, _ := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("Duration of HTTP server requests."),
		metric.WithUnit("s"),
	)
	requestCount, _ := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("Number of HTTP server requests."),
	)

	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := c.Writer.Status()

		// Optional: add tags for route, method, status
		// Here we keep it simple for illustration.
		requestDuration.Record(c.Request.Context(), duration)
		requestCount.Add(c.Request.Context(), 1)

		_ = status // to avoid unused variable if we don't add attributes
	}
}

// LoggingMiddleware logs requests using slog, automatically extracting trace_id and span_id.
func LoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Extract trace and span ID before c.Next() to ensure it's available
		span := trace.SpanFromContext(c.Request.Context())
		spanCtx := span.SpanContext()

		var traceID, spanID string
		if spanCtx.HasTraceID() {
			traceID = spanCtx.TraceID().String()
		}
		if spanCtx.HasSpanID() {
			spanID = spanCtx.SpanID().String()
		}

		// Process request
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method

		logger.Info("HTTP Request",
			slog.Int("status", status),
			slog.String("method", method),
			slog.String("path", path),
			slog.String("query", query),
			slog.Duration("latency", latency),
			slog.String("trace_id", traceID),
			slog.String("span_id", spanID),
		)
	}
}
