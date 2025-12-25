package ginboot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// CacheKeyGenerator defines a function to generate a cache key from the request
type CacheKeyGenerator func(c *gin.Context) string

// TagGenerator defines a function to generate tags for the cache entry
type TagGenerator func(c *gin.Context) []string

type cacheWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *cacheWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *cacheWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// DefaultKeyGenerator generates a key based on the request URL and query parameters
func DefaultKeyGenerator(c *gin.Context) string {
	url := c.Request.URL.String()
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

// CacheMiddleware returns a Gin middleware that caches responses
func CacheMiddleware(service CacheService, duration time.Duration, tagGen TagGenerator, keyGen CacheKeyGenerator) gin.HandlerFunc {
	if keyGen == nil {
		keyGen = DefaultKeyGenerator
	}

	return func(c *gin.Context) {
		// Only cache GET requests by default, or let the user decide via middleware usage
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}

		key := keyGen(c)

		// 1. Try to get from cache
		cachedData, err := service.Get(c.Request.Context(), key)
		if err == nil && cachedData != nil {
			// Cache hit
			c.Header("X-Cache", "HIT")
			c.Data(http.StatusOK, "application/json; charset=utf-8", cachedData)
			c.Abort()
			return
		}

		// 2. Cache miss, capture response
		c.Header("X-Cache", "MISS")
		writer := &cacheWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = writer

		c.Next()

		// 3. Save to cache if status is 200 OK
		if c.Writer.Status() == http.StatusOK {
			tags := []string{}
			if tagGen != nil {
				tags = tagGen(c)
			}

			// We execute this in background or synchronously based on preference.
			// Synchronous is safer to ensure consistency but adds latency.
			// Given the requirement "can't rely on in memory cache", reliable persistence is key.
			// Let's do it synchronously for now or decouple if needed.
			// Ideally error shouldn't fail the request.
			_ = service.Set(context.Background(), key, writer.body.Bytes(), tags, duration)
		}
	}
}
