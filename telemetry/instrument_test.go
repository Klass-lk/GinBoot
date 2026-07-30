package telemetry

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type instrumentedController struct {
	status  int
	payload gin.H
}

func (ic instrumentedController) Register(group *ginboot.ControllerGroup) {
	group.GET("/commit", func(ctx *ginboot.Context) (interface{}, error) {
		// The shape that hid errors: the handler writes its own response and
		// reports nothing back to the framework.
		ctx.Context.JSON(ic.status, ic.payload)
		return nil, nil
	})
}

func instrumented(t *testing.T, controller instrumentedController) (*ginboot.Server, *tracetest.SpanRecorder, *recordingHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	spans := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))

	logs := &recordingHandler{}

	server := ginboot.New()
	Instrument(server, "test-service", slog.New(logs))
	server.SetBasePath("/api/v1")
	server.RegisterController("apps", controller)

	return server, spans, logs
}

// TestInstrumentRecordsRequestIDOnSpan pins the middleware order.
//
// The request id middleware annotates the active span, so registering it before
// the tracing middleware meant it ran when no span existed yet and the attribute
// was silently dropped — leaving no way to find a trace from a request id.
func TestInstrumentRecordsRequestIDOnSpan(t *testing.T) {
	server, spans, _ := instrumented(t, instrumentedController{status: http.StatusOK, payload: gin.H{"ok": true}})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/apps/commit", nil)
	request.Header.Set("X-Request-ID", "req_from_caller")
	server.Engine().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	ended := spans.Ended()
	if len(ended) == 0 {
		t.Fatal("the tracing middleware recorded no span")
	}

	attrs := map[string]string{}
	for _, attr := range ended[len(ended)-1].Attributes() {
		attrs[string(attr.Key)] = attr.Value.Emit()
	}
	if attrs["http.request_id"] != "req_from_caller" {
		t.Errorf("http.request_id = %q, want the id recorded on the span", attrs["http.request_id"])
	}
}

// TestInstrumentReportsHandlerErrorDetail is the end-to-end assertion: a handler
// that writes its own failure ends up with the message on the span and in the log.
func TestInstrumentReportsHandlerErrorDetail(t *testing.T) {
	server, spans, logs := instrumented(t, instrumentedController{
		status:  http.StatusInternalServerError,
		payload: gin.H{"error": "failed to init github: bad key"},
	})

	response := httptest.NewRecorder()
	server.Engine().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/apps/commit", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}

	ended := spans.Ended()
	attrs := map[string]string{}
	for _, attr := range ended[len(ended)-1].Attributes() {
		attrs[string(attr.Key)] = attr.Value.Emit()
	}
	if !strings.Contains(attrs["error.message"], "failed to init github: bad key") {
		t.Errorf("error.message = %q", attrs["error.message"])
	}

	record := logs.find(t, "failed to init github")
	if got := recordAttrs(record)["error"]; !strings.Contains(got, "bad key") {
		t.Errorf("log error attribute = %q", got)
	}
}
