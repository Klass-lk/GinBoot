package ginboot

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type selfWritingController struct{}

func (selfWritingController) Register(group *ControllerGroup) {
	group.GET("/branches", func(ctx *Context) (interface{}, error) {
		ctx.Context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to init github"})
		return nil, nil
	})
}

// TestHandlerOwnResponseIsNotOverridden pins the fix for gin logging
// "Headers were already written. Wanted to override status code 500 with 200".
//
// A handler that delegates to a plain gin function writes the response itself and
// returns no value. The wrapper used to write a 200 status on top of it. Gin
// refuses the override, so the status the client saw was still correct — which is
// why this has to assert on gin's warning rather than on the response. The warning
// reads as a framework bug and buries the real status in noise.
//
// The warning is only emitted in debug mode, so the mode is set deliberately here
// rather than relying on the package default.
func TestHandlerOwnResponseIsNotOverridden(t *testing.T) {
	previousWriter := gin.DefaultWriter
	gin.SetMode(gin.DebugMode)
	var ginOutput bytes.Buffer
	gin.DefaultWriter = &ginOutput
	t.Cleanup(func() {
		gin.DefaultWriter = previousWriter
		gin.SetMode(gin.TestMode)
	})

	server := New()
	server.SetBasePath("/api/v1")
	server.RegisterController("apps", selfWritingController{})

	response := httptest.NewRecorder()
	server.Engine().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/apps/branches", nil))

	if got := ginOutput.String(); strings.Contains(got, "Headers were already written") {
		t.Errorf("the wrapper wrote a status over the handler's response: %s", got)
	}

	if response.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the handler's 500 to survive", response.Code)
	}
	if !strings.Contains(response.Body.String(), "failed to init github") {
		t.Errorf("body = %q, want the handler's payload", response.Body.String())
	}
	if strings.Count(response.Body.String(), "error") != 1 {
		t.Errorf("body was written more than once: %q", response.Body.String())
	}
}

type returningController struct{}

func (returningController) Register(group *ControllerGroup) {
	group.GET("/list", func() ([]string, error) {
		return []string{"alpha"}, nil
	})
}

// TestHandlerReturnValueStillSent guards the ordinary path: a handler that does
// not write its own response must still have its return value serialised.
func TestHandlerReturnValueStillSent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := New()
	server.SetBasePath("/api/v1")
	server.RegisterController("things", returningController{})

	response := httptest.NewRecorder()
	server.Engine().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/things/list", nil))

	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", response.Code)
	}
	if !strings.Contains(response.Body.String(), "alpha") {
		t.Errorf("body = %q, want the returned value", response.Body.String())
	}
}

type unmarshalable struct{}

func (unmarshalable) MarshalJSON() ([]byte, error) { return nil, errors.New("no JSON form") }

type marshalFailingController struct{}

func (marshalFailingController) Register(group *ControllerGroup) {
	group.GET("/broken", func() (any, error) { return unmarshalable{}, nil })
	group.GET("/fine", func() (any, error) { return map[string]string{"html": "<a&b>"}, nil })
}

// TestResponseThatCannotBeMarshalledIsA500 pins that a handler's value is
// marshalled before the status goes out: gin's c.JSON wrote 200 first, so a
// marshal failure reached the client as a 200 with an empty body and was
// logged as a success.
func TestResponseThatCannotBeMarshalledIsA500(t *testing.T) {
	server := New()
	server.SetBasePath("/api")
	server.RegisterController("x", marshalFailingController{})

	response := httptest.NewRecorder()
	server.Engine().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/x/broken", nil))
	if response.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", response.Code)
	}

	// A value that marshals is written exactly as gin's c.JSON writes it.
	response = httptest.NewRecorder()
	server.Engine().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/x/fine", nil))
	want := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(want)
	ginCtx.JSON(http.StatusOK, map[string]string{"html": "<a&b>"})
	if response.Code != http.StatusOK || response.Body.String() != want.Body.String() ||
		response.Header().Get("Content-Type") != want.Header().Get("Content-Type") {
		t.Errorf("got %d %q %q, want 200 %q %q", response.Code, response.Header().Get("Content-Type"), response.Body.String(),
			want.Header().Get("Content-Type"), want.Body.String())
	}
}
