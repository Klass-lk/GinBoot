package ginboot

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	tcddb "github.com/testcontainers/testcontainers-go/modules/dynamodb"
)

var (
	testDynamoClient *dynamodb.Client
	testRepo         *DynamoDBRepository[TestEntity]
	testTeardown     func()
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	dynamoDBContainer, err := tcddb.Run(ctx,
		"amazon/dynamodb-local:latest",
	)
	if err != nil {
		panic(err)
	}

	endpoint, err := dynamoDBContainer.Endpoint(ctx, "")
	if err != nil {
		panic(err)
	}

	cfg := aws.Config{
		Region: "us-east-1",
		EndpointResolver: aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
			return aws.Endpoint{URL: "http://" + endpoint}, nil
		}),
		Credentials: credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
	}

	testDynamoClient = dynamodb.NewFromConfig(cfg)

	// Set the table name globally
	NewDynamoDBConfig().WithTableName("test-table").WithSkipTableCreation(false)

	// Now initialize the actual testRepo that will be used by tests
	testRepo = NewDynamoDBRepository[TestEntity](testDynamoClient)

	testTeardown = func() {
		if err := dynamoDBContainer.Terminate(ctx); err != nil {
			panic(err)
		}
	}

	exitCode := m.Run()
	testTeardown()
	os.Exit(exitCode)
}

func setup(t *testing.T) (*DynamoDBRepository[TestEntity], func()) {
	// Clear table before each test
	ctx := context.Background()

	scanOutput, err := testDynamoClient.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(config.TableName),
	})
	if err != nil {
		t.Fatalf("failed to scan table for clearing: %s", err)
	}

	if len(scanOutput.Items) > 0 {
		writeRequests := make([]types.WriteRequest, len(scanOutput.Items))
		for i, item := range scanOutput.Items {
			writeRequests[i] = types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{Key: map[string]types.AttributeValue{
					"pk": item["pk"],
					"sk": item["sk"],
				}},
			}
		}

		_, err = testDynamoClient.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				config.TableName: writeRequests,
			},
		})
		if err != nil {
			t.Fatalf("failed to batch delete items during table clearing: %s", err)
		}
	}

	return testRepo, func() { /* no-op teardown for individual tests */ }
}

func TestDynamoDBRepository_FindById(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntity := TestEntity{ID: "1", Name: "test"}
	err := repo.Save(testEntity, partitionKey)
	assert.NoError(t, err)

	foundEntity, err := repo.FindById("1", partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, testEntity.ID, foundEntity.ID)
	assert.Equal(t, testEntity.Name, foundEntity.Name)
}

func TestDynamoDBRepository_FindAllById(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntity1 := TestEntity{ID: "1", Name: "test1"}
	testEntity2 := TestEntity{ID: "2", Name: "test2"}
	err := repo.Save(testEntity1, partitionKey)
	assert.NoError(t, err)
	err = repo.Save(testEntity2, partitionKey)
	assert.NoError(t, err)

	foundEntities, err := repo.FindAllById([]string{"1", "2"}, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, foundEntities, 2)

	// Create maps for easy lookup
	entityMap := make(map[string]TestEntity)
	for _, e := range foundEntities {
		entityMap[e.ID] = e
	}

	assert.Equal(t, testEntity1.Name, entityMap["1"].Name)
	assert.Equal(t, testEntity2.Name, entityMap["2"].Name)
}

func TestDynamoDBRepository_SaveAll(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntities := []TestEntity{
		{ID: "3", Name: "test3"},
		{ID: "4", Name: "test4"},
	}
	err := repo.SaveAll(testEntities, partitionKey)
	assert.NoError(t, err)

	foundEntity1, err := repo.FindById("3", partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, testEntities[0].Name, foundEntity1.Name)

	foundEntity2, err := repo.FindById("4", partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, testEntities[1].Name, foundEntity2.Name)
}

func TestDynamoDBRepository_Update(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntity := TestEntity{ID: "1", Name: "initial"}
	err := repo.Save(testEntity, partitionKey)
	assert.NoError(t, err)

	updatedEntity := TestEntity{ID: "1", Name: "updated"}
	err = repo.Update(updatedEntity, partitionKey)
	assert.NoError(t, err)

	foundUpdatedEntity, err := repo.FindById("1", partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, updatedEntity.Name, foundUpdatedEntity.Name)
}

func TestDynamoDBRepository_FindOneBy(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntity := TestEntity{ID: "5", Name: "findOneTest"}
	err := repo.Save(testEntity, partitionKey)
	assert.NoError(t, err)

	foundEntity, err := repo.FindOneBy("Name", "findOneTest", partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, testEntity.ID, foundEntity.ID)
	assert.Equal(t, testEntity.Name, foundEntity.Name)

	_, err = repo.FindOneBy("Name", "nonExistent", partitionKey)
	assert.Error(t, err)
}

func TestDynamoDBRepository_FindOneByFilters(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntity := TestEntity{ID: "6", Name: "filterTest", Value: 10}
	err := repo.Save(testEntity, partitionKey)
	assert.NoError(t, err)

	filters := map[string]interface{}{
		"Name":  "filterTest",
		"Value": 10,
	}
	foundEntity, err := repo.FindOneByFilters(filters, partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, testEntity.ID, foundEntity.ID)
	assert.Equal(t, testEntity.Name, foundEntity.Name)
	assert.Equal(t, testEntity.Value, foundEntity.Value)

	filters = map[string]interface{}{
		"Name":  "filterTest",
		"Value": 99,
	}
	_, err = repo.FindOneByFilters(filters, partitionKey)
	assert.Error(t, err)
}

func TestDynamoDBRepository_FindBy(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntity1 := TestEntity{ID: "7", Name: "findByTest", Value: 1}
	testEntity2 := TestEntity{ID: "8", Name: "findByTest", Value: 2}
	testEntity3 := TestEntity{ID: "9", Name: "another", Value: 1}
	err := repo.Save(testEntity1, partitionKey)
	assert.NoError(t, err)
	err = repo.Save(testEntity2, partitionKey)
	assert.NoError(t, err)
	err = repo.Save(testEntity3, partitionKey)
	assert.NoError(t, err)

	foundEntities, err := repo.FindBy("Name", "findByTest", partitionKey)
	assert.NoError(t, err)
	assert.Len(t, foundEntities, 2)

	// Create maps for easy lookup
	entityMap := make(map[string]TestEntity)
	for _, e := range foundEntities {
		entityMap[e.ID] = e
	}

	assert.Equal(t, testEntity1.Name, entityMap["7"].Name)
	assert.Equal(t, testEntity2.Name, entityMap["8"].Name)
}

func TestDynamoDBRepository_FindByFilters(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntity1 := TestEntity{ID: "10", Name: "filterTest1", Value: 100}
	testEntity2 := TestEntity{ID: "11", Name: "filterTest2", Value: 100}
	testEntity3 := TestEntity{ID: "12", Name: "filterTest1", Value: 200}
	err := repo.Save(testEntity1, partitionKey)
	assert.NoError(t, err)
	err = repo.Save(testEntity2, partitionKey)
	assert.NoError(t, err)
	err = repo.Save(testEntity3, partitionKey)
	assert.NoError(t, err)

	filters := map[string]interface{}{
		"Name":  "filterTest1",
		"Value": 100,
	}
	foundEntities, err := repo.FindByFilters(filters, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, foundEntities, 1)
	assert.Equal(t, testEntity1.ID, foundEntities[0].ID)

	filters = map[string]interface{}{
		"Value": 100,
	}
	foundEntities, err = repo.FindByFilters(filters, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, foundEntities, 2)
}

func TestDynamoDBRepository_FindAll(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	testEntity1 := TestEntity{ID: "13", Name: "all1", Value: 1}
	testEntity2 := TestEntity{ID: "14", Name: "all2", Value: 2}
	err := repo.Save(testEntity1, partitionKey)
	assert.NoError(t, err)
	err = repo.Save(testEntity2, partitionKey)
	assert.NoError(t, err)

	foundEntities, err := repo.FindAll(partitionKey)
	assert.NoError(t, err)
	assert.Len(t, foundEntities, 2)

	// Create maps for easy lookup
	entityMap := make(map[string]TestEntity)
	for _, e := range foundEntities {
		entityMap[e.ID] = e
	}

	assert.Equal(t, testEntity1.Name, entityMap["13"].Name)
	assert.Equal(t, testEntity2.Name, entityMap["14"].Name)
}

func TestDynamoDBRepository_FindAllPaginated(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	// Save 5 entities for pagination testing
	for i := 0; i < 5; i++ {
		err := repo.Save(TestEntity{ID: "paginated" + string(rune('A'+i)), Name: "paginated", Value: i}, partitionKey)
		assert.NoError(t, err)
	}

	// Test first page
	pageRequest1 := PageRequest{Page: 1, Size: 2}
	pageResponse1, err := repo.FindAllPaginated(pageRequest1, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponse1.Contents, 2)
	assert.Equal(t, 5, pageResponse1.TotalElements)
	assert.Equal(t, 3, pageResponse1.TotalPages)

	// Test second page
	pageRequest2 := PageRequest{Page: 2, Size: 2}
	pageResponse2, err := repo.FindAllPaginated(pageRequest2, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponse2.Contents, 2)
	assert.Equal(t, 5, pageResponse2.TotalElements)
	assert.Equal(t, 3, pageResponse2.TotalPages)

	// Test last page (with one item)
	pageRequest3 := PageRequest{Page: 3, Size: 2}
	pageResponse3, err := repo.FindAllPaginated(pageRequest3, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponse3.Contents, 1)
	assert.Equal(t, 5, pageResponse3.TotalElements)
	assert.Equal(t, 3, pageResponse3.TotalPages)

	// Test page size of -1 (all items)
	pageRequestAll := PageRequest{Page: 1, Size: -1}
	pageResponseAll, err := repo.FindAllPaginated(pageRequestAll, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponseAll.Contents, 5)
	assert.Equal(t, 5, pageResponseAll.TotalElements)
	assert.Equal(t, 1, pageResponseAll.TotalPages)
}

func TestDynamoDBRepository_FindAllPaginated_SingleResult(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	_ = repo.Save(TestEntity{ID: "single", Name: "filtered", Value: 10}, partitionKey)

	pageRequest := PageRequest{Page: 1, Size: 50}
	pageResponse, err := repo.FindAllPaginated(pageRequest, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponse.Contents, 1)
	assert.Equal(t, 1, pageResponse.TotalElements)
	assert.Equal(t, 1, pageResponse.TotalPages)
}

func TestDynamoDBRepository_FindByPaginated(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	// Save entities for pagination with filters
	_ = repo.Save(TestEntity{ID: "fp1", Name: "filtered", Value: 10}, partitionKey)
	_ = repo.Save(TestEntity{ID: "fp2", Name: "filtered", Value: 20}, partitionKey)
	_ = repo.Save(TestEntity{ID: "fp3", Name: "other", Value: 10}, partitionKey)
	_ = repo.Save(TestEntity{ID: "fp4", Name: "filtered", Value: 30}, partitionKey)

	filters := map[string]interface{}{
		"Name": "filtered",
	}

	// Test first page with filter
	pageRequest1 := PageRequest{Page: 1, Size: 2}
	pageResponse1, err := repo.FindByPaginated(pageRequest1, filters, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponse1.Contents, 2)
	assert.Equal(t, 3, pageResponse1.TotalElements)
	assert.Equal(t, 2, pageResponse1.TotalPages)

	// Test second page with filter
	pageRequest2 := PageRequest{Page: 2, Size: 2}
	pageResponse2, err := repo.FindByPaginated(pageRequest2, filters, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponse2.Contents, 1)
	assert.Equal(t, 3, pageResponse2.TotalElements)
	assert.Equal(t, 2, pageResponse2.TotalPages)

	// Test page size of -1 (all items)
	pageRequestAll := PageRequest{Page: 1, Size: -1}
	pageResponseAll, err := repo.FindByPaginated(pageRequestAll, filters, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponseAll.Contents, 3)
	assert.Equal(t, 3, pageResponseAll.TotalElements)
	assert.Equal(t, 1, pageResponseAll.TotalPages)
}

func TestDynamoDBRepository_FindByPaginated_SingleResult(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	_ = repo.Save(TestEntity{ID: "single", Name: "filtered", Value: 10}, partitionKey)

	filters := map[string]interface{}{
		"Name": "filtered",
	}

	pageRequest := PageRequest{Page: 1, Size: 50}
	pageResponse, err := repo.FindByPaginated(pageRequest, filters, partitionKey)
	assert.NoError(t, err)
	assert.Len(t, pageResponse.Contents, 1)
	assert.Equal(t, 1, pageResponse.TotalElements)
	assert.Equal(t, 1, pageResponse.TotalPages)
}

func TestDynamoDBRepository_CountBy(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	_ = repo.Save(TestEntity{ID: "cb1", Name: "countTest", Value: 1}, partitionKey)
	_ = repo.Save(TestEntity{ID: "cb2", Name: "countTest", Value: 2}, partitionKey)
	_ = repo.Save(TestEntity{ID: "cb3", Name: "another", Value: 1}, partitionKey)

	count, err := repo.CountBy("Name", "countTest", partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)

	count, err = repo.CountBy("Value", 1, partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)

	count, err = repo.CountBy("Name", "nonExistent", partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestDynamoDBRepository_CountByFilters(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	_ = repo.Save(TestEntity{ID: "cbf1", Name: "filterCount", Value: 10}, partitionKey)
	_ = repo.Save(TestEntity{ID: "cbf2", Name: "filterCount", Value: 20}, partitionKey)
	_ = repo.Save(TestEntity{ID: "cbf3", Name: "other", Value: 10}, partitionKey)

	filters := map[string]interface{}{
		"Name":  "filterCount",
		"Value": 10,
	}
	count, err := repo.CountByFilters(filters, partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	filters = map[string]interface{}{
		"Value": 10,
	}
	count, err = repo.CountByFilters(filters, partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)

	filters = map[string]interface{}{
		"Name": "nonExistent",
	}
	count, err = repo.CountByFilters(filters, partitionKey)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestDynamoDBRepository_ExistsBy(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	_ = repo.Save(TestEntity{ID: "eb1", Name: "existsTest", Value: 1}, partitionKey)

	exists, err := repo.ExistsBy("Name", "existsTest", partitionKey)
	assert.NoError(t, err)
	assert.True(t, exists)

	exists, err = repo.ExistsBy("Name", "nonExistent", partitionKey)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestDynamoDBRepository_ExistsByFilters(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition"
	_ = repo.Save(TestEntity{ID: "ebf1", Name: "filterExists", Value: 10}, partitionKey)

	filters := map[string]interface{}{
		"Name":  "filterExists",
		"Value": 10,
	}
	exists, err := repo.ExistsByFilters(filters, partitionKey)
	assert.NoError(t, err)
	assert.True(t, exists)

	filters = map[string]interface{}{
		"Name":  "filterExists",
		"Value": 99,
	}
	exists, err = repo.ExistsByFilters(filters, partitionKey)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestDynamoDBRepository_DeleteAll(t *testing.T) {
	repo, teardown := setup(t)
	defer teardown()

	partitionKey := "test-partition-for-delete"
	testEntity1 := TestEntity{ID: "del1", Name: "delete_me"}
	testEntity2 := TestEntity{ID: "del2", Name: "delete_me_too"}
	testEntity3 := TestEntity{ID: "del3", Name: "keep_me"}
	err := repo.Save(testEntity1, partitionKey)
	assert.NoError(t, err)
	err = repo.Save(testEntity2, partitionKey)
	assert.NoError(t, err)
	err = repo.Save(testEntity3, partitionKey)
	assert.NoError(t, err)

	// Confirm items are there
	found, err := repo.FindAll(partitionKey)
	assert.NoError(t, err)
	assert.Len(t, found, 3)

	// Delete a subset
	err = repo.DeleteAll([]string{"del1", "del2"}, partitionKey)
	assert.NoError(t, err)

	// Confirm items are gone
	foundAfterDelete, err := repo.FindAll(partitionKey)
	assert.NoError(t, err)
	assert.Len(t, foundAfterDelete, 1)
	assert.Equal(t, "del3", foundAfterDelete[0].ID)
}
