package ginboot

import (
	"bytes"
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
