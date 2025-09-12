package ginboot

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoDBRepository[T interface{}] struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoDBRepository[T interface{}](client *dynamodb.Client, tableName string) *DynamoDBRepository[T] {
	return &DynamoDBRepository[T]{
		client:    client,
		tableName: tableName,
	}
}

func (r *DynamoDBRepository[T]) FindById(id string) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return result, err
	}

	input := &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key:       map[string]types.AttributeValue{"id": key},
	}

	output, err := r.client.GetItem(ctx, input)
	if err != nil {
		return result, err
	}

	if output.Item == nil {
		return result, errors.New("item not found")
	}

	err = attributevalue.UnmarshalMap(output.Item, &result)
	return result, err
}

func (r *DynamoDBRepository[T]) FindAllById(ids []string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keys := make([]map[string]types.AttributeValue, len(ids))
	for i, id := range ids {
		key, err := attributevalue.Marshal(id)
		if err != nil {
			return nil, err
		}
		keys[i] = map[string]types.AttributeValue{"id": key}
	}

	input := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			r.tableName: {
				Keys: keys,
			},
		},
	}

	output, err := r.client.BatchGetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	var results []T
	for _, item := range output.Responses[r.tableName] {
		var entity T
		err = attributevalue.UnmarshalMap(item, &entity)
		if err != nil {
			return nil, err
		}
		results = append(results, entity)
	}

	return results, nil
}

func (r *DynamoDBRepository[T]) Save(doc T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := attributevalue.MarshalMap(doc)
	if err != nil {
		return err
	}

	input := &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
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
		item, err := attributevalue.MarshalMap(doc)
		if err != nil {
			return err
		}
		writeRequests[i] = types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		}
	}

	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			r.tableName: writeRequests,
		},
	}

	_, err := r.client.BatchWriteItem(ctx, input)
	return err
}

func (r *DynamoDBRepository[T]) Update(doc T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	item, err := attributevalue.MarshalMap(doc)
	if err != nil {
		return err
	}

	input := &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	}

	_, err = r.client.PutItem(ctx, input)
	return err
}

func (r *DynamoDBRepository[T]) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key, err := attributevalue.Marshal(id)
	if err != nil {
		return err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: &r.tableName,
		Key:       map[string]types.AttributeValue{"id": key},
	}

	_, err = r.client.DeleteItem(ctx, input)
	return err
}

func (r *DynamoDBRepository[T]) FindOneBy(field string, value interface{}) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T

	// Note: Using Scan for FindOneBy on arbitrary fields can be inefficient for large tables.
	// For better performance on frequently queried non-primary key fields, consider using Global Secondary Indexes (GSIs).

	filterExpression := aws.String("#F = :val")
	expressionAttributeValues, err := attributevalue.MarshalMap(map[string]interface{}{
		":val": value,
	})
	if err != nil {
		return result, err
	}
	expressionAttributeNames := map[string]string{
		"#F": field,
	}

	input := &dynamodb.ScanInput{
		TableName:                 &r.tableName,
		Limit:                     aws.Int32(1),
		FilterExpression:          filterExpression,
		ExpressionAttributeValues: expressionAttributeValues,
		ExpressionAttributeNames:  expressionAttributeNames,
	}

	output, err := r.client.Scan(ctx, input)
	if err != nil {
		return result, err
	}

	if len(output.Items) == 0 {
		return result, errors.New("item not found")
	}

	err = attributevalue.UnmarshalMap(output.Items[0], &result)
	return result, err
}

func (r *DynamoDBRepository[T]) FindOneByFilters(filters map[string]interface{}) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T

	// Note: Using Scan for FindOneByFilters can be inefficient for large tables.
	// For better performance on frequently queried non-primary key fields, consider using Global Secondary Indexes (GSIs).

	filterExpressions := make([]string, 0, len(filters))
	expressionAttributeValues := make(map[string]types.AttributeValue)
	expressionAttributeNames := make(map[string]string)

	for field, value := range filters {
		placeholder := "#" + field // Use a placeholder for the attribute name
		filterExpressions = append(filterExpressions, placeholder+" = :"+field)
		attrValue, err := attributevalue.Marshal(value)
		if err != nil {
			return result, err
		}
		expressionAttributeValues[":"+field] = attrValue
		expressionAttributeNames[placeholder] = field // Map placeholder to actual field name
	}

	filterExpression := aws.String(strings.Join(filterExpressions, " AND "))

	input := &dynamodb.ScanInput{
		TableName:                 &r.tableName,
		Limit:                     aws.Int32(1),
		FilterExpression:          filterExpression,
		ExpressionAttributeValues: expressionAttributeValues,
		ExpressionAttributeNames:  expressionAttributeNames,
	}

	output, err := r.client.Scan(ctx, input)
	if err != nil {
		return result, err
	}

	if len(output.Items) == 0 {
		return result, errors.New("item not found")
	}

	err = attributevalue.UnmarshalMap(output.Items[0], &result)
	return result, err
}

func (r *DynamoDBRepository[T]) FindBy(field string, value interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T

	// Note: Using Scan for FindBy on arbitrary fields can be inefficient for large tables.
	// For better performance on frequently queried non-primary key fields, consider using Global Secondary Indexes (GSIs).

	filterExpression := aws.String("#F = :val")
	expressionAttributeValues, err := attributevalue.MarshalMap(map[string]interface{}{
		":val": value,
	})
	if err != nil {
		return nil, err
	}
	expressionAttributeNames := map[string]string{
		"#F": field,
	}

	input := &dynamodb.ScanInput{
		TableName:                 &r.tableName,
		FilterExpression:          filterExpression,
		ExpressionAttributeValues: expressionAttributeValues,
		ExpressionAttributeNames:  expressionAttributeNames,
	}

	output, err := r.client.Scan(ctx, input)
	if err != nil {
		return nil, err
	}

	err = attributevalue.UnmarshalListOfMaps(output.Items, &results)
	return results, err
}

func (r *DynamoDBRepository[T]) FindByFilters(filters map[string]interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T

	// Note: Using Scan for FindByFilters can be inefficient for large tables.
	// For better performance on frequently queried non-primary key fields, consider using Global Secondary Indexes (GSIs).

	filterExpressions := make([]string, 0, len(filters))
	expressionAttributeValues := make(map[string]types.AttributeValue)
	expressionAttributeNames := make(map[string]string)

	for field, value := range filters {
		placeholder := "#" + field // Use a placeholder for the attribute name
		filterExpressions = append(filterExpressions, placeholder+" = :"+field)
		attrValue, err := attributevalue.Marshal(value)
		if err != nil {
			return nil, err
		}
		expressionAttributeValues[":"+field] = attrValue
		expressionAttributeNames[placeholder] = field // Map placeholder to actual field name
	}

	filterExpression := aws.String(strings.Join(filterExpressions, " AND "))

	input := &dynamodb.ScanInput{
		TableName:                 &r.tableName,
		FilterExpression:          filterExpression,
		ExpressionAttributeValues: expressionAttributeValues,
		ExpressionAttributeNames:  expressionAttributeNames,
	}

	output, err := r.client.Scan(ctx, input)
	if err != nil {
		return nil, err
	}

	err = attributevalue.UnmarshalListOfMaps(output.Items, &results)
	return results, err
}

func (r *DynamoDBRepository[T]) FindAll(findOpts ...interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T

	input := &dynamodb.ScanInput{
		TableName: &r.tableName,
	}

	output, err := r.client.Scan(ctx, input)
	if err != nil {
		return nil, err
	}

	err = attributevalue.UnmarshalListOfMaps(output.Items, &results)
	return results, err
}

func (r *DynamoDBRepository[T]) FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var results []T
	var lastEvaluatedKey map[string]types.AttributeValue

	// DynamoDB pagination works with ExclusiveStartKey, not offset.
	// To simulate offset, we need to scan and discard items until the desired page.
	// This can be inefficient for deep pagination.

	// First, get the total count (can be expensive for large tables)
	countInput := &dynamodb.ScanInput{
		TableName: &r.tableName,
		Select:    types.SelectCount,
	}
	countOutput, err := r.client.Scan(ctx, countInput)
	if err != nil {
		return PageResponse[T]{}, err
	}
	totalElements := int(countOutput.Count)

	// Calculate the number of items to skip
	skip := (pageRequest.Page - 1) * pageRequest.Size

	// Perform scan operations to get to the correct page
	itemsScanned := 0
	for itemsScanned < skip {
		scanInput := &dynamodb.ScanInput{
			TableName:         &r.tableName,
			Limit:             aws.Int32(int32(skip - itemsScanned)),
			ExclusiveStartKey: lastEvaluatedKey,
		}
		scanOutput, err := r.client.Scan(ctx, scanInput)
		if err != nil {
			return PageResponse[T]{}, err
		}
		itemsScanned += len(scanOutput.Items)
		lastEvaluatedKey = scanOutput.LastEvaluatedKey
		if lastEvaluatedKey == nil && itemsScanned < skip {
			// Reached end of table before reaching the skip point
			break
		}
	}

	// Now fetch the actual page content
	scanInput := &dynamodb.ScanInput{
		TableName:         &r.tableName,
		Limit:             aws.Int32(int32(pageRequest.Size)),
		ExclusiveStartKey: lastEvaluatedKey,
	}
	scanOutput, err := r.client.Scan(ctx, scanInput)
	if err != nil {
		return PageResponse[T]{}, err
	}

	err = attributevalue.UnmarshalListOfMaps(scanOutput.Items, &results)
	if err != nil {
		return PageResponse[T]{}, err
	}

	totalPages := int(math.Ceil(float64(totalElements) / float64(pageRequest.Size)))

	return PageResponse[T]{
		Contents:         results,
		NumberOfElements: len(results),
		Pageable:         pageRequest,
		TotalElements:    totalElements,
		TotalPages:       totalPages,
	}, nil
}

func (r *DynamoDBRepository[T]) FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var allMatchingItems []T
	var lastEvaluatedKey map[string]types.AttributeValue

	// Note: Using Scan for FindByPaginated can be inefficient for large tables and deep pagination.
	// For better performance, consider using Global Secondary Indexes (GSIs) with Query operations
	// if your pagination and filtering needs align with GSI capabilities.

	filterExpressions := make([]string, 0, len(filters))
	expressionAttributeValues := make(map[string]types.AttributeValue)
	expressionAttributeNames := make(map[string]string)

	for field, value := range filters {
		placeholder := "#" + field
		filterExpressions = append(filterExpressions, placeholder+" = :"+field)
		attrValue, err := attributevalue.Marshal(value)
		if err != nil {
			return PageResponse[T]{}, err
		}
		expressionAttributeValues[":"+field] = attrValue
		expressionAttributeNames[placeholder] = field
	}

	filterExpression := aws.String(strings.Join(filterExpressions, " AND "))

	// Continuously scan until we have enough items or no more items
	for {
		scanInput := &dynamodb.ScanInput{
			TableName:                 &r.tableName,
			Limit:                     aws.Int32(100), // Scan a reasonable number of items at a time
			ExclusiveStartKey:         lastEvaluatedKey,
			FilterExpression:          filterExpression,
			ExpressionAttributeValues: expressionAttributeValues,
			ExpressionAttributeNames:  expressionAttributeNames,
		}

		scanOutput, err := r.client.Scan(ctx, scanInput)
		if err != nil {
			return PageResponse[T]{}, err
		}

		var scannedItems []T
		err = attributevalue.UnmarshalListOfMaps(scanOutput.Items, &scannedItems)
		if err != nil {
			return PageResponse[T]{}, err
		}
		allMatchingItems = append(allMatchingItems, scannedItems...)

		lastEvaluatedKey = scanOutput.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break // No more items to scan
		}
	}

	totalElements := len(allMatchingItems)

	skip := (pageRequest.Page - 1) * pageRequest.Size
	if skip >= totalElements {
		return PageResponse[T]{
			Contents:         []T{},
			NumberOfElements: 0,
			Pageable:         pageRequest,
			TotalElements:    totalElements,
			TotalPages:       int(math.Ceil(float64(totalElements) / float64(pageRequest.Size))),
		}, nil
	}

	endIndex := skip + pageRequest.Size
	if endIndex > totalElements {
		endIndex = totalElements
	}

	results := allMatchingItems[skip:endIndex]

	totalPages := int(math.Ceil(float64(totalElements) / float64(pageRequest.Size)))

	return PageResponse[T]{
		Contents:         results,
		NumberOfElements: len(results),
		Pageable:         pageRequest,
		TotalElements:    totalElements,
		TotalPages:       totalPages,
	}, nil
}

func (r *DynamoDBRepository[T]) CountBy(field string, value interface{}) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Note: Using Scan for CountBy can be inefficient for large tables.
	// For better performance on frequently queried non-primary key fields, consider using Global Secondary Indexes (GSIs).

	filterExpression := aws.String("#F = :val")
	expressionAttributeValues, err := attributevalue.MarshalMap(map[string]interface{}{
		":val": value,
	})
	if err != nil {
		return 0, err
	}
	expressionAttributeNames := map[string]string{
		"#F": field,
	}

	input := &dynamodb.ScanInput{
		TableName:                 &r.tableName,
		Select:                    types.SelectCount,
		FilterExpression:          filterExpression,
		ExpressionAttributeValues: expressionAttributeValues,
		ExpressionAttributeNames:  expressionAttributeNames,
	}

	output, err := r.client.Scan(ctx, input)
	if err != nil {
		return 0, err
	}

	return int64(output.Count), nil
}

func (r *DynamoDBRepository[T]) CountByFilters(filters map[string]interface{}) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Note: Using Scan for CountByFilters can be inefficient for large tables.
	// For better performance on frequently queried non-primary key fields, consider using Global Secondary Indexes (GSIs).

	filterExpressions := make([]string, 0, len(filters))
	expressionAttributeValues := make(map[string]types.AttributeValue)
	expressionAttributeNames := make(map[string]string)

	for field, value := range filters {
		placeholder := "#" + field
		filterExpressions = append(filterExpressions, placeholder+" = :"+field)
		attrValue, err := attributevalue.Marshal(value)
		if err != nil {
			return 0, err
		}
		expressionAttributeValues[":"+field] = attrValue
		expressionAttributeNames[placeholder] = field
	}

	filterExpression := aws.String(strings.Join(filterExpressions, " AND "))

	input := &dynamodb.ScanInput{
		TableName:                 &r.tableName,
		Select:                    types.SelectCount,
		FilterExpression:          filterExpression,
		ExpressionAttributeValues: expressionAttributeValues,
		ExpressionAttributeNames:  expressionAttributeNames,
	}

	output, err := r.client.Scan(ctx, input)
	if err != nil {
		return 0, err
	}

	return int64(output.Count), nil
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
