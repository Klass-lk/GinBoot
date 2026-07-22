package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func generateReqID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("req_%s", hex.EncodeToString(b))
}

// RequestIDMiddleware attaches a unique X-Request-ID header to every request and response,
// and records it as an attribute on the active OpenTelemetry span.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = c.GetHeader("x-request-id")
		}

		span := trace.SpanFromContext(c.Request.Context())
		spanCtx := span.SpanContext()

		if reqID == "" && spanCtx.HasTraceID() {
			reqID = spanCtx.TraceID().String()
		}

		if reqID == "" {
			reqID = generateReqID()
		}

		// Set response header
		c.Writer.Header().Set("X-Request-ID", reqID)
		c.Set("request_id", reqID)

		// Record attribute on active span if recording
		if span.IsRecording() {
			span.SetAttributes(
				attribute.String("http.request_id", reqID),
				attribute.String("x-request-id", reqID),
			)
		}

		c.Next()
	}
}

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
		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}

		attrs := metric.WithAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.route", route),
			attribute.Int("http.status_code", status),
		)

		requestDuration.Record(c.Request.Context(), duration, attrs)
		requestCount.Add(c.Request.Context(), 1, attrs)
	}
}

// LoggingMiddleware logs requests using slog, automatically extracting trace_id, span_id, and request_id.
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

		reqID, _ := c.Get("request_id")
		reqIDStr, _ := reqID.(string)

		logger.Info("HTTP Request",
			slog.Int("status", status),
			slog.String("method", method),
			slog.String("path", path),
			slog.String("query", query),
			slog.Duration("latency", latency),
			slog.String("request_id", reqIDStr),
			slog.String("trace_id", traceID),
			slog.String("span_id", spanID),
		)
	}
}
