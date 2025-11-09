package ginboot

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

type DynamoDBRepository[T any] struct {
	client *dynamodb.Client
	ttl    time.Duration
}

func NewDynamoDBRepository[T any](client *dynamodb.Client) *DynamoDBRepository[T] {
	repo := &DynamoDBRepository[T]{
		client: client,
	}

	if config.SkipTableCreation {
		return repo
	}

	// Check if table exists, if not, create it
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := repo.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(config.TableName),
	})

	if err != nil {
		var notFoundEx *types.ResourceNotFoundException
		if errors.As(err, &notFoundEx) {
			log.Printf("DynamoDB table %s does not exist, creating it...", config.TableName)
			err = repo.CreateTable(ctx)
			if err != nil {
				log.Fatalf("Failed to create DynamoDB table %s: %v", config.TableName, err)
			}
			log.Printf("DynamoDB table %s created successfully.", config.TableName)
		} else {
			log.Fatalf("Failed to describe DynamoDB table %s: %v", config.TableName, err)
		}
	}

	return repo
}

func NewDynamoDBRepositoryWithTTL[T any](client *dynamodb.Client, ttl time.Duration) *DynamoDBRepository[T] {
	repo := &DynamoDBRepository[T]{
		client: client,
		ttl:    ttl,
	}

	if config.SkipTableCreation {
		return repo
	}

	// Check if table exists, if not, create it
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := repo.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(config.TableName),
	})

	if err != nil {
		var notFoundEx *types.ResourceNotFoundException
		if errors.As(err, &notFoundEx) {
			log.Printf("DynamoDB table %s does not exist, creating it...", config.TableName)
			err = repo.CreateTable(ctx)
			if err != nil {
				log.Fatalf("Failed to create DynamoDB table %s: %v", config.TableName, err)
			}
			log.Printf("DynamoDB table %s created successfully.", config.TableName)
		} else {
			log.Fatalf("Failed to describe DynamoDB table %s: %v", config.TableName, err)
		}
	}

	if repo.ttl > 0 {
		repo.EnableTTL(ctx)
	}

	return repo
}

func (r *DynamoDBRepository[T]) GetClient() *dynamodb.Client {
	return r.client
}

func (r *DynamoDBRepository[T]) findById(pk string, sk string) (DynamoDBItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var item DynamoDBItem

	key, err := attributevalue.MarshalMap(map[string]string{
		"pk": pk,
		"sk": sk,
	})
	if err != nil {
		return item, err
	}

	input := &dynamodb.GetItemInput{
		TableName: aws.String(config.TableName),
		Key:       key,
	}

	output, err := r.client.GetItem(ctx, input)
	if err != nil {
		return item, err
	}

	if output.Item == nil {
		return item, errors.New("item not found")
	}

	err = attributevalue.UnmarshalMap(output.Item, &item)
	return item, err
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

	err = json.Unmarshal([]byte(item.Data), &result)
	return result, err
}

func (r *DynamoDBRepository[T]) FindAllById(ids []string, partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if len(ids) == 0 {
		return []T{}, nil
	}

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	keys := make([]map[string]types.AttributeValue, len(ids))
	for i, id := range ids {
		// SK is the id from the list
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
			config.TableName: {
				Keys:           keys,
				ConsistentRead: aws.Bool(true),
			},
		},
	}

	output, err := r.client.BatchGetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	var results []T
	for _, item := range output.Responses[config.TableName] {
		var dynamoDBItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &dynamoDBItem)
		if err != nil {
			return nil, err
		}

		var result T
		err = json.Unmarshal([]byte(dynamoDBItem.Data), &result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	// Sort results by createdAt in descending order
	sort.Slice(results, func(i, j int) bool {
		createdAtI, err := r.getCreatedAt(results[i])
		if err != nil {
			return false
		}
		createdAtJ, err := r.getCreatedAt(results[j])
		if err != nil {
			return false
		}
		return createdAtI > createdAtJ
	})

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
		version = item.Version
		createdAt = item.CreatedAt
	} else {
		// Item does not exist, get createdAt from doc
		createdAt, err = r.getCreatedAt(doc)
		if err != nil {
			return err
		}
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	newItem := DynamoDBItem{
		PK:        pk,
		SK:        sk,
		ID:        id, // Keep for GSI, though may be redundant for some queries now
		Data:      string(data),
		CreatedAt: createdAt,
		UpdatedAt: now,
		Version:   version + 1,
	}

	if r.ttl > 0 {
		newItem.TTL = time.Now().Add(r.ttl).Unix()
	}

	if newItem.CreatedAt == 0 {
		newItem.CreatedAt = now
	}

	av, err := attributevalue.MarshalMap(newItem)
	if err != nil {
		return err
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(config.TableName),
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

		// Get current version and increment it
		var version int64
		var createdAt int64

		// Try to find existing item to get version
		item, err := r.findById(pk, sk)
		if err == nil {
			// Item exists, get its version and createdAt
			version = item.Version
			createdAt = item.CreatedAt
		} else {
			// Item does not exist, get createdAt from doc
			createdAt, err = r.getCreatedAt(doc)
			if err != nil {
				return err
			}
		}

		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}

		newItem := DynamoDBItem{
			PK:        pk,
			SK:        sk,
			ID:        id,
			Data:      string(data),
			CreatedAt: createdAt,
			UpdatedAt: now,
			Version:   version + 1,
		}

		if r.ttl > 0 {
			newItem.TTL = time.Now().Add(r.ttl).Unix()
		}

		if newItem.CreatedAt == 0 {
			newItem.CreatedAt = now
		}

		av, err := attributevalue.MarshalMap(newItem)
		if err != nil {
			return err
		}

		writeRequests[i] = types.WriteRequest{
			PutRequest: &types.PutRequest{Item: av},
		}
	}

	// Batch write in chunks of 25
	for i := 0; i < len(writeRequests); i += 25 {
		end := i + 25
		if end > len(writeRequests) {
			end = len(writeRequests)
		}

		batchWriteInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				config.TableName: writeRequests[i:end],
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
		TableName: aws.String(config.TableName),
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
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return result, err
	}

	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return result, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
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

	return result, errors.New("item not found")
}

func (r *DynamoDBRepository[T]) FindOneByFilters(filters map[string]interface{}, partitionKey string) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return result, err
	}

	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return result, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
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

	return result, errors.New("item not found")
}

func (r *DynamoDBRepository[T]) FindBy(field string, value interface{}, partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false), // Sort by createdAt DESC
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, err
	}

	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
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
			// Handle operators like $gte, $lt
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
					// Unknown operator, treat as no match
					match = false
				}
			}
		} else {
			// Direct equality match
			if !reflect.DeepEqual(fieldValue, value) {
				match = false
			}
		}

		if match {
			results = append(results, temp)
		}
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) FindByFilters(filters map[string]interface{}, partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false), // Sort by createdAt DESC
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, err
	}

	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
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
				// Handle operators like $gte, $lt
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
						// Unknown operator, treat as no match
						match = false
					}
				}
			} else {
				// Direct equality match
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

	return results, nil
}
func (r *DynamoDBRepository[T]) FindAll(partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		IndexName:              aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false), // Sort by createdAt DESC
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, err
	}

	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
		if err != nil {
			return nil, err
		}
		results = append(results, temp)
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) FindAllPaginated(pageRequest PageRequest, partitionKey string) (PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return PageResponse[T]{}, err
	}

	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return PageResponse[T]{}, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
		if err != nil {
			return PageResponse[T]{}, err
		}
		results = append(results, temp)
	}

	if pageRequest.Size == -1 {
		return PageResponse[T]{
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

	return PageResponse[T]{
		Contents:         pagedResults,
		NumberOfElements: len(pagedResults),
		Pageable:         pageRequest,
		TotalElements:    totalElements,
		TotalPages:       totalPages,
	}, nil
}

func (r *DynamoDBRepository[T]) FindByPaginated(pageRequest PageRequest, filters map[string]interface{}, partitionKey string) (PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return PageResponse[T]{}, err
	}

	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return PageResponse[T]{}, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
		if err != nil {
			return PageResponse[T]{}, err
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

	if pageRequest.Size == -1 {
		return PageResponse[T]{
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

	return PageResponse[T]{
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
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return 0, err
	}

	var count int64
	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return 0, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
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
			// Handle operators like $gte, $lt
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
					// Unknown operator, treat as no match
					match = false
				}
			}
		} else {
			// Direct equality match
			if !reflect.DeepEqual(fieldValue, value) {
				match = false
			}
		}

		if match {
			count++
		}
	}

	return count, nil
}

func (r *DynamoDBRepository[T]) CountByFilters(filters map[string]interface{}, partitionKey string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey // Composite PK

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	output, err := r.client.Query(ctx, input)
	if err != nil {
		return 0, err
	}

	var count int64
	for _, item := range output.Items {
		var temp T
		var tempItem DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &tempItem)
		if err != nil {
			return 0, err
		}

		err = json.Unmarshal([]byte(tempItem.Data), &temp)
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
				// Handle operators like $gte, $lt
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
						// Unknown operator, treat as no match
						match = false
					}
				}
			} else {
				// Direct equality match
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

	// Batch delete in chunks of 25
	for i := 0; i < len(writeRequests); i += 25 {
		end := i + 25
		if end > len(writeRequests) {
			end = len(writeRequests)
		}

		batchWriteInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				config.TableName: writeRequests[i:end],
			},
		}
		_, err := r.client.BatchWriteItem(context.TODO(), batchWriteInput)
		if err != nil {
			return err
		}
	}

	return nil
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

func (r *DynamoDBRepository[T]) getCreatedAt(entity T) (int64, error) {
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Name == "CreatedAt" {
			return val.Field(i).Int(), nil
		}
	}

	return 0, errors.New("createdAt field not found in struct")
}

func (r *DynamoDBRepository[T]) EnableTTL(ctx context.Context) {
	log.Printf("Ensuring TTL is enabled on attribute 'ttl' for table %s...", config.TableName)
	updateTTLInput := &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(config.TableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	}

	_, err := r.client.UpdateTimeToLive(ctx, updateTTLInput)
	if err != nil {
		log.Printf("Failed to enable TTL for table %s: %v", config.TableName, err)
	} else {
		log.Printf("TTL on attribute 'ttl' for table %s is being enabled/is already enabled.", config.TableName)
	}
}

func (r *DynamoDBRepository[T]) CreateTable(ctx context.Context) error {
	input := &dynamodb.CreateTableInput{
		TableName: aws.String(config.TableName),
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
				AttributeName: aws.String("id"), // Attribute for GSI
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("createdAt"), // Attribute for GSI
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
