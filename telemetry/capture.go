package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// errorDetailKey carries a failed request's message from the capture middleware
// to the logging middleware, which runs outside it and owns the log record.
const errorDetailKey = "ginboot.error_detail"

// defaultMaxCaptureBytes bounds what one payload contributes to a span. Traces
// are billed by ingested volume and a span is not a place to store a document, so
// an oversized body is reported as truncated rather than sent in full.
const defaultMaxCaptureBytes = 4096

// CaptureOptions configures what a request records beyond its method, route and
// status.
//
// Recovering the message from a failed response is always on: it costs one small
// attribute on requests that are already exceptional, and without it an error
// arrives in a log backend as a bare status code. Recording the payloads of
// successful requests is opt-in, because it is the setting that multiplies trace
// volume and the one that can carry personal data.
type CaptureOptions struct {
	// RequestBodies records the request payload of every captured request.
	RequestBodies bool
	// ResponseBodies records the response payload of every captured request.
	ResponseBodies bool
	// MaxBytes caps a single recorded payload. Zero selects the default.
	MaxBytes int
	// SkipPaths excludes requests whose path contains one of these fragments.
	// Routes carrying credentials or bulk data belong here.
	SkipPaths []string
	// RedactKeys replaces the value of any JSON field whose name contains one of
	// these, compared case-insensitively.
	RedactKeys []string
}

// DefaultCaptureOptions returns the options an instrumented server uses.
//
// The body settings are read from the environment so an operator can turn
// payload capture on for a deployment without a code change, which is what makes
// it usable as a debugging switch on a platform.
func DefaultCaptureOptions() CaptureOptions {
	return CaptureOptions{
		RequestBodies:  envBool("GINBOOT_CAPTURE_REQUEST_BODIES"),
		ResponseBodies: envBool("GINBOOT_CAPTURE_RESPONSE_BODIES"),
		MaxBytes:       envInt("GINBOOT_CAPTURE_MAX_BYTES", defaultMaxCaptureBytes),
		SkipPaths:      []string{"/health", "/healthz", "/metrics", "/logs", "/traces"},
		RedactKeys: []string{
			"password", "passphrase", "secret", "token", "key",
			"credential", "authorization", "private",
		},
	}
}

func envBool(name string) bool {
	value, err := strconv.ParseBool(os.Getenv(name))
	return err == nil && value
}

func envInt(name string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(name)); err == nil && value > 0 {
		return value
	}
	return fallback
}

func (o CaptureOptions) maxBytes() int {
	if o.MaxBytes > 0 {
		return o.MaxBytes
	}
	return defaultMaxCaptureBytes
}

func (o CaptureOptions) skips(path string) bool {
	for _, fragment := range o.SkipPaths {
		if fragment != "" && strings.Contains(path, fragment) {
			return true
		}
	}
	return false
}

// RequestDetailMiddleware records a request's payloads on its span and recovers
// the message from a failed response.
//
// A handler that reports its failure by writing a response — c.JSON with a
// status and a body, rather than returning an error — leaves gin's error list
// empty. Nothing downstream then knows what went wrong, so the span and the log
// record carry a status code and no explanation. The message is already in the
// response body, so it is read back from there and handed to gin, which makes
// every such handler observable without changing any of them.
//
// It must be registered after the tracing middleware, so a span is available, and
// after the logging middleware, so its own post-processing runs first and the log
// record can include what it found. Instrument does both.
func RequestDetailMiddleware(opts CaptureOptions) gin.HandlerFunc {
	limit := opts.maxBytes()

	return func(c *gin.Context) {
		if opts.skips(c.Request.URL.Path) {
			c.Next()
			return
		}

		var requestBody string
		if opts.RequestBodies {
			requestBody = captureRequestBody(c, opts, limit)
		}

		recorder := &responseRecorder{ResponseWriter: c.Writer, limit: limit}
		c.Writer = recorder

		c.Next()

		status := c.Writer.Status()
		responseBody := recorder.body.String()

		span := trace.SpanFromContext(c.Request.Context())
		if span.IsRecording() {
			if requestBody != "" {
				span.SetAttributes(attribute.String("http.request.body", requestBody))
			}
			// A failed response is recorded whether or not payload capture is on:
			// it is the explanation for the failure, not a payload.
			if responseBody != "" && (opts.ResponseBodies || status >= 400) {
				span.SetAttributes(attribute.String("http.response.body", redact(responseBody, opts, limit)))
			}
		}

		if status < 400 {
			return
		}
		detail := errorMessage(responseBody, limit)
		if detail == "" {
			return
		}

		// Both consumers: gin's error list is what the surrounding logging
		// middleware and any other instrumentation read, and the context value
		// carries the unformatted message for the log record.
		c.Set(errorDetailKey, detail)
		if len(c.Errors) == 0 {
			_ = c.Error(errors.New(detail))
		}
	}
}

// captureRequestBody reads the request payload and puts it back, so the handler
// that runs next still sees an unread body.
func captureRequestBody(c *gin.Context, opts CaptureOptions, limit int) string {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return ""
	}
	if !strings.Contains(c.ContentType(), "json") {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, int64(limit)+1))
	if err != nil {
		return ""
	}

	// What was read has been consumed and must be replaced. Anything past the cap
	// is streamed straight through, so a large upload still reaches the handler.
	original := c.Request.Body
	c.Request.Body = struct {
		io.Reader
		io.Closer
	}{Reader: io.MultiReader(bytes.NewReader(body), original), Closer: original}

	return redact(string(body), opts, limit)
}

// responseRecorder keeps a bounded copy of what a handler wrote while passing it
// through to the client unchanged.
type responseRecorder struct {
	gin.ResponseWriter
	body  bytes.Buffer
	limit int
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.record(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) WriteString(s string) (int, error) {
	r.record([]byte(s))
	return r.ResponseWriter.WriteString(s)
}

func (r *responseRecorder) record(b []byte) {
	if remaining := r.limit - r.body.Len(); remaining > 0 {
		if len(b) > remaining {
			b = b[:remaining]
		}
		r.body.Write(b)
	}
}

// errorMessage pulls the human-readable part out of an error response. Handlers
// are not consistent about which key they use, and a body that is not JSON is
// returned as-is so a failure from outside the application is still reported.
func errorMessage(body string, limit int) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return truncate(body, limit)
	}

	for _, key := range []string{"error", "message", "error_message", "detail"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil && text != "" {
			return truncate(text, limit)
		}
		if len(raw) > 0 {
			return truncate(string(raw), limit)
		}
	}
	return truncate(body, limit)
}

// redact replaces the values of sensitive fields in a JSON payload. A body that
// is not JSON is dropped when it mentions a sensitive field name, because there
// is no structure to tell which part of it is the credential.
func redact(body string, opts CaptureOptions, limit int) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		if opts.mentionsSensitiveKey(trimmed) {
			return "[redacted: unparsable body naming a sensitive field]"
		}
		return truncate(trimmed, limit)
	}

	encoded, err := json.Marshal(opts.redactValue(decoded))
	if err != nil {
		return ""
	}
	return truncate(string(encoded), limit)
}

func (o CaptureOptions) redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			if o.isSensitiveKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = o.redactValue(nested)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, nested := range typed {
			out[i] = o.redactValue(nested)
		}
		return out
	}
	return value
}

func (o CaptureOptions) isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, sensitive := range o.RedactKeys {
		if sensitive != "" && strings.Contains(lower, strings.ToLower(sensitive)) {
			return true
		}
	}
	return false
}

func (o CaptureOptions) mentionsSensitiveKey(body string) bool {
	lower := strings.ToLower(body)
	for _, sensitive := range o.RedactKeys {
		if sensitive != "" && strings.Contains(lower, strings.ToLower(sensitive)) {
			return true
		}
	}
	return false
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…[truncated]"
}
