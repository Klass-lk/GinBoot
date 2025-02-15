package ginboot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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

			ctx := NewContext(c, nil)
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

			ctx := NewContext(c, nil)
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

			ctx := NewContext(c, nil)
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
