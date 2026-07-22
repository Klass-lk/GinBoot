package sql

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/klass-lk/ginboot"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	testSQLCacheDB      *gorm.DB
	testSQLCacheRepo    *SQLRepository[ginboot.CacheEntry]
	testSQLTagRepo      *SQLRepository[ginboot.TagEntry]
	testSQLCacheService *SQLCacheService
	onceSQLCache        sync.Once
)

func setupSQLCache(t *testing.T) (*SQLCacheService, func()) {
	onceSQLCache.Do(func() {
		var err error
		testSQLCacheDB, err = gorm.Open(sqlite.Open("file::memory_cache_service:?cache=shared"), &gorm.Config{})
		if err != nil {
			panic(err)
		}

		testSQLCacheRepo = NewSQLRepository[ginboot.CacheEntry](testSQLCacheDB)
		testSQLTagRepo = NewSQLRepository[ginboot.TagEntry](testSQLCacheDB)

		testSQLCacheService = NewSQLCacheService(testSQLCacheRepo, testSQLTagRepo)
	})

	if testSQLCacheDB != nil {
		_ = testSQLCacheRepo.DeleteAll()
		_ = testSQLTagRepo.DeleteAll()
	}

	return testSQLCacheService, func() {}
}

func TestSQLCacheService_SetAndGet(t *testing.T) {
	service, teardown := setupSQLCache(t)
	defer teardown()

	ctx := context.Background()
	key := "test-key"
	val := []byte("test-val")
	tags := []string{"tag1", "tag2"}

	err := service.Set(ctx, key, val, tags, time.Minute)
	assert.NoError(t, err)

	got, err := service.Get(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, val, got)
}

func TestSQLCacheService_GetMiss(t *testing.T) {
	service, teardown := setupSQLCache(t)
	defer teardown()

	ctx := context.Background()
	got, err := service.Get(ctx, "missing-key")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestSQLCacheService_Invalidate(t *testing.T) {
	service, teardown := setupSQLCache(t)
	defer teardown()

	ctx := context.Background()
	key1 := "k1"
	val1 := []byte("v1")
	key2 := "k2"
	val2 := []byte("v2")

	// Set k1 with tag1
	err := service.Set(ctx, key1, val1, []string{"tag1"}, time.Minute)
	assert.NoError(t, err)

	// Set k2 with tag2
	err = service.Set(ctx, key2, val2, []string{"tag2"}, time.Minute)
	assert.NoError(t, err)

	// Invalidate tag1 -> should remove k1
	err = service.Invalidate(ctx, "tag1")
	assert.NoError(t, err)

	// Check k1 gone
	got1, err := service.Get(ctx, key1)
	assert.NoError(t, err)
	assert.Nil(t, got1)

	// Check k2 still there
	got2, err := service.Get(ctx, key2)
	assert.NoError(t, err)
	assert.Equal(t, val2, got2)
}
