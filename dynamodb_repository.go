package ginboot

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoDBItem struct {
	PK        string `dynamodbav:"pk"`
	SK        string `dynamodbav:"sk"`
	Data      string `dynamodbav:"data"`
	CreatedAt int64  `dynamodbav:"createdAt"`
	UpdatedAt int64  `dynamodbav:"updatedAt"`
	Version   int64  `dynamodbav:"version"`
}

type DynamoDBRepository[T any] struct {
	client *dynamodb.Client
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

func (r *DynamoDBRepository[T]) findById(id string) (DynamoDBItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var item DynamoDBItem
	var entity T
	pk := r.getPK(entity)

	key, err := attributevalue.MarshalMap(map[string]string{
		"pk": pk,
		"sk": id,
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

func (r *DynamoDBRepository[T]) FindById(id string) (T, error) {
	var result T
	item, err := r.findById(id)
	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(item.Data), &result)
	return result, err
}

func (r *DynamoDBRepository[T]) FindAllById(ids []string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity)

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
			config.TableName: {
				Keys: keys,
			},
		},
	}

	output, err := r.client.BatchGetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	var results []T
	for _, item := range output.Responses[config.TableName] {
		var result T
		var temp DynamoDBItem
		err = attributevalue.UnmarshalMap(item, &temp)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(temp.Data), &result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) Save(doc T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UnixMilli()

	pk := r.getPK(doc)
	sk, err := r.getSK(doc)
	if err != nil {
		return err
	}

	// Get current version and increment it
	var version int64
	var createdAt int64

	// Try to find existing item to get version
	item, err := r.findById(sk)
	if err == nil {
		// Item exists, get its version and createdAt
		version = item.Version
		createdAt = item.CreatedAt
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	newItem := DynamoDBItem{
		PK:        pk,
		SK:        sk,
		Data:      string(data),
		CreatedAt: createdAt,
		UpdatedAt: now,
		Version:   version + 1,
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

func (r *DynamoDBRepository[T]) SaveOrUpdate(doc T) error {
	return r.Save(doc)
}

func (r *DynamoDBRepository[T]) SaveAll(docs []T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if len(docs) == 0 {
		return nil
	}

	writeRequests := make([]types.WriteRequest, len(docs))
	for i, doc := range docs {
		now := time.Now().UnixMilli()

		pk := r.getPK(doc)
		sk, err := r.getSK(doc)
		if err != nil {
			return err
		}

		// Get current version and increment it
		var version int64
		var createdAt int64

		// Try to find existing item to get version
		item, err := r.FindById(sk)
		if err == nil {
			// Item exists, get its version and createdAt
			val := reflect.ValueOf(item)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}
			version = val.FieldByName("Version").Int()
			createdAt = val.FieldByName("CreatedAt").Int()
		}

		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}

		newItem := DynamoDBItem{
			PK:        pk,
			SK:        sk,
			Data:      string(data),
			CreatedAt: createdAt,
			UpdatedAt: now,
			Version:   version + 1,
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

	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			config.TableName: writeRequests,
		},
	}

	_, err := r.client.BatchWriteItem(ctx, input)
	return err
}

func (r *DynamoDBRepository[T]) Update(doc T) error {
	return r.Save(doc)
}

func (r *DynamoDBRepository[T]) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity)

	key, err := attributevalue.MarshalMap(map[string]string{
		"pk": pk,
		"sk": id,
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

func (r *DynamoDBRepository[T]) FindOneBy(field string, value interface{}) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	pk := r.getPK(result)

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

func (r *DynamoDBRepository[T]) FindOneByFilters(filters map[string]interface{}) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	pk := r.getPK(result)

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

func (r *DynamoDBRepository[T]) FindBy(field string, value interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity)

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
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
		if fieldValue == value {
			results = append(results, temp)
		}
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) FindByFilters(filters map[string]interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity)

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
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

	return results, nil
}
func (r *DynamoDBRepository[T]) FindAll(findOpts ...interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity)

	input := &dynamodb.QueryInput{
		TableName:              aws.String(config.TableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
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

func (r *DynamoDBRepository[T]) FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity)

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

	pagedResults := results[start:end]

	return PageResponse[T]{
		Contents:         pagedResults,
		NumberOfElements: len(pagedResults),
		Pageable:         pageRequest,
		TotalElements:    totalElements,
		TotalPages:       totalPages,
	}, nil
}

func (r *DynamoDBRepository[T]) FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity)

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

	pagedResults := results[start:end]

	return PageResponse[T]{
		Contents:         pagedResults,
		NumberOfElements: len(pagedResults),
		Pageable:         pageRequest,
		TotalElements:    totalElements,
		TotalPages:       totalPages,
	}, nil
}

func (r *DynamoDBRepository[T]) CountBy(field string, value interface{}) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity)

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
		if fieldValue == value {
			count++
		}
	}

	return count, nil
}

func (r *DynamoDBRepository[T]) CountByFilters(filters map[string]interface{}) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity)

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

		for field, value := range filters {
			fieldValue := val.FieldByName(field).Interface()
			if fieldValue != value {
				match = false
				break
			}
		}

		if match {
			count++
		}
	}

	return count, nil
}

func (r *DynamoDBRepository[T]) ExistsBy(field string, value interface{}) (bool, error) {
	count, err := r.CountBy(field, value)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *DynamoDBRepository[T]) ExistsByFilters(filters map[string]interface{}) (bool, error) {
	count, err := r.CountByFilters(filters)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *DynamoDBRepository[T]) getPK(entity T) string {
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	return val.Type().Name()
}

func (r *DynamoDBRepository[T]) getSK(entity T) (string, error) {
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if _, ok := field.Tag.Lookup("ginboot"); ok {
			return val.Field(i).String(), nil
		}
	}

	return "", errors.New("ginboot:\"id\" tag not found in struct")
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
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
	}

	_, err := r.client.CreateTable(ctx, input)
	return err
}
