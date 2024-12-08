package ginboot

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type testController struct {
	handleCalled bool
}

func (c *testController) Register(group *ControllerGroup) {
	group.GET("/test", c.handleTest)
}

func (c *testController) handleTest(ctx *Context) {
	c.handleCalled = true
	ctx.JSON(200, gin.H{"status": "ok"})
}

func TestServer_RegisterController(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()
	controller := &testController{}

	server.RegisterController("/api", controller)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/test", nil)
	server.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, controller.handleCalled)
}

func TestControllerGroup_Methods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	methods := []struct {
		name   string
		method string
		setup  func(*ControllerGroup)
	}{
		{
			name:   "GET",
			method: "GET",
			setup: func(g *ControllerGroup) {
				g.GET("/test", func(c *Context) { c.Status(200) })
			},
		},
		{
			name:   "POST",
			method: "POST",
			setup: func(g *ControllerGroup) {
				g.POST("/test", func(c *Context) { c.Status(200) })
			},
		},
		{
			name:   "PUT",
			method: "PUT",
			setup: func(g *ControllerGroup) {
				g.PUT("/test", func(c *Context) { c.Status(200) })
			},
		},
		{
			name:   "DELETE",
			method: "DELETE",
			setup: func(g *ControllerGroup) {
				g.DELETE("/test", func(c *Context) { c.Status(200) })
			},
		},
		{
			name:   "PATCH",
			method: "PATCH",
			setup: func(g *ControllerGroup) {
				g.PATCH("/test", func(c *Context) { c.Status(200) })
			},
		},
		{
			name:   "OPTIONS",
			method: "OPTIONS",
			setup: func(g *ControllerGroup) {
				g.OPTIONS("/test", func(c *Context) { c.Status(200) })
			},
		},
		{
			name:   "HEAD",
			method: "HEAD",
			setup: func(g *ControllerGroup) {
				g.HEAD("/test", func(c *Context) { c.Status(200) })
			},
		},
	}

	for _, tt := range methods {
		t.Run(tt.name, func(t *testing.T) {
			group := server.Group("/api")
			tt.setup(group)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, "/api/test", nil)
			server.engine.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestControllerGroup_Group(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	// Create nested groups
	api := server.Group("/api")
	v1 := api.Group("/v1")
	users := v1.Group("/users")

	// Add handler to nested group
	users.GET("", func(c *Context) {
		c.JSON(200, gin.H{"path": "/api/v1/users"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	server.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestControllerGroup_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := New()

	// Create middleware
	called := false
	middleware := func(c *gin.Context) {
		called = true
		c.Next()
	}

	// Add middleware to group
	api := server.Group("/api")
	api.Use(middleware)
	api.GET("/test", func(c *Context) {
		c.Status(200)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/test", nil)
	server.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}
