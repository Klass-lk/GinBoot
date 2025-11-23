package ginboot

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestDocument is a sample document for testing
type TestDocument struct {
	ID        string    `bson:"_id" ginboot:"id"`
	Name      string    `bson:"name"`
	Age       int       `bson:"age"`
	CreatedAt time.Time `bson:"created_at"`
}

// setupTestContainer creates a MongoDB test container
func setupTestContainer(t *testing.T) (testcontainers.Container, *MongoConfig, error) {
	ctx := context.Background()

	mongoPort := "27017/tcp"
	natPort := nat.Port(mongoPort)

	req := testcontainers.ContainerRequest{
		Image:        "mongo:latest",
		ExposedPorts: []string{mongoPort},
		WaitingFor: wait.ForAll(
			wait.ForLog("Waiting for connections"),
			wait.ForListeningPort(natPort),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start container: %v", err)
	}

	mappedPort, err := container.MappedPort(ctx, natPort)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get container external port: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get container host: %v", err)
	}

	config := &MongoConfig{
		Host:     host,
		Port:     int(mappedPort.Int()),
		Database: "test_db",
	}

	return container, config, nil
}

func TestMongoRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping MongoDB integration test in short mode")
	}

	// Check if Docker is available
	client, err := testcontainers.NewDockerClient()
	if err != nil {
		t.Skip("Docker not available:", err)
	}
	defer client.Close()

	// Setup test container
	container, config, err := setupTestContainer(t)
	if err != nil {
		t.Fatalf("Failed to setup test container: %v", err)
	}
	defer container.Terminate(context.Background())

	// Create MongoDB connection
	db, err := config.Connect()
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Create repository with explicit collection name
	repo := NewMongoRepository[TestDocument](db, "test_documents")

	// Test cases
	t.Run("Save and FindById", func(t *testing.T) {
		doc := TestDocument{
			ID:        primitive.NewObjectID().Hex(),
			Name:      "John Doe",
			Age:       30,
			CreatedAt: time.Now(),
		}

		// Save document
		err := repo.Save(doc)
		assert.NoError(t, err)

		// Find document by ID
		found, err := repo.FindById(doc.ID)
		assert.NoError(t, err)
		assert.Equal(t, doc.Name, found.Name)
		assert.Equal(t, doc.Age, found.Age)
	})

	t.Run("FindAll", func(t *testing.T) {
		// Create multiple documents
		docs := []TestDocument{
			{
				ID:        primitive.NewObjectID().Hex(),
				Name:      "Alice",
				Age:       25,
				CreatedAt: time.Now(),
			},
			{
				ID:        primitive.NewObjectID().Hex(),
				Name:      "Bob",
				Age:       35,
				CreatedAt: time.Now(),
			},
		}

		// Save all documents
		for _, doc := range docs {
			err := repo.Save(doc)
			assert.NoError(t, err)
		}

		// Find all documents
		found, err := repo.FindAll()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(found), 2)
	})

	t.Run("Update", func(t *testing.T) {
		doc := TestDocument{
			ID:        primitive.NewObjectID().Hex(),
			Name:      "Jane Doe",
			Age:       28,
			CreatedAt: time.Now(),
		}

		// Save initial document
		err := repo.Save(doc)
		assert.NoError(t, err)

		// Update document
		doc.Age = 29
		err = repo.Update(doc)
		assert.NoError(t, err)

		// Verify update
		found, err := repo.FindById(doc.ID)
		assert.NoError(t, err)
		assert.Equal(t, 29, found.Age)
	})

	t.Run("Delete", func(t *testing.T) {
		doc := TestDocument{
			ID:        primitive.NewObjectID().Hex(),
			Name:      "To Delete",
			Age:       40,
			CreatedAt: time.Now(),
		}

		// Save document
		err := repo.Save(doc)
		assert.NoError(t, err)

		// Delete document
		err = repo.Delete(doc.ID)
		assert.NoError(t, err)

		// Verify deletion
		_, err = repo.FindById(doc.ID)
		assert.Error(t, err)
	})

	t.Run("FindBy", func(t *testing.T) {
		// Create test documents
		docs := []TestDocument{
			{
				ID:        primitive.NewObjectID().Hex(),
				Name:      "Same Age",
				Age:       50,
				CreatedAt: time.Now(),
			},
			{
				ID:        primitive.NewObjectID().Hex(),
				Name:      "Also Same Age",
				Age:       50,
				CreatedAt: time.Now(),
			},
		}

		// Save documents
		for _, doc := range docs {
			err := repo.Save(doc)
			assert.NoError(t, err)
		}

		// Find by age
		found, err := repo.FindBy("age", 50)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(found))
		assert.Equal(t, 50, found[0].Age)
		assert.Equal(t, 50, found[1].Age)
	})

	t.Run("FindOneBy", func(t *testing.T) {
		doc := TestDocument{
			ID:        primitive.NewObjectID().Hex(),
			Name:      "Unique Name",
			Age:       45,
			CreatedAt: time.Now(),
		}

		// Save document
		err := repo.Save(doc)
		assert.NoError(t, err)

		// Find one by name
		found, err := repo.FindOneBy("name", "Unique Name")
		assert.NoError(t, err)
		assert.Equal(t, doc.Name, found.Name)
		assert.Equal(t, doc.Age, found.Age)
	})

	t.Run("Pagination", func(t *testing.T) {
		// Create 20 test documents
		for i := 0; i < 20; i++ {
			doc := TestDocument{
				ID:        primitive.NewObjectID().Hex(),
				Name:      fmt.Sprintf("User %d", i),
				Age:       20 + i,
				CreatedAt: time.Now(),
			}
			err := repo.Save(doc)
			assert.NoError(t, err)
		}

		// Test pagination
		pageRequest := PageRequest{
			Page: 1,
			Size: 5,
			Sort: SortField{
				Field:     "age",
				Direction: 1,
			},
		}

		response, err := repo.FindAllPaginated(pageRequest)
		assert.NoError(t, err)
		assert.Equal(t, 5, len(response.Contents))
		assert.True(t, response.TotalPages > 1)
		assert.True(t, response.TotalElements >= 20)
	})
}
