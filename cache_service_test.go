package ginboot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDynamoDBClient is a mock implementation of DynamoDBAPI
type MockDynamoDBClient struct {
	mock.Mock
}

func (m *MockDynamoDBClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.GetItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.PutItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.DeleteItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.QueryOutput), args.Error(1)
}

func (m *MockDynamoDBClient) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.ScanOutput), args.Error(1)
}

func (m *MockDynamoDBClient) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.BatchWriteItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.BatchGetItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) UpdateTimeToLive(ctx context.Context, params *dynamodb.UpdateTimeToLiveInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateTimeToLiveOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.UpdateTimeToLiveOutput), args.Error(1)
}

func (m *MockDynamoDBClient) CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.CreateTableOutput), args.Error(1)
}

func (m *MockDynamoDBClient) DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.DescribeTableOutput), args.Error(1)
}

func (m *MockDynamoDBClient) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.TransactWriteItemsOutput), args.Error(1)
}

func TestCacheService_Set(t *testing.T) {
	mockClient := new(MockDynamoDBClient)

	// Expect DescribeTable for Repo initialization (called twice)
	mockClient.On("DescribeTable", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.DescribeTableOutput{
		Table: &types.TableDescription{
			TableStatus: types.TableStatusActive,
		},
	}, nil).Twice()

	service := NewDynamoDBCacheService(mockClient)
	ctx := context.Background()
	key := "key1"
	data := []byte("value")
	tags := []string{"tag1"}

	// Expect GetItem (Read before Write) for CacheEntry and TagEntry (called twice)
	// Return nil item to simulate new creation
	mockClient.On("GetItem", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
		Item: nil,
	}, nil).Twice()

	// Expect PutItem for CacheEntry and TagEntry
	mockClient.On("PutItem", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.PutItemOutput{}, nil).Twice()

	err := service.Set(ctx, key, data, tags, time.Minute)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestCacheService_Get_Hit(t *testing.T) {
	mockClient := new(MockDynamoDBClient)

	// Expect DescribeTable for Repo initialization
	mockClient.On("DescribeTable", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.DescribeTableOutput{
		Table: &types.TableDescription{
			TableStatus: types.TableStatusActive,
		},
	}, nil).Twice()

	service := NewDynamoDBCacheService(mockClient)
	ctx := context.Background()
	key := "key1"
	data := []byte("value")

	cacheEntry := CacheEntry{
		PK:   CachePartitionPrefix + key,
		SK:   CacheSortKey,
		Data: data,
		TTL:  time.Now().Add(time.Minute).Unix(),
	}
	// Wrap in DynamoDBItem as Repository does
	ceJson, _ := json.Marshal(cacheEntry)
	ddbItem := DynamoDBItem{
		PK:   "CacheEntry#" + key,
		SK:   CacheSortKey,
		Data: string(ceJson),
	}
	itemMap, _ := attributevalue.MarshalMap(ddbItem)

	mockClient.On("GetItem", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
		Item: itemMap,
	}, nil)

	result, err := service.Get(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, data, result)
	mockClient.AssertExpectations(t)
}

func TestCacheService_Get_Miss(t *testing.T) {
	mockClient := new(MockDynamoDBClient)

	// Expect DescribeTable for Repo initialization
	mockClient.On("DescribeTable", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.DescribeTableOutput{
		Table: &types.TableDescription{
			TableStatus: types.TableStatusActive,
		},
	}, nil).Twice()

	service := NewDynamoDBCacheService(mockClient)
	ctx := context.Background()
	key := "key1"

	mockClient.On("GetItem", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
		Item: nil,
	}, nil)

	result, err := service.Get(ctx, key)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCacheService_Get_Expired(t *testing.T) {
	mockClient := new(MockDynamoDBClient)

	// Expect DescribeTable for Repo initialization
	mockClient.On("DescribeTable", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.DescribeTableOutput{
		Table: &types.TableDescription{
			TableStatus: types.TableStatusActive,
		},
	}, nil).Twice()

	service := NewDynamoDBCacheService(mockClient)
	ctx := context.Background()
	key := "key1"

	cacheEntry := CacheEntry{
		PK:   CachePartitionPrefix + key,
		SK:   CacheSortKey,
		Data: []byte("value"),
		TTL:  time.Now().Add(-time.Minute).Unix(), // Expired
	}
	ceJson, _ := json.Marshal(cacheEntry)
	ddbItem := DynamoDBItem{
		PK:   "CacheEntry#" + key,
		SK:   CacheSortKey,
		Data: string(ceJson),
	}
	itemMap, _ := attributevalue.MarshalMap(ddbItem)

	mockClient.On("GetItem", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
		Item: itemMap,
	}, nil)

	result, err := service.Get(ctx, key)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCacheService_Invalidate(t *testing.T) {
	mockClient := new(MockDynamoDBClient)

	// Expect DescribeTable for Repo initialization
	mockClient.On("DescribeTable", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.DescribeTableOutput{
		Table: &types.TableDescription{
			TableStatus: types.TableStatusActive,
		},
	}, nil).Twice()

	service := NewDynamoDBCacheService(mockClient)
	ctx := context.Background()

	// Mock Query for tag
	tagEntry := TagEntry{
		PK: TagPartitionPrefix + "tag1",
		SK: "key1", // SK stores the cache key
	}
	teJson, _ := json.Marshal(tagEntry)
	ddbItem := DynamoDBItem{
		PK:   "TagEntry#tag1",
		SK:   "key1",
		Data: string(teJson),
	}
	tagItemMap, _ := attributevalue.MarshalMap(ddbItem)

	mockClient.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.QueryOutput{
		Items: []map[string]types.AttributeValue{tagItemMap},
	}, nil)

	// Mock DeleteItem for Cache Entry
	mockClient.On("DeleteItem", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.DeleteItemOutput{}, nil)

	// Mock BatchWriteItem for deletion of tag entries
	mockClient.On("BatchWriteItem", mock.Anything, mock.Anything, mock.Anything).Return(&dynamodb.BatchWriteItemOutput{}, nil)

	err := service.Invalidate(ctx, "tag1")
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}
