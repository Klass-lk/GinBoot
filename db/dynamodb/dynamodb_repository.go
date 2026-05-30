package dynamodb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/klass-lk/ginboot"
)

type DynamoDBItem struct {
	PK        string `dynamodbav:"pk"`
	SK        string `dynamodbav:"sk"`
	ID        string `dynamodbav:"id"` // Added for GSI
	Data      string `dynamodbav:"data"`
	CreatedAt int64  `dynamodbav:"createdAt"`
	UpdatedAt int64  `dynamodbav:"updatedAt"`
	Version   int64  `dynamodbav:"version"`
	TTL       int64  `dynamodbav:"ttl,omitempty"`
}

// DynamoDBAPI defines the interface for DynamoDB operations
type DynamoDBAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	UpdateTimeToLive(ctx context.Context, params *dynamodb.UpdateTimeToLiveInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateTimeToLiveOutput, error)
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

type DynamoDBRepository[T any] struct {
	client DynamoDBAPI
	ttl    time.Duration
}

func NewDynamoDBRepository[T any](client DynamoDBAPI) *DynamoDBRepository[T] {
	repo := &DynamoDBRepository[T]{
		client: client,
	}

	if dynamoConfig.SkipTableCreation {
		return repo
	}

	// Check if table exists, if not, create it
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := repo.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(dynamoConfig.TableName),
	})

	if err != nil {
		var notFoundEx *types.ResourceNotFoundException
		if errors.As(err, &notFoundEx) {
			log.Printf("DynamoDB table %s does not exist, creating it...", dynamoConfig.TableName)
			err = repo.CreateTable(ctx)
			if err != nil {
				log.Fatalf("Failed to create DynamoDB table %s: %v", dynamoConfig.TableName, err)
			}
			log.Printf("DynamoDB table %s created successfully.", dynamoConfig.TableName)
		} else {
			log.Fatalf("Failed to describe DynamoDB table %s: %v", dynamoConfig.TableName, err)
		}
	}

	return repo
}

func NewDynamoDBRepositoryWithTTL[T any](client DynamoDBAPI, ttl time.Duration) *DynamoDBRepository[T] {
	repo := &DynamoDBRepository[T]{
		client: client,
		ttl:    ttl,
	}

	if dynamoConfig.SkipTableCreation {
		return repo
	}

	// Check if table exists, if not, create it
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := repo.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(dynamoConfig.TableName),
	})

	if err != nil {
		var notFoundEx *types.ResourceNotFoundException
		if errors.As(err, &notFoundEx) {
			log.Printf("DynamoDB table %s does not exist, creating it...", dynamoConfig.TableName)
			err = repo.CreateTable(ctx)
			if err != nil {
				log.Fatalf("Failed to create DynamoDB table %s: %v", dynamoConfig.TableName, err)
			}
			log.Printf("DynamoDB table %s created successfully.", dynamoConfig.TableName)
		} else {
			log.Fatalf("Failed to describe DynamoDB table %s: %v", dynamoConfig.TableName, err)
		}
	}

	if repo.ttl > 0 {
		repo.EnableTTL(ctx)
	}

	return repo
}

func (r *DynamoDBRepository[T]) GetClient() DynamoDBAPI {
	return r.client
}

func (r *DynamoDBRepository[T]) findById(pk string, sk string) (map[string]types.AttributeValue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key, err := attributevalue.MarshalMap(map[string]string{
		"pk": pk,
		"sk": sk,
	})
	if err != nil {
		return nil, err
	}

	input := &dynamodb.GetItemInput{
		TableName: aws.String(dynamoConfig.TableName),
		Key:       key,
	}

	output, err := r.client.GetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if output.Item == nil {
		return nil, errors.New("item not found")
	}

	return output.Item, nil
}

func (r *DynamoDBRepository[T]) FindById(entityId string, partitionKey string) (T, error) {
	var result T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK
	sk := entityId

	item, err := r.findById(pk, sk)
	if err != nil {
		return result, err
	}

	err = UnmarshalLegacyOrNative(item, &result)
	return result, err
}

func (r *DynamoDBRepository[T]) FindAllById(ids []string, partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if len(ids) == 0 {
		return []T{}, nil
	}

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	keys := make([]map[string]types.AttributeValue, len(ids))
	for i, id := range ids {
		key, err := attributevalue.MarshalMap(map[string]string{
			"pk": pk,
			"sk": id,
		})
		if err != nil {
			return nil, err
		}
		keys[i] = key
	}

	input := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			dynamoConfig.TableName: {
				Keys:           keys,
				ConsistentRead: aws.Bool(true),
			},
		},
	}

	output, err := r.client.BatchGetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	type sortedItem struct {
		CreatedAt int64
		Result    T
	}
	var sortedItems []sortedItem
	for _, item := range output.Responses[dynamoConfig.TableName] {
		var result T
		err = UnmarshalLegacyOrNative(item, &result)
		if err != nil {
			return nil, err
		}
		
		var createdAt int64
		if cAttr, ok := item["createdAt"].(*types.AttributeValueMemberN); ok {
			createdAt, _ = strconv.ParseInt(cAttr.Value, 10, 64)
		}
		
		sortedItems = append(sortedItems, sortedItem{
			CreatedAt: createdAt,
			Result:    result,
		})
	}

	sort.Slice(sortedItems, func(i, j int) bool {
		return sortedItems[i].CreatedAt > sortedItems[j].CreatedAt
	})

	var results []T
	for _, si := range sortedItems {
		results = append(results, si.Result)
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) Save(doc T, partitionKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UnixMilli()

	pk := r.getPK(doc) + "#" + partitionKey // Composite PK
	id, err := r.getGinbootId(doc)
	if err != nil {
		return err
	}
	sk := id // SK is the entity id

	// Get current version and increment it
	var version int64
	var createdAt int64

	// Try to find existing item to get version
	item, err := r.findById(pk, sk)
	if err == nil {
		// Item exists, get its version and createdAt
		if vAttr, ok := item["version"].(*types.AttributeValueMemberN); ok {
			version, _ = strconv.ParseInt(vAttr.Value, 10, 64)
		}
		if cAttr, ok := item["createdAt"].(*types.AttributeValueMemberN); ok {
			createdAt, _ = strconv.ParseInt(cAttr.Value, 10, 64)
		}
	}

	// Marshal doc natively
	av, err := attributevalue.MarshalMap(doc)
	if err != nil {
		return err
	}
    
    if createdAt == 0 {
		createdAt = now
	}
    newVersion := version + 1
    
    av["pk"] = &types.AttributeValueMemberS{Value: pk}
    av["sk"] = &types.AttributeValueMemberS{Value: sk}
    av["id"] = &types.AttributeValueMemberS{Value: id}
    av["createdAt"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(createdAt, 10)}
    av["updatedAt"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(now, 10)}
    av["version"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(newVersion, 10)}
    if r.ttl > 0 {
        ttlVal := time.Now().Add(r.ttl).Unix()
        av["ttl"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(ttlVal, 10)}
    }

	input := &dynamodb.PutItemInput{
		TableName: aws.String(dynamoConfig.TableName),
		Item:      av,
	}

	_, err = r.client.PutItem(ctx, input)
	return err
}

func (r *DynamoDBRepository[T]) SaveOrUpdate(doc T, partitionKey string) error {
	return r.Save(doc, partitionKey)
}

func (r *DynamoDBRepository[T]) SaveAll(docs []T, partitionKey string) error {
	if len(docs) == 0 {
		return nil
	}

	writeRequests := make([]types.WriteRequest, len(docs))
	for i, doc := range docs {
		now := time.Now().UnixMilli()

		pk := r.getPK(doc) + "#" + partitionKey // Composite PK
		id, err := r.getGinbootId(doc)
		if err != nil {
			return err
		}
		sk := id // SK is the entity id

		var version int64
		var createdAt int64

		item, err := r.findById(pk, sk)
		if err == nil {
			if vAttr, ok := item["version"].(*types.AttributeValueMemberN); ok {
				version, _ = strconv.ParseInt(vAttr.Value, 10, 64)
			}
			if cAttr, ok := item["createdAt"].(*types.AttributeValueMemberN); ok {
				createdAt, _ = strconv.ParseInt(cAttr.Value, 10, 64)
			}
		}

		av, err := attributevalue.MarshalMap(doc)
		if err != nil {
			return err
		}
		
		if createdAt == 0 {
			createdAt = now
		}
		newVersion := version + 1
		
		av["pk"] = &types.AttributeValueMemberS{Value: pk}
		av["sk"] = &types.AttributeValueMemberS{Value: sk}
		av["id"] = &types.AttributeValueMemberS{Value: id}
		av["createdAt"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(createdAt, 10)}
		av["updatedAt"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(now, 10)}
		av["version"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(newVersion, 10)}
		if r.ttl > 0 {
			ttlVal := time.Now().Add(r.ttl).Unix()
			av["ttl"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(ttlVal, 10)}
		}

		writeRequests[i] = types.WriteRequest{
			PutRequest: &types.PutRequest{Item: av},
		}
	}

	for i := 0; i < len(writeRequests); i += 25 {
		end := i + 25
		if end > len(writeRequests) {
			end = len(writeRequests)
		}

		batchWriteInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				dynamoConfig.TableName: writeRequests[i:end],
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := r.client.BatchWriteItem(ctx, batchWriteInput)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *DynamoDBRepository[T]) Update(doc T, partitionKey string) error {
	return r.Save(doc, partitionKey)
}

func (r *DynamoDBRepository[T]) Delete(id string, partitionKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK
	sk := id

	key, err := attributevalue.MarshalMap(map[string]string{
		"pk": pk,
		"sk": sk,
	})
	if err != nil {
		return err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(dynamoConfig.TableName),
		Key:       key,
	}

	_, err = r.client.DeleteItem(ctx, input)
	return err
}

func (r *DynamoDBRepository[T]) FindOneBy(field string, value interface{}, partitionKey string) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return result, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return result, err
				}

			val := reflect.ValueOf(temp)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			fieldValue := val.FieldByName(field).Interface()
			if fieldValue == value {
				return temp, nil
			}
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return result, errors.New("item not found")
}

func (r *DynamoDBRepository[T]) FindOneByFilters(filters map[string]interface{}, partitionKey string) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return result, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return result, err
				}

			match := true
			val := reflect.ValueOf(temp)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			for field, value := range filters {
				fieldValue := val.FieldByName(field).Interface()
				if fieldValue != value {
					match = false
					break
				}
			}

			if match {
				return temp, nil
			}
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return result, errors.New("item not found")
}

func (r *DynamoDBRepository[T]) FindBy(field string, value interface{}, partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false),
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return nil, err
				}

			val := reflect.ValueOf(temp)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			fieldValue := val.FieldByName(field).Interface()

			match := true
			if opMap, ok := value.(map[string]interface{}); ok {
				for op, opValue := range opMap {
					switch op {
					case "$gte":
						if !reflect.DeepEqual(fieldValue, opValue) && !((fieldValue.(int64)) >= (opValue.(time.Time)).UnixMilli()) {
							match = false
						}
					case "$lt":
						if !reflect.DeepEqual(fieldValue, opValue) && !((fieldValue.(int64)) < (opValue.(time.Time)).UnixMilli()) {
							match = false
						}
					default:
						match = false
					}
				}
			} else {
				if !reflect.DeepEqual(fieldValue, value) {
					match = false
				}
			}

			if match {
				results = append(results, temp)
			}
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) FindByFilters(filters map[string]interface{}, partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false),
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return nil, err
				}

			match := true
			val := reflect.ValueOf(temp)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			for field, filterValue := range filters {
				fieldValue := val.FieldByName(field).Interface()

				if opMap, ok := filterValue.(map[string]interface{}); ok {
					for op, opValue := range opMap {
						switch op {
						case "$gte":
							if !reflect.DeepEqual(fieldValue, opValue) && !((fieldValue.(int64)) >= (opValue.(time.Time)).UnixMilli()) {
								match = false
							}
						case "$lt":
							if !reflect.DeepEqual(fieldValue, opValue) && !((fieldValue.(int64)) < (opValue.(time.Time)).UnixMilli()) {
								match = false
							}
						default:
							match = false
						}
					}
				} else {
					if !reflect.DeepEqual(fieldValue, filterValue) {
						match = false
					}
				}

				if !match {
					break
				}
			}

			if match {
				results = append(results, temp)
			}
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) FindAll(partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false),
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return nil, err
				}
			results = append(results, temp)
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) FindAllPaginated(pageRequest ginboot.PageRequest, partitionKey string) (ginboot.PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false),
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return ginboot.PageResponse[T]{}, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return ginboot.PageResponse[T]{}, err
				}
			results = append(results, temp)
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	if pageRequest.Size == -1 {
		return ginboot.PageResponse[T]{
			Contents:         results,
			NumberOfElements: len(results),
			Pageable:         pageRequest,
			TotalElements:    len(results),
			TotalPages:       1,
		}, nil
	}

	totalElements := len(results)
	totalPages := (totalElements + pageRequest.Size - 1) / pageRequest.Size

	start := (pageRequest.Page - 1) * pageRequest.Size
	end := start + pageRequest.Size
	if start > totalElements {
		start = totalElements
	}
	if end > totalElements {
		end = totalElements
	}

	var pagedResults []T
	if start >= totalElements {
		pagedResults = []T{}
	} else {
		if end > totalElements {
			end = totalElements
		}
		pagedResults = results[start:end]
	}

	return ginboot.PageResponse[T]{
		Contents:         pagedResults,
		NumberOfElements: len(pagedResults),
		Pageable:         pageRequest,
		TotalElements:    totalElements,
		TotalPages:       totalPages,
	}, nil
}

func (r *DynamoDBRepository[T]) FindByPaginated(pageRequest ginboot.PageRequest, filters map[string]interface{}, partitionKey string) (ginboot.PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false),
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return ginboot.PageResponse[T]{}, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return ginboot.PageResponse[T]{}, err
				}

			match := true
			val := reflect.ValueOf(temp)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			for field, value := range filters {
				fieldValue := val.FieldByName(field).Interface()
				if fieldValue != value {
					match = false
					break
				}
			}

			if match {
				results = append(results, temp)
			}
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	if pageRequest.Size == -1 {
		return ginboot.PageResponse[T]{
			Contents:         results,
			NumberOfElements: len(results),
			Pageable:         pageRequest,
			TotalElements:    len(results),
			TotalPages:       1,
		}, nil
	}

	totalElements := len(results)
	totalPages := (totalElements + pageRequest.Size - 1) / pageRequest.Size

	start := (pageRequest.Page - 1) * pageRequest.Size
	end := start + pageRequest.Size

	var pagedResults []T
	if start >= totalElements {
		pagedResults = []T{}
	} else {
		if end > totalElements {
			end = totalElements
		}
		pagedResults = results[start:end]
	}

	return ginboot.PageResponse[T]{
		Contents:         pagedResults,
		NumberOfElements: len(pagedResults),
		Pageable:         pageRequest,
		TotalElements:    totalElements,
		TotalPages:       totalPages,
	}, nil
}

func (r *DynamoDBRepository[T]) CountBy(field string, value interface{}, partitionKey string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	var count int64
	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return 0, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return 0, err
				}

			val := reflect.ValueOf(temp)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			fieldValue := val.FieldByName(field).Interface()

			match := true
			if opMap, ok := value.(map[string]interface{}); ok {
				for op, opValue := range opMap {
					switch op {
					case "$gte":
						if !reflect.DeepEqual(fieldValue, opValue) && !((fieldValue.(int64)) >= (opValue.(time.Time)).UnixMilli()) {
							match = false
						}
					case "$lt":
						if !reflect.DeepEqual(fieldValue, opValue) && !((fieldValue.(int64)) < (opValue.(time.Time)).UnixMilli()) {
							match = false
						}
					default:
						match = false
					}
				}
			} else {
				if !reflect.DeepEqual(fieldValue, value) {
					match = false
				}
			}

			if match {
				count++
			}
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return count, nil
}

func (r *DynamoDBRepository[T]) CountByFilters(filters map[string]interface{}, partitionKey string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	var count int64
	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return 0, err
		}

		for _, item := range output.Items {
			var temp T
			err = UnmarshalLegacyOrNative(item, &temp)
				if err != nil {
					return 0, err
				}

			match := true
			val := reflect.ValueOf(temp)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			for field, filterValue := range filters {
				fieldValue := val.FieldByName(field).Interface()

				if opMap, ok := filterValue.(map[string]interface{}); ok {
					for op, opValue := range opMap {
						switch op {
						case "$gte":
							if !reflect.DeepEqual(fieldValue, opValue) && !((fieldValue.(int64)) >= (opValue.(time.Time)).UnixMilli()) {
								match = false
							}
						case "$lt":
							if !reflect.DeepEqual(fieldValue, opValue) && !((fieldValue.(int64)) < (opValue.(time.Time)).UnixMilli()) {
								match = false
							}
						default:
							match = false
						}
					}
				} else {
					if !reflect.DeepEqual(fieldValue, filterValue) {
						match = false
					}
				}

				if !match {
					break
				}
			}

			if match {
				count++
			}
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return count, nil
}

func (r *DynamoDBRepository[T]) ExistsBy(field string, value interface{}, partitionKey string) (bool, error) {
	count, err := r.CountBy(field, value, partitionKey)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *DynamoDBRepository[T]) ExistsByFilters(filters map[string]interface{}, partitionKey string) (bool, error) {
	count, err := r.CountByFilters(filters, partitionKey)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *DynamoDBRepository[T]) DeleteAll(ids []string, partitionKey string) error {
	if len(ids) == 0 {
		return nil
	}

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	writeRequests := make([]types.WriteRequest, len(ids))
	for i, id := range ids {
		sk := id
		key, err := attributevalue.MarshalMap(map[string]string{
			"pk": pk,
			"sk": sk,
		})
		if err != nil {
			return err
		}
		writeRequests[i] = types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{Key: key},
		}
	}

	for i := 0; i < len(writeRequests); i += 25 {
		end := i + 25
		if end > len(writeRequests) {
			end = len(writeRequests)
		}

		batchWriteInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				dynamoConfig.TableName: writeRequests[i:end],
			},
		}
		_, err := r.client.BatchWriteItem(context.TODO(), batchWriteInput)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *DynamoDBRepository[T]) DeleteBy(field string, value interface{}, partitionKey string) error {
	items, err := r.FindBy(field, value, partitionKey)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		return nil
	}

	ids := make([]string, len(items))
	for i, item := range items {
		id, err := r.getGinbootId(item)
		if err != nil {
			return err
		}
		ids[i] = id
	}

	return r.DeleteAll(ids, partitionKey)
}

func (r *DynamoDBRepository[T]) DeleteByFilters(filters map[string]interface{}, partitionKey string) error {
	items, err := r.FindByFilters(filters, partitionKey)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		return nil
	}

	ids := make([]string, len(items))
	for i, item := range items {
		id, err := r.getGinbootId(item)
		if err != nil {
			return err
		}
		ids[i] = id
	}

	return r.DeleteAll(ids, partitionKey)
}


// UnmarshalLegacyOrNative handles backward compatibility for legacy JSON 'data' strings
func UnmarshalLegacyOrNative[T any](item map[string]types.AttributeValue, result *T) error {
	if dataAttr, ok := item["data"]; ok {
		if strVal, ok := dataAttr.(*types.AttributeValueMemberS); ok && strVal.Value != "" {
			return json.Unmarshal([]byte(strVal.Value), result)
		}
	}
	return attributevalue.UnmarshalMap(item, result)
}

func getDynamoDBAttributeName[T any](goFieldName string) string {
	var entity T
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()
	
	if field, ok := typ.FieldByName(goFieldName); ok {
		if tag, ok := field.Tag.Lookup("dynamodbav"); ok {
			return tag
		}
		if tag, ok := field.Tag.Lookup("json"); ok {
			return tag
		}
	}
	return goFieldName
}

func (r *DynamoDBRepository[T]) getPK(entity T) string {
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	return val.Type().Name()
}

func (r *DynamoDBRepository[T]) getGinbootId(entity T) (string, error) {
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if tag, ok := field.Tag.Lookup("ginboot"); ok && tag == "id" {
			return val.Field(i).String(), nil
		}
	}

	return "", errors.New("ginboot:\"id\" tag not found in struct")
}

const (
	EntityIdIndex        = "EntityIdIndex"
	PKCreatedAtSortIndex = "PK-createdAt-sort-index"
)

func (r *DynamoDBRepository[T]) EnableTTL(ctx context.Context) {
	log.Printf("Ensuring TTL is enabled on attribute 'ttl' for table %s...", dynamoConfig.TableName)
	updateTTLInput := &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(dynamoConfig.TableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	}

	_, err := r.client.UpdateTimeToLive(ctx, updateTTLInput)
	if err != nil {
		log.Printf("Failed to enable TTL for table %s: %v", dynamoConfig.TableName, err)
	} else {
		log.Printf("TTL on attribute 'ttl' for table %s is being enabled/is already enabled.", dynamoConfig.TableName)
	}
}

func (r *DynamoDBRepository[T]) CreateTable(ctx context.Context) error {
	input := &dynamodb.CreateTableInput{
		TableName: aws.String(dynamoConfig.TableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("pk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("sk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("createdAt"),
				AttributeType: types.ScalarAttributeTypeN,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("pk"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("sk"),
				KeyType:       types.KeyTypeRange,
			},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String(EntityIdIndex),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("id"),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
			{
				IndexName: aws.String(PKCreatedAtSortIndex),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("pk"),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String("createdAt"),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
	}

	_, err := r.client.CreateTable(ctx, input)
	return err
}


type cursorToken struct {
	PK        string `json:"pk"`
	CreatedAt int64  `json:"c"`
	SK        string `json:"sk"`
}

func (r *DynamoDBRepository[T]) FindAllCursorPaginated(pageRequest ginboot.CursorPageRequest, partitionKey string) (ginboot.CursorPageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey

	input := &dynamodb.QueryInput{
		TableName:              aws.String(dynamoConfig.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(pageRequest.Sort.Direction == 1),
	}

	if pageRequest.Size > 0 {
		input.Limit = aws.Int32(int32(pageRequest.Size))
	} else {
		input.Limit = aws.Int32(20)
	}

	if pageRequest.NextToken != "" {
		decodedToken, err := base64.StdEncoding.DecodeString(pageRequest.NextToken)
		if err != nil {
			return ginboot.CursorPageResponse[T]{}, errors.New("invalid nextToken")
		}
		var token cursorToken
		if err := json.Unmarshal(decodedToken, &token); err != nil {
			return ginboot.CursorPageResponse[T]{}, errors.New("invalid nextToken payload")
		}
		
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"pk":        &types.AttributeValueMemberS{Value: token.PK},
			"sk":        &types.AttributeValueMemberS{Value: token.SK},
			"createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(token.CreatedAt, 10)},
		}
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return ginboot.CursorPageResponse[T]{}, err
	}

	for _, item := range output.Items {
		var temp T
		err = UnmarshalLegacyOrNative(item, &temp)
		if err != nil {
			return ginboot.CursorPageResponse[T]{}, err
		}
		results = append(results, temp)
	}

	var newNextToken string
	if output.LastEvaluatedKey != nil {
		var token cursorToken
		
		if pkAttr, ok := output.LastEvaluatedKey["pk"].(*types.AttributeValueMemberS); ok {
			token.PK = pkAttr.Value
		}
		if skAttr, ok := output.LastEvaluatedKey["sk"].(*types.AttributeValueMemberS); ok {
			token.SK = skAttr.Value
		}
		if cAttr, ok := output.LastEvaluatedKey["createdAt"].(*types.AttributeValueMemberN); ok {
			token.CreatedAt, _ = strconv.ParseInt(cAttr.Value, 10, 64)
		}
		
		b, _ := json.Marshal(token)
		newNextToken = base64.StdEncoding.EncodeToString(b)
	}

	return ginboot.CursorPageResponse[T]{
		Contents:  results,
		NextToken: newNextToken,
		Pageable:  pageRequest,
	}, nil
}
