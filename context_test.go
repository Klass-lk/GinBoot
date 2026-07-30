package ginboot

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/config"
	"github.com/klass-lk/ginboot/service"
	"github.com/stretchr/testify/assert"
)

func TestContext_GetAuthContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		setupContext func(*gin.Context)
		expectError  bool
		expectedAuth AuthContext
	}{
		{
			name: "successful auth context retrieval",
			setupContext: func(c *gin.Context) {
				c.Set("user_id", "123")
				c.Set("role", "admin")
			},
			expectError: false,
			expectedAuth: AuthContext{
				UserID: "123",
				Roles:  []string{"admin"},
			},
		},
		{
			name: "missing user_id",
			setupContext: func(c *gin.Context) {
				c.Set("role", "admin")
			},
			expectError: true,
		},
		{
			name: "missing role",
			setupContext: func(c *gin.Context) {
				c.Set("user_id", "123")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			tt.setupContext(c)

			ctx := NewContext(c, nil, nil, nil)
			auth, err := ctx.GetAuthContext()

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, http.StatusUnauthorized, w.Code)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAuth, auth)
			}
		})
	}
}

type TestRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func TestContext_GetRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		requestBody string
		expectError bool
	}{
		{
			name:        "valid request",
			requestBody: `{"name":"John","email":"john@example.com"}`,
			expectError: false,
		},
		{
			name:        "invalid json",
			requestBody: `{"name":"John","email":}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			ctx := NewContext(c, nil, nil, nil)
			var testReq TestRequest
			err := ctx.GetRequest(&testReq)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, http.StatusBadRequest, w.Code)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "John", testReq.Name)
				assert.Equal(t, "john@example.com", testReq.Email)
			}
		})
	}
}

func TestContext_GetPageRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		queryParams  map[string]string
		expectedPage int
		expectedSize int
		expectedSort SortField
		expectAbort  bool
	}{
		{
			name:         "default values",
			queryParams:  map[string]string{},
			expectedPage: 1,
			expectedSize: 10,
			expectedSort: SortField{Field: "_id", Direction: 1},
		},
		{
			name: "custom values",
			queryParams: map[string]string{
				"page": "2",
				"size": "20",
				"sort": "name,desc",
			},
			expectedPage: 2,
			expectedSize: 20,
			expectedSort: SortField{Field: "name", Direction: -1},
		},
		{
			name: "single sort field without comma",
			queryParams: map[string]string{
				"sort": "name",
			},
			expectedPage: 1,
			expectedSize: 10,
			expectedSort: SortField{Field: "name", Direction: 1},
		},
		{
			name: "invalid page",
			queryParams: map[string]string{
				"page": "invalid",
			},
			expectAbort: true,
		},
		{
			name: "invalid size",
			queryParams: map[string]string{
				"size": "invalid",
			},
			expectAbort: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Build URL with query parameters
			req := httptest.NewRequest("GET", "/?", nil)
			q := req.URL.Query()
			for key, value := range tt.queryParams {
				q.Add(key, value)
			}
			req.URL.RawQuery = q.Encode()
			c.Request = req

			ctx := NewContext(c, nil, nil, nil)
			result := ctx.GetPageRequest()

			if tt.expectAbort {
				assert.Equal(t, http.StatusBadRequest, w.Code)
			} else {
				assert.Equal(t, tt.expectedPage, result.Page)
				assert.Equal(t, tt.expectedSize, result.Size)
				assert.Equal(t, tt.expectedSort, result.Sort)
			}
		})
	}
}

func TestContext_GetFileService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	mFS := &mockFileService{}
	ctx := NewContext(c, mFS, nil, nil)
	assert.Equal(t, mFS, ctx.GetFileService())
}

func TestContext_Logger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/", nil)
	c.Request = req

	// Case 1: Custom logger provided
	customLogger := NewSlogLogger(nil)
	ctx1 := NewContext(c, nil, customLogger, nil)
	assert.NotNil(t, ctx1.Logger())

	// Case 2: Nil logger (fallback)
	ctx2 := NewContext(c, nil, nil, nil)
	assert.NotNil(t, ctx2.Logger())
}

func TestContext_RecordError_Nil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/", nil)
	c.Request = req
	ctx := NewContext(c, nil, nil, nil)

	assert.NotPanics(t, func() {
		ctx.RecordError(nil)
	})
}

func TestContext_SendError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "ApiError with valid HTTP status code",
			err:            ApiError{ErrorCode: "404", Message: "Not Found"},
			expectedStatus: http.StatusNotFound,
			expectedCode:   "404",
		},
		{
			name:           "ApiError with non-numeric error code",
			err:            ApiError{ErrorCode: "INVALID_PARAM", Message: "Bad parameter"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAM",
		},
		{
			name:           "Generic error",
			err:            errors.New("internal error"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest("GET", "/", nil)
			c.Request = req

			ctx := NewContext(c, nil, nil, nil)
			ctx.SendError(tt.err)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedCode)
		})
	}
}

func TestContextServiceClientHelpers(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		Ginboot: config.GinbootRootConfig{
			Services: map[string]config.ServiceTargetConfig{
				"mock-svc": {URL: mockServer.URL},
			},
		},
	}

	client := service.NewServiceClient(service.NewConfigServiceResolver(cfg))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/", nil)
	c.Request = req

	ctx := NewContext(c, nil, nil, client)

	assert.Equal(t, client, ctx.ServiceClient())

	var target map[string]string
	err := ctx.CallService("mock-svc", "/test", nil, &target)
	assert.NoError(t, err)
	assert.Equal(t, "ok", target["status"])

	err = ctx.CallServiceAsync("mock-svc", "/test", nil)
	assert.NoError(t, err)

	err = ctx.CallServiceWithMethod("POST", "mock-svc", "/test", nil, &target)
	assert.NoError(t, err)

	err = ctx.CallServiceAsyncWithMethod("POST", "mock-svc", "/test", nil)
	assert.NoError(t, err)
}

