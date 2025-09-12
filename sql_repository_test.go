// This is a test comment to trigger re-evaluation.
package ginboot

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/stretchr/testify/assert"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	testSQLTableName = "test_entities"
)

// TestSQLEntity is a simple struct for testing SQL repository
type TestSQLEntity struct {
	ID   string `db:"id"`
	Name string `db:"name"`
	Age  int    `db:"age"`
}

// GetTableName implements the Document interface for TestSQLEntity
func (t TestSQLEntity) GetTableName() string {
	return testSQLTableName
}

// Global variables for SQL setup
var (
	testSQLDB   *sql.DB
	testSQLRepo *SQLRepository[TestSQLEntity]
	onceSQL     sync.Once
)

func setupSQL(t *testing.T) (*SQLRepository[TestSQLEntity], func()) {
	onceSQL.Do(func() {
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

		// Initialize PostgreSQL connection
		testSQLDB, err = sql.Open("postgres", connStr)
		if err != nil {
			panic(fmt.Sprintf("Failed to connect to PostgreSQL: %v", err))
		}

		// Ping the database to ensure connection is established, with retries
		maxRetries := 5
		for i := 0; i < maxRetries; i++ {
			err = testSQLDB.Ping()
			if err == nil {
				break
			}
			fmt.Printf("Failed to ping PostgreSQL, retrying (%d/%d): %v\n", i+1, maxRetries, err)
			time.Sleep(1 * time.Second) // Wait for 1 second before retrying
		}
		if err != nil {
			panic(fmt.Sprintf("Failed to ping PostgreSQL after %d retries: %v", maxRetries, err))
		}

		// Initialize SQLRepository
		testSQLRepo = NewSQLRepository[TestSQLEntity](testSQLDB)

		// Create table for testing
		err = testSQLRepo.CreateTable()
		if err != nil {
			panic(fmt.Sprintf("Failed to create test table: %v", err))
		}
	})

	// Clear table before each test
	_, err := testSQLDB.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY", testSQLTableName))
	assert.NoError(t, err)

	return testSQLRepo, func() { /* no-op teardown for individual tests */ }
}

func TestSQLRepository_CreateTable(t *testing.T) {
	// Table is created in TestMain, so this test just verifies its existence
	// by trying to create it again (which should not return an error if it exists)
	repo, teardown := setupSQL(t)
	defer teardown()

	err := repo.CreateTable()
	assert.NoError(t, err)
}

func TestSQLRepository_SaveAndFindById(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity := TestSQLEntity{ID: "1", Name: "TestName", Age: 30}
	err := repo.Save(entity)
	assert.NoError(t, err)

	found, err := repo.FindById("1")
	assert.NoError(t, err)
	assert.Equal(t, entity, found)
}

func TestSQLRepository_Update(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity := TestSQLEntity{ID: "1", Name: "OldName", Age: 25}
	err := repo.Save(entity)
	assert.NoError(t, err)

	updatedEntity := TestSQLEntity{ID: "1", Name: "NewName", Age: 35}
	err = repo.Update(updatedEntity)
	assert.NoError(t, err)

	found, err := repo.FindById("1")
	assert.NoError(t, err)
	assert.Equal(t, updatedEntity, found)
}

func TestSQLRepository_FindAllById(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	// Test with empty IDs slice
	found, err := repo.FindAllById([]string{})
	assert.NoError(t, err)
	assert.Empty(t, found)

	// Save some entities
	entity1 := TestSQLEntity{ID: "1", Name: "TestName1", Age: 10}
	entity2 := TestSQLEntity{ID: "2", Name: "TestName2", Age: 20}
	err = repo.Save(entity1)
	assert.NoError(t, err)
	err = repo.Save(entity2)
	assert.NoError(t, err)

	// Test with existing IDs
	found, err = repo.FindAllById([]string{"1", "2"})
	assert.NoError(t, err)
	assert.Len(t, found, 2)
	assert.Contains(t, found, entity1)
	assert.Contains(t, found, entity2)

	// Test with non-existent IDs
	found, err = repo.FindAllById([]string{"3"})
	assert.NoError(t, err)
	assert.Empty(t, found)

	// Test with a mix of existing and non-existent IDs
	found, err = repo.FindAllById([]string{"1", "3"})
	assert.NoError(t, err)
	assert.Len(t, found, 1)
	assert.Contains(t, found, entity1)
}

func TestSQLRepository_SaveOrUpdate(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity := TestSQLEntity{ID: "1", Name: "TestName", Age: 30}
	err := repo.SaveOrUpdate(entity)
	assert.NoError(t, err)

	found, err := repo.FindById("1")
	assert.NoError(t, err)
	assert.Equal(t, entity, found)

	// Update existing entity
	updatedEntity := TestSQLEntity{ID: "1", Name: "UpdatedName", Age: 35}
	err = repo.SaveOrUpdate(updatedEntity)
	assert.NoError(t, err)

	found, err = repo.FindById("1")
	assert.NoError(t, err)
	assert.Equal(t, updatedEntity, found)
}

func TestSQLRepository_SaveAll(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	// Test with empty slice
	err := repo.SaveAll([]TestSQLEntity{})
	assert.NoError(t, err)

	// Test with multiple entities
	entities := []TestSQLEntity{
		{ID: "1", Name: "Entity1", Age: 10},
		{ID: "2", Name: "Entity2", Age: 20},
	}
	err = repo.SaveAll(entities)
	assert.NoError(t, err)

	found1, err := repo.FindById("1")
	assert.NoError(t, err)
	assert.Equal(t, entities[0], found1)

	found2, err := repo.FindById("2")
	assert.NoError(t, err)
	assert.Equal(t, entities[1], found2)
}

func TestSQLRepository_FindOneBy(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity := TestSQLEntity{ID: "1", Name: "UniqueName", Age: 30}
	err := repo.Save(entity)
	assert.NoError(t, err)

	// Find by existing field and value
	found, err := repo.FindOneBy("name", "UniqueName")
	assert.NoError(t, err)
	assert.Equal(t, entity, found)

	// Find by non-existent field and value
	_, err = repo.FindOneBy("name", "NonExistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no rows in result set")

	// Find by non-existent field
	_, err = repo.FindOneBy("nonexistent_field", "some_value")
	assert.Error(t, err)
}

func TestSQLRepository_FindOneByFilters(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity := TestSQLEntity{ID: "1", Name: "FilteredName", Age: 40}
	err := repo.Save(entity)
	assert.NoError(t, err)

	// Find by multiple filters
	filters := map[string]interface{}{"name": "FilteredName", "age": 40}
	found, err := repo.FindOneByFilters(filters)
	assert.NoError(t, err)
	assert.Equal(t, entity, found)

	// Find by filters with no match
	filtersNoMatch := map[string]interface{}{"name": "FilteredName", "age": 99}
	_, err = repo.FindOneByFilters(filtersNoMatch)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no rows in result set")
}

func TestSQLRepository_FindBy(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity1 := TestSQLEntity{ID: "1", Name: "SharedName", Age: 10}
	entity2 := TestSQLEntity{ID: "2", Name: "SharedName", Age: 20}
	entity3 := TestSQLEntity{ID: "3", Name: "UniqueName", Age: 30}
	err := repo.Save(entity1)
	assert.NoError(t, err)
	err = repo.Save(entity2)
	assert.NoError(t, err)
	err = repo.Save(entity3)
	assert.NoError(t, err)

	// Find by existing field and value (multiple results)
	found, err := repo.FindBy("name", "SharedName")
	assert.NoError(t, err)
	assert.Len(t, found, 2)
	assert.Contains(t, found, entity1)
	assert.Contains(t, found, entity2)

	// Find by non-existent field and value
	found, err = repo.FindBy("name", "NonExistent")
	assert.NoError(t, err)
	assert.Empty(t, found)
}

func TestSQLRepository_FindByFilters(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity1 := TestSQLEntity{ID: "1", Name: "FilterTest", Age: 10}
	entity2 := TestSQLEntity{ID: "2", Name: "FilterTest", Age: 20}
	entity3 := TestSQLEntity{ID: "3", Name: "Another", Age: 10}
	err := repo.Save(entity1)
	assert.NoError(t, err)
	err = repo.Save(entity2)
	assert.NoError(t, err)
	err = repo.Save(entity3)
	assert.NoError(t, err)

	// Find by multiple filters
	filters := map[string]interface{}{"name": "FilterTest", "age": 10}
	found, err := repo.FindByFilters(filters)
	assert.NoError(t, err)
	assert.Len(t, found, 1)
	assert.Contains(t, found, entity1)

	// Find by filters with no match
	filtersNoMatch := map[string]interface{}{"name": "FilterTest", "age": 99}
	found, err = repo.FindByFilters(filtersNoMatch)
	assert.NoError(t, err)
	assert.Empty(t, found)
}

func TestSQLRepository_FindAll(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	// Test with empty table
	found, err := repo.FindAll()
	assert.NoError(t, err)
	assert.Empty(t, found)

	// Save some entities
	entity1 := TestSQLEntity{ID: "1", Name: "All1", Age: 10}
	entity2 := TestSQLEntity{ID: "2", Name: "All2", Age: 20}
	err = repo.Save(entity1)
	assert.NoError(t, err)
	err = repo.Save(entity2)
	assert.NoError(t, err)

	// Find all existing entities
	found, err = repo.FindAll()
	assert.NoError(t, err)
	assert.Len(t, found, 2)
	assert.Contains(t, found, entity1)
	assert.Contains(t, found, entity2)
}

func TestSQLRepository_CountBy(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity1 := TestSQLEntity{ID: "1", Name: "CountName", Age: 10}
	entity2 := TestSQLEntity{ID: "2", Name: "CountName", Age: 20}
	err := repo.Save(entity1)
	assert.NoError(t, err)
	err = repo.Save(entity2)
	assert.NoError(t, err)

	// Count by existing field and value
	count, err := repo.CountBy("name", "CountName")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Count by non-existent field and value
	count, err = repo.CountBy("name", "NonExistent")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSQLRepository_ExistsBy(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity := TestSQLEntity{ID: "1", Name: "ExistsName", Age: 10}
	err := repo.Save(entity)
	assert.NoError(t, err)

	// Exists by existing field and value
	exists, err := repo.ExistsBy("name", "ExistsName")
	assert.NoError(t, err)
	assert.True(t, exists)

	// Exists by non-existent field and value
	exists, err = repo.ExistsBy("name", "NonExistent")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestSQLRepository_CountByFilters(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity1 := TestSQLEntity{ID: "1", Name: "FilterCount", Age: 10}
	entity2 := TestSQLEntity{ID: "2", Name: "FilterCount", Age: 20}
	entity3 := TestSQLEntity{ID: "3", Name: "AnotherCount", Age: 10}
	err := repo.Save(entity1)
	assert.NoError(t, err)
	err = repo.Save(entity2)
	assert.NoError(t, err)
	err = repo.Save(entity3)
	assert.NoError(t, err)

	// Count by multiple filters
	filters := map[string]interface{}{"name": "FilterCount", "age": 10}
	count, err := repo.CountByFilters(filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Count by filters with no match
	filtersNoMatch := map[string]interface{}{"name": "FilterCount", "age": 99}
	count, err = repo.CountByFilters(filtersNoMatch)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSQLRepository_ExistsByFilters(t *testing.T) {
	repo, teardown := setupSQL(t)
	defer teardown()

	entity := TestSQLEntity{ID: "1", Name: "ExistsFilter", Age: 10}
	err := repo.Save(entity)
	assert.NoError(t, err)

	// Exists by multiple filters
	filters := map[string]interface{}{"name": "ExistsFilter", "age": 10}
	exists, err := repo.ExistsByFilters(filters)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Exists by filters with no match
	filtersNoMatch := map[string]interface{}{"name": "ExistsFilter", "age": 99}
	exists, err = repo.ExistsByFilters(filtersNoMatch)
	assert.NoError(t, err)
	assert.False(t, exists)
}
