package ginboot

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestServer_New(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	assert.NotNil(t, server)
	assert.NotNil(t, server.engine)
}

func TestServer_SetBasePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	// Set base path before registering routes
	server.SetBasePath("/api/v1")

	// Register route after setting base path
	server.Group("").GET("/test", func(c *Context) (string, error) {
		return "test", nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	server.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestServer_CustomCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	origins := []string{"http://localhost:3000"}
	methods := []string{"GET", "POST"}
	headers := []string{"Content-Type"}
	maxAge := 24 * time.Hour

	server.CustomCORS(origins, methods, headers, maxAge)

	server.engine.GET("/test", func(c *gin.Context) {
		c.Status(200)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	server.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET,POST", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestServer_Start(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	// Test invalid port
	err := server.Start(-1)
	assert.Error(t, err)

	// Note: We can't easily test successful server start in a unit test
	// as it blocks. In a real scenario, you might want to use integration tests
	// for this functionality.
}
