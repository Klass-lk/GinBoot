package ginboot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type TestRouterRequest struct {
	Name string `json:"name"`
}

type TestResponse struct {
	Message string `json:"message"`
}

// Mock middleware for testing
func testMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("middleware", "called")
		c.Next()
	}
}

func TestRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("server group creation", func(t *testing.T) {
		server := &Server{
			engine:   gin.New(),
			basePath: "/api",
		}
		group := server.Group("/v1")
		assert.NotNil(t, group)
		assert.Equal(t, "/api/v1", group.group.BasePath())
	})

	t.Run("controller registration", func(t *testing.T) {
		server := &Server{
			engine: gin.New(),
		}

		mockController := &MockController{}
		server.RegisterController("/test", mockController)
		assert.True(t, mockController.registerCalled)
	})

	t.Run("handler wrapper tests", func(t *testing.T) {
		tests := []struct {
			name         string
			handler      interface{}
			method       string
			path         string
			body         string
			expectedCode int
			expectedBody string
			middleware   []gin.HandlerFunc
		}{
			{
				name: "no args handler",
				handler: func() (*TestResponse, error) {
					return &TestResponse{Message: "success"}, nil
				},
				method:       "GET",
				expectedCode: http.StatusOK,
				expectedBody: `{"message":"success"}`,
			},
			{
				name: "string response handler",
				handler: func() (string, error) {
					return "plain text response", nil
				},
				method:       "GET",
				expectedCode: http.StatusOK,
				expectedBody: "plain text response",
			},
			{
				name: "context only handler",
				handler: func(ctx *Context) (*TestResponse, error) {
					return &TestResponse{Message: "with context"}, nil
				},
				method:       "GET",
				expectedCode: http.StatusOK,
				expectedBody: `{"message":"with context"}`,
			},
			{
				name: "request only handler",
				handler: func(req TestRouterRequest) (*TestResponse, error) {
					return &TestResponse{Message: "Hello " + req.Name}, nil
				},
				method:       "POST",
				body:         `{"name":"world"}`,
				expectedCode: http.StatusOK,
				expectedBody: `{"message":"Hello world"}`,
			},
			{
				name: "context and request handler",
				handler: func(ctx *Context, req TestRouterRequest) (*TestResponse, error) {
					return &TestResponse{Message: "Hello " + req.Name}, nil
				},
				method:       "POST",
				body:         `{"name":"world"}`,
				expectedCode: http.StatusOK,
				expectedBody: `{"message":"Hello world"}`,
			},
			{
				name: "invalid request body",
				handler: func(req TestRouterRequest) (*TestResponse, error) {
					return &TestResponse{Message: "Hello " + req.Name}, nil
				},
				method:       "POST",
				body:         `invalid json`,
				expectedCode: http.StatusBadRequest,
			},
			{
				name: "with middleware",
				handler: func(ctx *Context) (*TestResponse, error) {
					if _, exists := ctx.Get("middleware"); !exists {
						t.Error("Middleware was not called")
					}
					return &TestResponse{Message: "with middleware"}, nil
				},
				method:       "GET",
				expectedCode: http.StatusOK,
				expectedBody: `{"message":"with middleware"}`,
				middleware:   []gin.HandlerFunc{testMiddleware()},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server := &Server{engine: gin.New()}
				group := server.Group("/test")

				switch tt.method {
				case "GET":
					group.GET("", tt.handler, tt.middleware...)
				case "POST":
					group.POST("", tt.handler, tt.middleware...)
				case "PUT":
					group.PUT("", tt.handler, tt.middleware...)
				case "DELETE":
					group.DELETE("", tt.handler, tt.middleware...)
				}

				w := httptest.NewRecorder()
				req := httptest.NewRequest(tt.method, "/test", strings.NewReader(tt.body))
				if tt.body != "" {
					req.Header.Set("Content-Type", "application/json")
				}
				server.engine.ServeHTTP(w, req)

				assert.Equal(t, tt.expectedCode, w.Code)
				if tt.expectedBody != "" {
					if w.Header().Get("Content-Type") == "text/plain; charset=utf-8" {
						assert.Equal(t, tt.expectedBody, w.Body.String())
					} else {
						var expected, actual map[string]interface{}
						err := json.Unmarshal([]byte(tt.expectedBody), &expected)
						assert.NoError(t, err)
						err = json.Unmarshal(w.Body.Bytes(), &actual)
						assert.NoError(t, err)
						assert.Equal(t, expected, actual)
					}
				}
			})
		}
	})

	t.Run("group methods", func(t *testing.T) {
		server := &Server{engine: gin.New()}
		group := server.Group("/test")

		methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
		for _, method := range methods {
			t.Run(method+" method", func(t *testing.T) {
				handler := func(ctx *Context) (*TestResponse, error) {
					return &TestResponse{Message: method}, nil
				}

				switch method {
				case "GET":
					group.GET("/"+method, handler)
				case "POST":
					group.POST("/"+method, handler)
				case "PUT":
					group.PUT("/"+method, handler)
				case "DELETE":
					group.DELETE("/"+method, handler)
				case "PATCH":
					group.PATCH("/"+method, handler)
				case "OPTIONS":
					group.OPTIONS("/"+method, handler)
				case "HEAD":
					group.HEAD("/"+method, handler)
				}

				w := httptest.NewRecorder()
				req := httptest.NewRequest(method, "/test/"+method, nil)
				server.engine.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code)
				if method != "HEAD" {
					var response TestResponse
					err := json.Unmarshal(w.Body.Bytes(), &response)
					assert.NoError(t, err)
					assert.Equal(t, method, response.Message)
				}
			})
		}
	})

	t.Run("nested groups", func(t *testing.T) {
		server := &Server{engine: gin.New()}
		group1 := server.Group("/v1")
		group2 := group1.Group("/api")
		group3 := group2.Group("/test")

		handler := func(ctx *Context) (*TestResponse, error) {
			return &TestResponse{Message: "nested"}, nil
		}
		group3.GET("", handler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/api/test", nil)
		server.engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var response TestResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "nested", response.Message)
	})

	t.Run("middleware chain", func(t *testing.T) {
		server := &Server{engine: gin.New()}
		group := server.Group("/test")

		middleware1Called := false
		middleware2Called := false

		middleware1 := func(c *gin.Context) {
			middleware1Called = true
			c.Next()
		}
		middleware2 := func(c *gin.Context) {
			middleware2Called = true
			c.Next()
		}

		group.Use(middleware1)
		group.GET("", func(ctx *Context) (*TestResponse, error) {
			return &TestResponse{Message: "success"}, nil
		}, middleware2)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		server.engine.ServeHTTP(w, req)

		assert.True(t, middleware1Called)
		assert.True(t, middleware2Called)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// Mock controller for testing
type MockController struct {
	registerCalled bool
}

func (m *MockController) Register(group *ControllerGroup) {
	m.registerCalled = true
}
