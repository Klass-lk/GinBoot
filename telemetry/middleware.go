package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
		if reqID == "" {
			reqID = c.GetHeader("apigw-requestid")
		}
		if reqID == "" {
			reqID = generateReqID()
		}

		// Set response header
		c.Writer.Header().Set("X-Request-ID", reqID)
		c.Set("request_id", reqID)

		// Record attribute on active span if recording
		span := trace.SpanFromContext(c.Request.Context())
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

// LoggingMiddleware logs requests beautifully to the console while sending structured log events to OpenTelemetry / slog.
func LoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

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
		clientIP := c.ClientIP()

		reqID, _ := c.Get("request_id")
		reqIDStr, _ := reqID.(string)

		fullPath := path
		if rawQuery != "" {
			fullPath = path + "?" + rawQuery
		}

		// Record error status on OpenTelemetry span if status >= 400
		if span.IsRecording() {
			if status >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d Error", status))
				if len(c.Errors) > 0 {
					span.RecordError(c.Errors.Last().Err)
					span.SetAttributes(attribute.String("error.message", c.Errors.String()))
				} else {
					span.SetAttributes(attribute.String("error.message", fmt.Sprintf("HTTP %d status code", status)))
				}
			} else {
				span.SetStatus(codes.Ok, "OK")
			}
		}

		// 1. Send structured log record to OpenTelemetry & slog logger
		// Filter out internal polling endpoints (/logs, /metrics, /traces, /health) to prevent log inflation loop
		isTelemetryPolling := strings.HasSuffix(path, "/logs") || strings.HasSuffix(path, "/metrics") || strings.HasSuffix(path, "/traces") || strings.HasSuffix(path, "/health") || strings.HasSuffix(path, "/healthz")
		if logger != nil && !isTelemetryPolling {
			logLevel := slog.LevelInfo
			if status >= 500 {
				logLevel = slog.LevelError
			} else if status >= 400 {
				logLevel = slog.LevelWarn
			}

			logger.Log(c.Request.Context(), logLevel, "HTTP Request",
				slog.Int("status", status),
				slog.String("method", method),
				slog.String("path", fullPath),
				slog.Duration("latency", latency),
				slog.String("client_ip", clientIP),
				slog.String("request_id", reqIDStr),
				slog.String("trace_id", traceID),
				slog.String("span_id", spanID),
			)
		}

		// 2. Format a beautiful colored GINBOOT request log for the console
		statusColor := getStatusColor(status)
		methodColor := getMethodColor(method)
		resetColor := "\033[0m"

		traceInfo := ""
		if traceID != "" {
			traceInfo = fmt.Sprintf(" | trace_id=%s", traceID)
		}

		fmt.Fprintf(gin.DefaultWriter, "[GINBOOT] %s |%s %3d %s| %13v | %15s |%s %-7s %s %s%s\n",
			start.Format("2006/01/02 - 15:04:05"),
			statusColor, status, resetColor,
			latency,
			clientIP,
			methodColor, method, resetColor,
			fullPath,
			traceInfo,
		)
	}
}

func getStatusColor(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "\033[97;42m" // White text on Green background
	case code >= 300 && code < 400:
		return "\033[97;43m" // White text on Yellow background
	case code >= 400 && code < 500:
		return "\033[97;43m" // White text on Yellow background
	default:
		return "\033[97;41m" // White text on Red background
	}
}

func getMethodColor(method string) string {
	switch method {
	case "GET":
		return "\033[97;44m" // White text on Blue background
	case "POST":
		return "\033[97;42m" // White text on Green background
	case "PUT":
		return "\033[97;43m" // White text on Yellow background
	case "DELETE":
		return "\033[97;41m" // White text on Red background
	case "PATCH":
		return "\033[97;45m" // White text on Magenta background
	default:
		return "\033[97;46m" // White text on Cyan background
	}
}
