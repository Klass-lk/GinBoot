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
	// Requests currently being served. Only the method is attached: this is a
	// gauge sampled on every export, so adding the route would multiply the
	// series count by the size of the API surface.
	activeRequests, _ := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("Number of in-flight HTTP server requests."),
	)

	return func(c *gin.Context) {
		start := time.Now()

		inFlight := metric.WithAttributes(attribute.String("http.method", c.Request.Method))
		activeRequests.Add(c.Request.Context(), 1, inFlight)
		defer activeRequests.Add(c.Request.Context(), -1, inFlight)

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

		// The message explaining a failure, when anything managed to supply one.
		// RequestDetailMiddleware recovers it from the response body for handlers
		// that report failures by writing a response rather than returning an
		// error, which is otherwise invisible from here.
		errorDetail, _ := c.Value(errorDetailKey).(string)
		if errorDetail == "" && len(c.Errors) > 0 {
			errorDetail = c.Errors.Last().Err.Error()
		}

		// Record error status on OpenTelemetry span if status >= 400
		if span.IsRecording() {
			if status >= 400 {
				// Prefer the real message: a description of "HTTP 500 Error"
				// restates the status code and explains nothing.
				description := errorDetail
				if description == "" {
					description = fmt.Sprintf("HTTP %d Error", status)
				}
				span.SetStatus(codes.Error, description)

				if len(c.Errors) > 0 {
					span.RecordError(c.Errors.Last().Err)
				}
				if errorDetail != "" {
					span.SetAttributes(attribute.String("error.message", errorDetail))
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

			// The message summarises the request rather than repeating the
			// constant string "HTTP Request". A log backend renders the body
			// prominently and the attributes as metadata, so a message that is
			// identical on every line forces the reader to expand each record to
			// learn anything. The attributes are still emitted unchanged for
			// querying and aggregation.
			message := fmt.Sprintf("%s %s %d %s", method, fullPath, status, formatLatency(latency))

			attrs := []any{
				slog.Int("status", status),
				slog.String("method", method),
				slog.String("path", fullPath),
				slog.Duration("latency", latency),
				slog.String("client_ip", clientIP),
				slog.String("request_id", reqIDStr),
				slog.String("trace_id", traceID),
				slog.String("span_id", spanID),
			}

			// Without this the record for a failed request carries only its status,
			// which is what made an error in a log backend unreadable: every line
			// looked the same and none said what went wrong. It is appended to the
			// message too, because a log viewer shows the body first.
			if errorDetail != "" {
				message += " — " + errorDetail
				attrs = append(attrs, slog.String("error", errorDetail))
			}

			logger.Log(c.Request.Context(), logLevel, message, attrs...)
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

// formatLatency renders a duration at a readable scale for a log message.
// time.Duration's own formatting produces values like "1.007531416s", where the
// trailing precision is noise when scanning a log.
func formatLatency(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d >= time.Microsecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	default:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
}
