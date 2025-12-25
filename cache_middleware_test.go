package ginboot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCacheService is a mock implementation of CacheService interface
type MockCacheService struct {
	mock.Mock
}

func (m *MockCacheService) Set(ctx context.Context, key string, data []byte, tags []string, duration time.Duration) error {
	args := m.Called(ctx, key, data, tags, duration)
	return args.Error(0)
}

func (m *MockCacheService) Get(ctx context.Context, key string) ([]byte, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCacheService) Invalidate(ctx context.Context, tags ...string) error {
	args := m.Called(ctx, tags)
	return args.Error(0)
}

func TestCacheMiddleware_Miss(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockService := new(MockCacheService)

	// Setup router
	r := gin.New()
	r.Use(CacheMiddleware(mockService, time.Minute, nil, nil))
	r.GET("/test", func(c *gin.Context) {
		c.String(200, "hello world")
	})

	// Expect Get (miss)
	mockService.On("Get", mock.Anything, mock.Anything).Return(nil, nil)

	// Expect Set
	mockService.On("Set", mock.Anything, mock.Anything, []byte("hello world"), []string{}, time.Minute).Return(nil)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "hello world", w.Body.String())
	mockService.AssertExpectations(t)
}

func TestCacheMiddleware_Hit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockService := new(MockCacheService)

	cachedResponse := []byte("cached response")

	// Setup router
	r := gin.New()
	r.Use(CacheMiddleware(mockService, time.Minute, nil, nil))
	r.GET("/test-hit", func(c *gin.Context) {
		c.String(200, "should not run")
	})

	// Expect Get (hit)
	mockService.On("Get", mock.Anything, mock.Anything).Return(cachedResponse, nil)

	// Expect NO Set

	req := httptest.NewRequest(http.MethodGet, "/test-hit", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "cached response", w.Body.String()) // Should return cached data, not "should not run"
	mockService.AssertExpectations(t)
}

func TestCacheMiddleware_Tags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockService := new(MockCacheService)

	tagGen := func(c *gin.Context) []string {
		return []string{"tag1"}
	}

	// Setup router
	r := gin.New()
	r.Use(CacheMiddleware(mockService, time.Minute, tagGen, nil))
	r.GET("/test-tags", func(c *gin.Context) {
		c.String(200, "hello tags")
	})

	// Expect Get (miss)
	mockService.On("Get", mock.Anything, mock.Anything).Return(nil, nil)

	// Expect Set with tags
	mockService.On("Set", mock.Anything, mock.Anything, []byte("hello tags"), []string{"tag1"}, time.Minute).Return(nil)

	req := httptest.NewRequest(http.MethodGet, "/test-tags", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	mockService.AssertExpectations(t)
}
