package ginboot

//
//import (
//	"context"
//	"fmt"
//	"strings"
//
//	"github.com/aws/aws-sdk-go-v2/aws"
//	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
//	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
//	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
//)
//
//type DynamoRepository[T Document] struct {
//	client    *dynamodb.Client
//	tableName string
//}
//
//type Expression struct {
//	Filter *string
//	Names  map[string]string
//	Values map[string]types.AttributeValue
//}
//
//func NewDynamoRepository[T Document](client *dynamodb.Client, tableName string) *DynamoRepository[T] {
//	return &DynamoRepository[T]{
//		client:    client,
//		tableName: tableName,
//	}
//}
//
//func (r *DynamoRepository[T]) FindById(id string) (T, error) {
//	var result T
//	key := map[string]types.AttributeValue{
//		"_id": &types.AttributeValueMemberS{Value: id},
//	}
//
//	input := &dynamodb.GetItemInput{
//		TableName: aws.String(r.tableName),
//		Key:       key,
//	}
//
//	output, err := r.client.GetItem(context.Background(), input)
//	if err != nil {
//		return result, err
//	}
//	if output.Item == nil {
//		return result, fmt.Errorf("item not found")
//	}
//
//	err = attributevalue.UnmarshalMap(output.Item, &result)
//	return result, err
//}
//
//func (r *DynamoRepository[T]) FindAllById(ids []string) ([]T, error) {
//	var results []T
//	if len(ids) == 0 {
//		return results, nil
//	}
//
//	keys := make([]map[string]types.AttributeValue, len(ids))
//	for i, id := range ids {
//		keys[i] = map[string]types.AttributeValue{
//			"_id": &types.AttributeValueMemberS{Value: id},
//		}
//	}
//
//	input := &dynamodb.BatchGetItemInput{
//		RequestItems: map[string]types.KeysAndAttributes{
//			r.tableName: {
//				Keys: keys,
//			},
//		},
//	}
//
//	output, err := r.client.BatchGetItem(context.Background(), input)
//	if err != nil {
//		return nil, err
//	}
//
//	if items, ok := output.Responses[r.tableName]; ok {
//		err = attributevalue.UnmarshalListOfMaps(items, &results)
//	}
//	return results, err
//}
//
//func (r *DynamoRepository[T]) Save(doc T) error {
//	item, err := attributevalue.MarshalMap(doc)
//	if err != nil {
//		return err
//	}
//
//	// Add collection name as partition key
//	collection := doc.GetCollectionName()
//	collectionKey, err := attributevalue.Marshal(collection)
//	if err != nil {
//		return err
//	}
//	item["collection"] = collectionKey
//
//	input := &dynamodb.PutItemInput{
//		TableName: aws.String(r.tableName),
//		Item:      item,
//	}
//
//	_, err = r.client.PutItem(context.Background(), input)
//	return err
//}
//
//func (r *DynamoRepository[T]) SaveAll(docs []T) error {
//	if len(docs) == 0 {
//		return nil
//	}
//
//	writeRequests := make([]types.WriteRequest, len(docs))
//	for i, doc := range docs {
//		item, err := attributevalue.MarshalMap(doc)
//		if err != nil {
//			return err
//		}
//
//		// Add collection name as partition key
//		collection := doc.GetCollectionName()
//		collectionKey, err := attributevalue.Marshal(collection)
//		if err != nil {
//			return err
//		}
//		item["collection"] = collectionKey
//
//		writeRequests[i] = types.WriteRequest{
//			PutRequest: &types.PutRequest{
//				Item: item,
//			},
//		}
//	}
//
//	input := &dynamodb.BatchWriteItemInput{
//		RequestItems: map[string][]types.WriteRequest{
//			r.tableName: writeRequests,
//		},
//	}
//
//	_, err := r.client.BatchWriteItem(context.Background(), input)
//	return err
//}
//
//func (r *DynamoRepository[T]) Update(doc T) error {
//	return r.Save(doc)
//}
//
//func (r *DynamoRepository[T]) Delete(id string) error {
//	input := &dynamodb.DeleteItemInput{
//		TableName: aws.String(r.tableName),
//		Key: map[string]types.AttributeValue{
//			"_id": &types.AttributeValueMemberS{Value: id},
//		},
//	}
//
//	_, err := r.client.DeleteItem(context.Background(), input)
//	return err
//}
//
//func (r *DynamoRepository[T]) FindOneBy(field string, value interface{}) (T, error) {
//	var result T
//	val, err := attributevalue.Marshal(value)
//	if err != nil {
//		return result, err
//	}
//
//	input := &dynamodb.QueryInput{
//		TableName:              aws.String(r.tableName),
//		KeyConditionExpression: aws.String("collection = :c"),
//		FilterExpression:       aws.String(fmt.Sprintf("%s = :v", field)),
//		ExpressionAttributeValues: map[string]types.AttributeValue{
//			":c": val,
//			":v": val,
//		},
//		Limit: aws.Int32(1),
//	}
//
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return result, err
//	}
//
//	if len(output.Items) == 0 {
//		return result, fmt.Errorf("item not found")
//	}
//
//	err = attributevalue.UnmarshalMap(output.Items[0], &result)
//	return result, err
//}
//
//func (r *DynamoRepository[T]) FindOneByFilters(filters map[string]interface{}) (T, error) {
//	var result T
//	var doc T
//	collection := doc.GetCollectionName()
//	collectionKey, err := attributevalue.Marshal(collection)
//	if err != nil {
//		return result, err
//	}
//
//	expr, err := r.buildFilterExpression(filters)
//	if err != nil {
//		return result, err
//	}
//
//	input := &dynamodb.QueryInput{
//		TableName:                 aws.String(r.tableName),
//		KeyConditionExpression:    aws.String("collection = :c"),
//		FilterExpression:          expr.Filter,
//		ExpressionAttributeNames:  expr.Names,
//		ExpressionAttributeValues: expr.Values,
//		Limit:                     aws.Int32(1),
//	}
//	input.ExpressionAttributeValues[":c"] = collectionKey
//
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return result, err
//	}
//
//	if len(output.Items) == 0 {
//		return result, fmt.Errorf("item not found")
//	}
//
//	err = attributevalue.UnmarshalMap(output.Items[0], &result)
//	return result, err
//}
//
//func (r *DynamoRepository[T]) FindBy(field string, value interface{}) ([]T, error) {
//	var results []T
//	var doc T
//	collection := doc.GetCollectionName()
//	collectionKey, err := attributevalue.Marshal(collection)
//	if err != nil {
//		return nil, err
//	}
//
//	val, err := attributevalue.Marshal(value)
//	if err != nil {
//		return nil, err
//	}
//
//	input := &dynamodb.QueryInput{
//		TableName:              aws.String(r.tableName),
//		KeyConditionExpression: aws.String("collection = :c"),
//		FilterExpression:       aws.String(fmt.Sprintf("%s = :v", field)),
//		ExpressionAttributeValues: map[string]types.AttributeValue{
//			":c": collectionKey,
//			":v": val,
//		},
//	}
//
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return nil, err
//	}
//
//	err = attributevalue.UnmarshalListOfMaps(output.Items, &results)
//	return results, err
//}
//
//func (r *DynamoRepository[T]) FindByFilters(filters map[string]interface{}) ([]T, error) {
//	var results []T
//	var doc T
//	collection := doc.GetCollectionName()
//	collectionKey, err := attributevalue.Marshal(collection)
//	if err != nil {
//		return nil, err
//	}
//
//	expr, err := r.buildFilterExpression(filters)
//	if err != nil {
//		return nil, err
//	}
//
//	input := &dynamodb.QueryInput{
//		TableName:                 aws.String(r.tableName),
//		KeyConditionExpression:    aws.String("collection = :c"),
//		FilterExpression:          expr.Filter,
//		ExpressionAttributeNames:  expr.Names,
//		ExpressionAttributeValues: expr.Values,
//	}
//	input.ExpressionAttributeValues[":c"] = collectionKey
//
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return nil, err
//	}
//
//	err = attributevalue.UnmarshalListOfMaps(output.Items, &results)
//	return results, err
//}
//
//func (r *DynamoRepository[T]) FindAll(options ...interface{}) ([]T, error) {
//	var results []T
//	var doc T
//	collection := doc.GetCollectionName()
//	collectionKey, err := attributevalue.Marshal(collection)
//	if err != nil {
//		return nil, err
//	}
//
//	input := &dynamodb.QueryInput{
//		TableName:              aws.String(r.tableName),
//		KeyConditionExpression: aws.String("collection = :c"),
//		ExpressionAttributeValues: map[string]types.AttributeValue{
//			":c": collectionKey,
//		},
//	}
//
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return nil, err
//	}
//
//	err = attributevalue.UnmarshalListOfMaps(output.Items, &results)
//	return results, err
//}
//
//func (r *DynamoRepository[T]) FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error) {
//	var results []T
//	var doc T
//	collection := doc.GetCollectionName()
//	collectionKey, err := attributevalue.Marshal(collection)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	// First, get total count
//	countInput := &dynamodb.QueryInput{
//		TableName:              aws.String(r.tableName),
//		KeyConditionExpression: aws.String("collection = :c"),
//		ExpressionAttributeValues: map[string]types.AttributeValue{
//			":c": collectionKey,
//		},
//		Select: types.SelectCount,
//	}
//
//	countOutput, err := r.client.Query(context.Background(), countInput)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	totalElements := int(countOutput.Count)
//	totalPages := (totalElements + pageRequest.Size - 1) / pageRequest.Size
//
//	// Now get the page of data
//	input := &dynamodb.QueryInput{
//		TableName:              aws.String(r.tableName),
//		KeyConditionExpression: aws.String("collection = :c"),
//		ExpressionAttributeValues: map[string]types.AttributeValue{
//			":c": collectionKey,
//		},
//		Limit: aws.Int32(int32(pageRequest.Size)),
//	}
//
//	// Add sort if specified
//	if pageRequest.Sort.Field != "" {
//		input.ExpressionAttributeNames = map[string]string{
//			"#sortKey": pageRequest.Sort.Field,
//		}
//		input.ScanIndexForward = aws.Bool(pageRequest.Sort.Direction >= 0) // ascending if >= 0, descending if < 0
//	}
//
//	// Skip to the correct page
//	if pageRequest.Page > 1 {
//		// We need to scan through previous pages to get to our target page
//		skip := (pageRequest.Page - 1) * pageRequest.Size
//		var lastKey map[string]types.AttributeValue
//
//		for skip > 0 {
//			batchSize := min(skip, 100) // DynamoDB max limit is 100
//			batchInput := *input
//			batchInput.Limit = aws.Int32(int32(batchSize))
//			if lastKey != nil {
//				batchInput.ExclusiveStartKey = lastKey
//			}
//
//			batchOutput, err := r.client.Query(context.Background(), &batchInput)
//			if err != nil {
//				return PageResponse[T]{}, err
//			}
//
//			if batchOutput.LastEvaluatedKey == nil {
//				// We've reached the end
//				break
//			}
//
//			lastKey = batchOutput.LastEvaluatedKey
//			skip -= len(batchOutput.Items)
//		}
//
//		if lastKey != nil {
//			input.ExclusiveStartKey = lastKey
//		}
//	}
//
//	// Get the actual page data
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	err = attributevalue.UnmarshalListOfMaps(output.Items, &results)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	return PageResponse[T]{
//		Contents:         results,
//		NumberOfElements: len(results),
//		TotalElements:    totalElements,
//		TotalPages:       totalPages,
//		Pageable:         pageRequest,
//	}, nil
//}
//
//func (r *DynamoRepository[T]) FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error) {
//	var results []T
//	var doc T
//	collection := doc.GetCollectionName()
//	collectionKey, err := attributevalue.Marshal(collection)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	expr, err := r.buildFilterExpression(filters)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	// First, get total count with filters
//	countInput := &dynamodb.QueryInput{
//		TableName:                 aws.String(r.tableName),
//		KeyConditionExpression:    aws.String("collection = :c"),
//		FilterExpression:          expr.Filter,
//		ExpressionAttributeNames:  expr.Names,
//		ExpressionAttributeValues: expr.Values,
//		Select:                    types.SelectCount,
//	}
//	countInput.ExpressionAttributeValues[":c"] = collectionKey
//
//	countOutput, err := r.client.Query(context.Background(), countInput)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	totalElements := int(countOutput.Count)
//	totalPages := (totalElements + pageRequest.Size - 1) / pageRequest.Size
//
//	// Now get the page of data
//	input := &dynamodb.QueryInput{
//		TableName:                 aws.String(r.tableName),
//		KeyConditionExpression:    aws.String("collection = :c"),
//		FilterExpression:          expr.Filter,
//		ExpressionAttributeNames:  expr.Names,
//		ExpressionAttributeValues: expr.Values,
//		Limit:                     aws.Int32(int32(pageRequest.Size)),
//	}
//	input.ExpressionAttributeValues[":c"] = collectionKey
//
//	// Add sort if specified
//	if pageRequest.Sort.Field != "" {
//		input.ExpressionAttributeNames["#sortKey"] = pageRequest.Sort.Field
//		input.ScanIndexForward = aws.Bool(pageRequest.Sort.Direction >= 0) // ascending if >= 0, descending if < 0
//	}
//
//	// Skip to the correct page
//	if pageRequest.Page > 1 {
//		// We need to scan through previous pages to get to our target page
//		skip := (pageRequest.Page - 1) * pageRequest.Size
//		var lastKey map[string]types.AttributeValue
//
//		for skip > 0 {
//			batchSize := min(skip, 100) // DynamoDB max limit is 100
//			batchInput := *input
//			batchInput.Limit = aws.Int32(int32(batchSize))
//			if lastKey != nil {
//				batchInput.ExclusiveStartKey = lastKey
//			}
//
//			batchOutput, err := r.client.Query(context.Background(), &batchInput)
//			if err != nil {
//				return PageResponse[T]{}, err
//			}
//
//			if batchOutput.LastEvaluatedKey == nil {
//				// We've reached the end
//				break
//			}
//
//			lastKey = batchOutput.LastEvaluatedKey
//			skip -= len(batchOutput.Items)
//		}
//
//		if lastKey != nil {
//			input.ExclusiveStartKey = lastKey
//		}
//	}
//
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	err = attributevalue.UnmarshalListOfMaps(output.Items, &results)
//	if err != nil {
//		return PageResponse[T]{}, err
//	}
//
//	return PageResponse[T]{
//		Contents:         results,
//		NumberOfElements: len(results),
//		TotalElements:    totalElements,
//		TotalPages:       totalPages,
//		Pageable:         pageRequest,
//	}, nil
//}
//
//func (r *DynamoRepository[T]) CountBy(field string, value interface{}) (int64, error) {
//	val, err := attributevalue.Marshal(value)
//	if err != nil {
//		return 0, err
//	}
//
//	input := &dynamodb.QueryInput{
//		TableName:              aws.String(r.tableName),
//		KeyConditionExpression: aws.String("collection = :c"),
//		FilterExpression:       aws.String(fmt.Sprintf("%s = :v", field)),
//		ExpressionAttributeValues: map[string]types.AttributeValue{
//			":c": val,
//			":v": val,
//		},
//		Select: types.SelectCount,
//	}
//
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return 0, err
//	}
//
//	return int64(output.Count), nil
//}
//
//func (r *DynamoRepository[T]) CountByFilters(filters map[string]interface{}) (int64, error) {
//	var doc T
//	collection := doc.GetCollectionName()
//	collectionKey, err := attributevalue.Marshal(collection)
//	if err != nil {
//		return 0, err
//	}
//
//	expr, err := r.buildFilterExpression(filters)
//	if err != nil {
//		return 0, err
//	}
//
//	input := &dynamodb.QueryInput{
//		TableName:                 aws.String(r.tableName),
//		KeyConditionExpression:    aws.String("collection = :c"),
//		FilterExpression:          expr.Filter,
//		ExpressionAttributeNames:  expr.Names,
//		ExpressionAttributeValues: expr.Values,
//		Select:                    types.SelectCount,
//	}
//	input.ExpressionAttributeValues[":c"] = collectionKey
//
//	output, err := r.client.Query(context.Background(), input)
//	if err != nil {
//		return 0, err
//	}
//
//	return int64(output.Count), nil
//}
//
//func (r *DynamoRepository[T]) ExistsBy(field string, value interface{}) (bool, error) {
//	count, err := r.CountBy(field, value)
//	return count > 0, err
//}
//
//func (r *DynamoRepository[T]) ExistsByFilters(filters map[string]interface{}) (bool, error) {
//	count, err := r.CountByFilters(filters)
//	return count > 0, err
//}
//
//func (r *DynamoRepository[T]) buildFilterExpression(filters map[string]interface{}) (*Expression, error) {
//	var conditions []string
//	expressionValues := make(map[string]types.AttributeValue)
//	expressionNames := make(map[string]string)
//
//	i := 1
//	for field, value := range filters {
//		placeholder := fmt.Sprintf(":v%d", i)
//		nameKey := fmt.Sprintf("#n%d", i)
//		conditions = append(conditions, fmt.Sprintf("%s = %s", nameKey, placeholder))
//
//		val, err := attributevalue.Marshal(value)
//		if err != nil {
//			return nil, err
//		}
//		expressionValues[placeholder] = val
//		expressionNames[nameKey] = field
//		i++
//	}
//
//	filterExpr := strings.Join(conditions, " AND ")
//	return &Expression{
//		Filter: aws.String(filterExpr),
//		Names:  expressionNames,
//		Values: expressionValues,
//	}, nil
//}
//
//func min(a, b int) int {
//	if a < b {
//		return a
//	}
//	return b
//}
