package ginboot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func serverWithConsumer(t *testing.T, c Consumer) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := New()
	s.RegisterConsumer(c)
	return s
}

func TestTriggerManifestIsServed(t *testing.T) {
	s := serverWithConsumer(t, NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error { return nil }))
	s.registerTriggersEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodGet, defaultTriggersPath, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("the manifest answered %d", w.Code)
	}

	var manifest TriggerManifest
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("the manifest is not readable JSON: %v", err)
	}
	if len(manifest.Triggers) != 1 || manifest.Triggers[0].Name != "sms" {
		t.Fatalf("the manifest described %+v", manifest.Triggers)
	}
}

// The control plane reads both manifests with one token. A configuration that
// protected one and published the other would be a setting nobody chose.
func TestBothManifestsShareOneAccessSetting(t *testing.T) {
	t.Setenv(workersAccessEnv, WorkersToken)
	t.Setenv(workersTokenEnv, "sekrit")

	s := serverWithConsumer(t, NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error { return nil }))
	s.registerWorkersEndpoint()
	s.registerTriggersEndpoint()

	for _, path := range []string{defaultWorkersPath, defaultTriggersPath} {
		w := httptest.NewRecorder()
		s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s answered %d without a token", path, w.Code)
		}

		w = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(WorkersTokenHeader, "sekrit")
		s.Engine().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s answered %d with the right token", path, w.Code)
		}
	}
}

func TestDisabledAccessServesNoTriggerManifest(t *testing.T) {
	t.Setenv(workersAccessEnv, WorkersDisabled)

	s := serverWithConsumer(t, NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error { return nil }))
	s.registerTriggersEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodGet, defaultTriggersPath, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("a disabled manifest answered %d; the path should be a 404 like any other", w.Code)
	}
}

// The delivery endpoint invokes application code with a caller-supplied
// payload. It must not exist outside a developer's own build.
func TestDeliveryEndpointIsAbsentOutsideDebugMode(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	defer gin.SetMode(gin.TestMode)

	s := New()
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error {
			t.Error("a consumer was invoked over HTTP in release mode")
			return nil
		}))
	s.registerTriggerDeliveryEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		defaultTriggersPath+"/sms", strings.NewReader(`{}`)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("the delivery endpoint answered %d in release mode", w.Code)
	}
}

func TestDeliveryEndpointRunsTheConsumer(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	type message struct {
		Text string `json:"text"`
	}
	var got message

	s := New()
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m message) error {
			got = m
			return nil
		}))
	s.registerTriggerDeliveryEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		defaultTriggersPath+"/sms", strings.NewReader(`{"text":"hello"}`)))

	if w.Code != http.StatusOK {
		t.Fatalf("delivery answered %d: %s", w.Code, w.Body.String())
	}
	if got.Text != "hello" {
		t.Errorf("the consumer received %+v", got)
	}
}

// A typo in the name is the mistake this endpoint produces most often, and a
// bare 404 sends someone to check their registration code instead of their URL.
func TestDeliveryEndpointNamesRegisteredConsumers(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	s := New()
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error { return nil }))
	s.registerTriggerDeliveryEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		defaultTriggersPath+"/smss", strings.NewReader(`{}`)))

	if w.Code != http.StatusNotFound {
		t.Fatalf("an unknown consumer answered %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "sms") {
		t.Errorf("the error did not name the consumers that do exist: %s", w.Body.String())
	}
}

// A failing handler reports its own error. In production the record would be
// redelivered; locally the message is the whole point.
func TestDeliveryEndpointReportsHandlerFailure(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	s := New()
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error {
			return context.DeadlineExceeded
		}))
	s.registerTriggerDeliveryEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		defaultTriggersPath+"/sms", strings.NewReader(`{}`)))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a failing handler answered %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), context.DeadlineExceeded.Error()) {
		t.Errorf("the handler's own error was not reported: %s", w.Body.String())
	}
}

// A source that is neither managed nor a queue has nothing this check can say
// about it — it must not be reported as a problem.
func TestUndeployableSaysNothingAboutOtherExternalSources(t *testing.T) {
	topic := &staticConsumer{
		name:   "alerts",
		source: EventSource{Kind: SourceTopic, Ref: "arn:aws:sns:ap-southeast-1:123456789012:alerts"},
	}
	if problem := undeployable(topic); problem != "" {
		t.Fatalf("an external topic was reported undeployable: %q", problem)
	}
}

// The manifest reports which runtime it is describing, because a schedule's
// resolution depends on it.
func TestManifestReportsTheTickRuntime(t *testing.T) {
	t.Setenv("AWS_LAMBDA_RUNTIME_API", "127.0.0.1:9001")

	r := NewConsumerRegistry(NewSlogLogger(nil))
	if got := r.Manifest().Runtime; got != "tick" {
		t.Fatalf("runtime = %q on a Lambda runtime, want \"tick\"", got)
	}
}

// A server with no registry still answers, rather than panicking on a nil
// dereference inside a request.
func TestManifestEndpointHandlesANilRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New()
	s.consumers = nil
	s.registerTriggersEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodGet, defaultTriggersPath, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("a nil registry answered %d", w.Code)
	}
	var manifest TriggerManifest
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("the answer is not readable JSON: %v", err)
	}
	if len(manifest.Triggers) != 0 {
		t.Errorf("a nil registry described %d triggers", len(manifest.Triggers))
	}
}

func TestDeliveryEndpointHandlesANilRegistry(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	s := New()
	s.consumers = nil
	s.registerTriggerDeliveryEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		defaultTriggersPath+"/sms", strings.NewReader(`{}`)))

	if w.Code != http.StatusNotFound {
		t.Fatalf("a nil registry answered %d", w.Code)
	}
}

// Turning the manifests off turns delivery off with them. It is the same
// setting, and a delivery endpoint left mounted when the manifest was withheld
// would be the more dangerous half of the pair still open.
func TestDisabledAccessMountsNoDeliveryEndpoint(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)
	t.Setenv(workersAccessEnv, WorkersDisabled)

	s := New()
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error {
			t.Error("a consumer ran through a disabled delivery endpoint")
			return nil
		}))
	s.registerTriggerDeliveryEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		defaultTriggersPath+"/sms", strings.NewReader(`{}`)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("a disabled delivery endpoint answered %d", w.Code)
	}
}

// Delivery is gated by the same token as the manifests. Without it, anyone who
// can reach the process can run its consumers.
func TestDeliveryEndpointRequiresTheToken(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)
	t.Setenv(workersAccessEnv, WorkersToken)
	t.Setenv(workersTokenEnv, "sekrit")

	ran := false
	s := New()
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error {
			ran = true
			return nil
		}))
	s.registerTriggerDeliveryEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		defaultTriggersPath+"/sms", strings.NewReader(`{}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("delivery without a token answered %d", w.Code)
	}
	if ran {
		t.Error("the consumer ran for an unauthorized caller")
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, defaultTriggersPath+"/sms", strings.NewReader(`{}`))
	req.Header.Set(WorkersTokenHeader, "sekrit")
	s.Engine().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delivery with the right token answered %d: %s", w.Code, w.Body.String())
	}
	if !ran {
		t.Error("the consumer did not run for an authorized caller")
	}
}

// errReader fails partway through, the way a connection dropped mid-upload does.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestDeliveryEndpointReportsAnUnreadableBody(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	s := New()
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error {
			t.Error("the consumer ran on a body that could not be read")
			return nil
		}))
	s.registerTriggerDeliveryEndpoint()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, defaultTriggersPath+"/sms", errReader{})
	s.Engine().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("an unreadable body answered %d", w.Code)
	}
}

// staticConsumer is a Consumer with no behaviour, for the checks that only read
// its declaration.
type staticConsumer struct {
	name   string
	source EventSource
}

func (c *staticConsumer) Name() string        { return c.name }
func (c *staticConsumer) Source() EventSource { return c.source }
func (c *staticConsumer) Handle(context.Context, Event) error {
	return nil
}

// An access mode nobody recognises fails closed. Serving the manifest on the
// strength of a typo is the one outcome that must not happen.
func TestUnrecognisedAccessModeServesNothing(t *testing.T) {
	t.Setenv(workersAccessEnv, "pubic")

	s := serverWithConsumer(t, NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error { return nil }))
	s.registerWorkersEndpoint()
	s.registerTriggersEndpoint()

	for _, path := range []string{defaultWorkersPath, defaultTriggersPath} {
		w := httptest.NewRecorder()
		s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s answered %d under an unrecognised access mode", path, w.Code)
		}
	}
}

// A path configured without its leading slash is still mounted where the
// operator meant, rather than silently not at all.
func TestConfiguredPathWithoutALeadingSlashIsMounted(t *testing.T) {
	t.Setenv(triggersPathEnv, "internal/triggers")

	s := serverWithConsumer(t, NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error { return nil }))
	s.registerTriggersEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/internal/triggers", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("the configured path answered %d", w.Code)
	}
}

// The worker manifest's own nil-scheduler branch, which the shared refactor
// moved but did not change.
func TestWorkerManifestHandlesANilScheduler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New()
	s.scheduler = nil
	s.registerWorkersEndpoint()

	w := httptest.NewRecorder()
	s.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodGet, defaultWorkersPath, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("a nil scheduler answered %d", w.Code)
	}
	var manifest WorkersManifest
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("the answer is not readable JSON: %v", err)
	}
	if len(manifest.Workers) != 0 {
		t.Errorf("a nil scheduler described %d workers", len(manifest.Workers))
	}
}

func TestConsumersAccessorReturnsTheRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New()
	if s.Consumers() == nil {
		t.Fatal("a new server has no consumer registry")
	}
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("sms"),
		func(ctx context.Context, m struct{}) error { return nil }))
	if s.Consumers().Len() != 1 {
		t.Fatalf("the registry holds %d consumers after one registration", s.Consumers().Len())
	}
}

// A refused registration is logged and dropped rather than returned, so main
// stays a run of statements. What must not happen is the consumer being
// registered anyway.
func TestRegisterConsumerDropsARefusedRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New()

	valid := NewQueueConsumer("sms", Queue("sms"), func(ctx context.Context, m struct{}) error { return nil })
	s.RegisterConsumer(valid)
	// Same name, different queue.
	s.RegisterConsumer(NewQueueConsumer("sms", Queue("other"), func(ctx context.Context, m struct{}) error { return nil }))

	if s.Consumers().Len() != 1 {
		t.Fatalf("a duplicate registration was kept: %d consumers", s.Consumers().Len())
	}
	registered, _ := s.Consumers().ByName("sms")
	if registered.Source().Ref != "sms" {
		t.Errorf("the duplicate replaced the original: ref is %q", registered.Source().Ref)
	}

	if err := s.RegisterConsumerErr(nil); err == nil {
		t.Error("RegisterConsumerErr accepted a nil consumer")
	}
}
