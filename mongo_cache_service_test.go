package ginboot

import (
	"context"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupMongoCache(t *testing.T) (*MongoCacheService, func()) {
	ctx := context.Background()
	mongoPort := "27017/tcp"

	// Basic container setup matching mongo_repository_test
	req := testcontainers.ContainerRequest{
		Image:        "mongo:latest",
		ExposedPorts: []string{mongoPort},
		WaitingFor: wait.ForAll(
			wait.ForLog("Waiting for connections"),
			wait.ForListeningPort(nat.Port(mongoPort)),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("Could not start mongo container: %v", err)
		return nil, nil
	}

	mappedPort, _ := container.MappedPort(ctx, nat.Port(mongoPort))
	host, _ := container.Host(ctx)

	config := &MongoConfig{
		Host:     host,
		Port:     int(mappedPort.Int()),
		Database: "test_cache_db",
	}

	db, err := config.Connect()
	if err != nil {
		t.Fatalf("Failed to connect to Mongo: %v", err)
	}

	repo := NewMongoRepository[CacheEntry](db, "cache_entries")
	service := NewMongoCacheService(repo)

	return service, func() {
		container.Terminate(ctx)
	}
}

func TestMongoCacheService_SetAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	service, teardown := setupMongoCache(t)
	if service == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	key := "m-key"
	val := []byte("m-val")
	tags := []string{"t1"}

	err := service.Set(ctx, key, val, tags, time.Minute)
	assert.NoError(t, err)

	got, err := service.Get(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, val, got)
}

func TestMongoCacheService_Invalidate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	service, teardown := setupMongoCache(t)
	if service == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	key1 := "mk1"
	val1 := []byte("mv1")

	// Set k1 with tag1
	err := service.Set(ctx, key1, val1, []string{"tag1"}, time.Minute)
	assert.NoError(t, err)

	// Invalidate tag1
	err = service.Invalidate(ctx, "tag1")
	assert.NoError(t, err)

	// Check k1 gone
	got1, err := service.Get(ctx, key1)
	assert.NoError(t, err)
	assert.Nil(t, got1)
}
