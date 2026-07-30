package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// recordingHandler keeps the log records a test produces, so the attributes of
// the request log can be asserted rather than inspected by eye.
type recordingHandler struct {
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) find(t *testing.T, substring string) slog.Record {
	t.Helper()
	for _, record := range h.records {
		if strings.Contains(record.Message, substring) {
			return record
		}
	}
	t.Fatalf("no log record whose message contains %q; got %d records", substring, len(h.records))
	return slog.Record{}
}

func recordAttrs(r slog.Record) map[string]string {
	attrs := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	return attrs
}

// captureRouter wires the middleware the way Instrument does: tracing outermost,
// logging next, request detail innermost.
func captureRouter(t *testing.T, opts CaptureOptions, method, path string, handler gin.HandlerFunc) (*gin.Engine, *tracetest.SpanRecorder, *recordingHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	spans := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	tracer := provider.Tracer("test")

	logs := &recordingHandler{}
	logger := slog.New(logs)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx, span := tracer.Start(c.Request.Context(), "server")
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.Use(LoggingMiddleware(logger))
	router.Use(RequestDetailMiddleware(opts))
	router.Handle(method, path, handler)

	return router, spans, logs
}

func spanAttrs(t *testing.T, recorder *tracetest.SpanRecorder) map[string]string {
	t.Helper()
	ended := recorder.Ended()
	if len(ended) == 0 {
		t.Fatal("no span was recorded")
	}
	attrs := map[string]string{}
	for _, attr := range ended[len(ended)-1].Attributes() {
		attrs[string(attr.Key)] = attr.Value.Emit()
	}
	return attrs
}

// TestRequestDetailRecoversErrorMessage is the fix for an error that arrived as a
// bare status code. The handler reports its failure by writing a response, which
// is the shape that left gin's error list empty and the log record uninformative.
func TestRequestDetailRecoversErrorMessage(t *testing.T) {
	router, spans, logs := captureRouter(t, DefaultCaptureOptions(), http.MethodGet, "/commit",
		func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to init github: bad key"})
		})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/commit", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "failed to init github") {
		t.Errorf("the response sent to the client was altered: %s", response.Body.String())
	}

	attrs := spanAttrs(t, spans)
	if attrs["error.message"] != "failed to init github: bad key" {
		t.Errorf("error.message = %q, want the handler's message", attrs["error.message"])
	}

	ended := spans.Ended()
	last := ended[len(ended)-1]
	if last.Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", last.Status().Code)
	}
	// Previously the description restated the status code and explained nothing.
	if last.Status().Description != "failed to init github: bad key" {
		t.Errorf("span status description = %q", last.Status().Description)
	}

	// The log record is the half that was missing: the message never reached it.
	record := logs.find(t, "failed to init github")
	if got := recordAttrs(record)["error"]; got != "failed to init github: bad key" {
		t.Errorf("log record error attribute = %q", got)
	}
	if record.Level != slog.LevelError {
		t.Errorf("log level = %v, want Error for a 500", record.Level)
	}
}

// TestRequestDetailLogsOneRecordPerRequest guards against the failure log being
// duplicated, which is what happens if the message is emitted separately from the
// request record instead of being folded into it.
func TestRequestDetailLogsOneRecordPerRequest(t *testing.T) {
	router, _, logs := captureRouter(t, DefaultCaptureOptions(), http.MethodGet, "/commit",
		func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
		})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/commit", nil))

	if len(logs.records) != 1 {
		t.Errorf("expected 1 log record for the request, got %d", len(logs.records))
	}
}

// TestRequestDetailBodiesAreOptIn keeps payload capture off by default: it is the
// setting that multiplies trace volume and can carry personal data.
func TestRequestDetailBodiesAreOptIn(t *testing.T) {
	payload := `{"branch":"main"}`

	off := DefaultCaptureOptions()
	off.RequestBodies = false
	off.ResponseBodies = false
	router, spans, _ := captureRouter(t, off, http.MethodPost, "/commit",
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	request := httptest.NewRequest(http.MethodPost, "/commit", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	attrs := spanAttrs(t, spans)
	if _, ok := attrs["http.request.body"]; ok {
		t.Error("request body was recorded while capture was off")
	}
	if _, ok := attrs["http.response.body"]; ok {
		t.Error("response body was recorded while capture was off")
	}

	on := DefaultCaptureOptions()
	on.RequestBodies = true
	on.ResponseBodies = true
	router, spans, _ = captureRouter(t, on, http.MethodPost, "/commit",
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	request = httptest.NewRequest(http.MethodPost, "/commit", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	attrs = spanAttrs(t, spans)
	if !strings.Contains(attrs["http.request.body"], "main") {
		t.Errorf("request body = %q, want it recorded when capture is on", attrs["http.request.body"])
	}
	if !strings.Contains(attrs["http.response.body"], "ok") {
		t.Errorf("response body = %q, want it recorded when capture is on", attrs["http.response.body"])
	}
}

// TestRequestDetailRecordsFailedResponseRegardless: a failed response is the
// explanation for the failure, not a payload, so it is recorded even with capture
// off. This is what makes an error readable without turning capture on globally.
func TestRequestDetailRecordsFailedResponseRegardless(t *testing.T) {
	opts := DefaultCaptureOptions()
	opts.ResponseBodies = false

	router, spans, _ := captureRouter(t, opts, http.MethodGet, "/commit",
		func(c *gin.Context) { c.JSON(http.StatusBadRequest, gin.H{"error": "invalid branch"}) })

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/commit", nil))

	if body := spanAttrs(t, spans)["http.response.body"]; !strings.Contains(body, "invalid branch") {
		t.Errorf("failed response body = %q, want it recorded", body)
	}
}

// TestRequestDetailLeavesRequestBodyReadable guards against the classic failure
// of consuming the payload while recording it.
func TestRequestDetailLeavesRequestBodyReadable(t *testing.T) {
	opts := DefaultCaptureOptions()
	opts.RequestBodies = true

	var seen string
	router, _, _ := captureRouter(t, opts, http.MethodPost, "/commit", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		seen = string(body)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	payload := `{"branch":"main","message":"ship it"}`
	request := httptest.NewRequest(http.MethodPost, "/commit", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	if seen != payload {
		t.Fatalf("handler read %q, want the untouched payload", seen)
	}
}

// TestRequestDetailRedactsSecrets matters because a payload can carry deployment
// credentials and a span is retained and widely readable.
func TestRequestDetailRedactsSecrets(t *testing.T) {
	opts := DefaultCaptureOptions()
	opts.RequestBodies = true

	router, spans, _ := captureRouter(t, opts, http.MethodPost, "/commit",
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	payload := `{"name":"prod","githubToken":"ghp_realsecret","nested":{"password":"hunter2"},"items":[{"apiKey":"sk-live"}]}`
	request := httptest.NewRequest(http.MethodPost, "/commit", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	recorded := spanAttrs(t, spans)["http.request.body"]
	for _, secret := range []string{"ghp_realsecret", "hunter2", "sk-live"} {
		if strings.Contains(recorded, secret) {
			t.Errorf("recorded body leaked %q: %s", secret, recorded)
		}
	}
	if !strings.Contains(recorded, "prod") {
		t.Errorf("redaction removed non-sensitive fields too: %s", recorded)
	}

	// The shape must survive redaction, or the recorded body is unreadable.
	var decoded map[string]any
	if err := json.Unmarshal([]byte(recorded), &decoded); err != nil {
		t.Errorf("redacted body is not valid JSON: %v", err)
	}
}

// TestRequestDetailSkipsConfiguredPaths lets an operator exclude the routes whose
// payloads are credentials outright, rather than relying on field names.
func TestRequestDetailSkipsConfiguredPaths(t *testing.T) {
	opts := DefaultCaptureOptions()
	opts.RequestBodies = true
	opts.SkipPaths = []string{"/vars"}

	router, spans, _ := captureRouter(t, opts, http.MethodPut, "/apps/3/vars",
		func(c *gin.Context) { c.JSON(http.StatusInternalServerError, gin.H{"error": "nope"}) })

	request := httptest.NewRequest(http.MethodPut, "/apps/3/vars", strings.NewReader(`{"DATABASE_URL":"postgres://u:pw@h/db"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	attrs := spanAttrs(t, spans)
	for _, key := range []string{"http.request.body", "http.response.body"} {
		if _, ok := attrs[key]; ok {
			t.Errorf("%s was recorded for a skipped path: %v", key, attrs)
		}
	}
}

// TestRequestDetailTruncatesLargePayloads keeps one request from dominating the
// trace bill.
func TestRequestDetailTruncatesLargePayloads(t *testing.T) {
	opts := DefaultCaptureOptions()
	opts.ResponseBodies = true
	opts.MaxBytes = 64

	router, spans, _ := captureRouter(t, opts, http.MethodGet, "/commit",
		func(c *gin.Context) { c.String(http.StatusOK, strings.Repeat("x", 4096)) })

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/commit", nil))

	recorded := spanAttrs(t, spans)["http.response.body"]
	if len(recorded) > 128 {
		t.Errorf("recorded body is %d bytes, want it bounded by MaxBytes", len(recorded))
	}
}

// TestRequestDetailIgnoresSuccessfulResponses keeps the log free of noise for
// requests that worked.
func TestRequestDetailIgnoresSuccessfulResponses(t *testing.T) {
	router, spans, logs := captureRouter(t, DefaultCaptureOptions(), http.MethodGet, "/commit",
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/commit", nil))

	if attrs := spanAttrs(t, spans); attrs["error.message"] != "" {
		t.Errorf("a successful request recorded an error: %v", attrs)
	}
	for _, record := range logs.records {
		if record.Level >= slog.LevelWarn {
			t.Errorf("a successful request logged at %v: %s", record.Level, record.Message)
		}
	}
}

// TestErrorMessagePicksTheReadablePart covers the keys the ecosystem's handlers
// actually use.
func TestErrorMessagePicksTheReadablePart(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"error":"application not found"}`, "application not found"},
		{`{"message":"access denied"}`, "access denied"},
		{`{"error_code":"404","message":"missing"}`, "missing"},
		{`plain text failure`, "plain text failure"},
		{``, ""},
		{`{"ok":true}`, `{"ok":true}`},
	}

	for _, tc := range cases {
		if got := errorMessage(tc.body, defaultMaxCaptureBytes); got != tc.want {
			t.Errorf("errorMessage(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}
