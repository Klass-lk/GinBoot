package ginboot

import (
	"context"
	"encoding/json"
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
