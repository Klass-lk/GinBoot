package ginboot

import (
	"context"
	"time"
)

// CacheService defines the interface for caching operations
type CacheService interface {
	// Set stores a value in the cache with the given key, tags, and duration
	Set(ctx context.Context, key string, data []byte, tags []string, duration time.Duration) error

	// Get retrieves a value from the cache by key
	Get(ctx context.Context, key string) ([]byte, error)

	// Invalidate removes all cache entries associated with the given tags
	Invalidate(ctx context.Context, tags ...string) error
}
