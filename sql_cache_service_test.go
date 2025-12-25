package ginboot

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	testSQLCacheDB      *sql.DB
	testSQLCacheRepo    *SQLRepository[CacheEntry]
	testSQLTagRepo      *SQLRepository[TagEntry]
	testSQLCacheService *SQLCacheService
	onceSQLCache        sync.Once
)

func setupSQLCache(t *testing.T) (*SQLCacheService, func()) {
	onceSQLCache.Do(func() {
		ctx := context.Background()

		pgContainer, err := tcpg.Run(ctx,
			"postgres:13-alpine",
			tcpg.WithDatabase("testdb"),
			tcpg.WithUsername("postgres"),
			tcpg.WithPassword("password"),
		)
		if err != nil {
			panic(fmt.Sprintf("Failed to start PostgreSQL container: %v", err))
		}

		connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			panic(fmt.Sprintf("Failed to get PostgreSQL connection string: %v", err))
		}

		testSQLCacheDB, err = sql.Open("postgres", connStr)
		if err != nil {
			panic(fmt.Sprintf("Failed to connect to PostgreSQL: %v", err))
		}

		// Wait for DB
		maxRetries := 5
		for i := 0; i < maxRetries; i++ {
			err = testSQLCacheDB.Ping()
			if err == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			panic(fmt.Sprintf("Failed to ping PostgreSQL: %v", err))
		}

		testSQLCacheRepo = NewSQLRepository[CacheEntry](testSQLCacheDB)
		testSQLTagRepo = NewSQLRepository[TagEntry](testSQLCacheDB)

		testSQLCacheService = NewSQLCacheService(testSQLCacheRepo, testSQLTagRepo)
	})

	// Truncate tables
	if testSQLCacheDB != nil {
		_, _ = testSQLCacheDB.Exec("TRUNCATE TABLE cache_entries")
		_, _ = testSQLCacheDB.Exec("TRUNCATE TABLE cache_tags")
	}

	return testSQLCacheService, func() {}
}

func TestSQLCacheService_SetAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
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
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	service, teardown := setupSQLCache(t)
	defer teardown()

	ctx := context.Background()
	got, err := service.Get(ctx, "missing-key")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestSQLCacheService_Invalidate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
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
