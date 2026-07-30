package ginboot

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/config"
	"github.com/klass-lk/ginboot/service"
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

func TestServer_Engine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()
	assert.Equal(t, server.engine, server.Engine())
}

func TestServer_CustomRunner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	runnerCalled := false
	customRunner := func(engine *gin.Engine) error {
		runnerCalled = true
		assert.NotNil(t, engine)
		return nil
	}

	server.SetRunner(customRunner)
	err := server.Start(8080)

	assert.NoError(t, err)
	assert.True(t, runnerCalled)
}

func TestServer_StartWithSwaggerExport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a temp file path for testing
	tmpFile := "test_swagger.json"
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})

	// Set the environment variable
	t.Setenv("GINBOOT_EXPORT_SWAGGER", tmpFile)

	// Mock os.Exit
	exitCalled := false
	var exitCode int
	originalOsExit := osExit
	osExit = func(code int) {
		exitCalled = true
		exitCode = code
	}
	defer func() { osExit = originalOsExit }()

	server := New()
	server.Group("/api").GET("/test", func(c *Context) (string, error) {
		return "test", nil
	})

	// Use a custom runner to prevent blocking
	customRunner := func(engine *gin.Engine) error {
		return nil
	}
	server.SetRunner(customRunner)

	err := server.Start(8080)
	assert.NoError(t, err)
	assert.True(t, exitCalled)
	assert.Equal(t, 0, exitCode)
}

func TestServer_CustomRunnerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	customRunner := func(engine *gin.Engine) error {
		return assert.AnError
	}

	server.SetRunner(customRunner)
	err := server.Start(8080)

	assert.Equal(t, assert.AnError, err)
}

type mockFileService struct {
	FileService
}

func TestServer_BindFileService(t *testing.T) {
	server := New()
	mFS := &mockFileService{}
	server.BindFileService(mFS)
	assert.Equal(t, mFS, server.fileService)
}

func TestServer_DefaultCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()
	server.DefaultCORS()
	assert.NotNil(t, server.corsConfig)
	assert.True(t, server.corsConfig.AllowAllOrigins)
}

func TestServer_DebugPrintRoute(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	assert.NotNil(t, gin.DebugPrintRouteFunc)
	assert.NotPanics(t, func() {
		gin.DebugPrintRouteFunc("GET", "/test-debug", "main.handler", 1)
	})
}

func TestIsExportingSwagger(t *testing.T) {
	assert.False(t, IsExportingSwagger())
	t.Setenv("GINBOOT_EXPORT_SWAGGER", "true")
	assert.True(t, IsExportingSwagger())
}

func TestServer_HealthRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()
	// Test relative basePath without leading slash to trigger !strings.HasPrefix(p, "/")
	server.SetBasePath("api/v1")
	server.registerDefaultHealthRoutes()

	routes := []string{"/healthz", "/health", "/api/v1/healthz", "/api/v1/health"}
	for _, route := range routes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", route, nil)
		server.engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"UP"`)
	}
}

func TestServer_StartWithSwaggerExportFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("GINBOOT_EXPORT_SWAGGER", "/invalid_dir_path_xyz/swagger.json")

	exitCalled := false
	var exitCode int
	originalOsExit := osExit
	osExit = func(code int) {
		exitCalled = true
		exitCode = code
	}
	defer func() { osExit = originalOsExit }()

	server := New()
	err := server.Start(8080)
	assert.Error(t, err)
	assert.True(t, exitCalled)
	assert.Equal(t, 1, exitCode)
}

func TestServer_SetLogger(t *testing.T) {
	server := New()
	logger := NewSlogLogger(nil)
	server.SetLogger(logger)
	assert.Equal(t, logger, server.logger)
}

func TestServer_ConfigAndServiceClient(t *testing.T) {
	server := New()
	assert.NotNil(t, server.Config())
	assert.NotNil(t, server.ServiceClient())

	cfg := &config.Config{}
	server.SetConfig(cfg)
	assert.Equal(t, cfg, server.Config())

	mockClient := service.NewServiceClient(nil)
	server.SetServiceClient(mockClient)
	assert.Equal(t, mockClient, server.ServiceClient())
}

