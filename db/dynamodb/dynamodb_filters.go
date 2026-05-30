package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func (r *DynamoDBRepository[T]) buildFilterExpression(filters map[string]interface{}) (*string, map[string]types.AttributeValue, map[string]string, error) {
	if len(filters) == 0 {
		return nil, nil, nil, nil
	}

	var expressions []string
	expressionAttributeValues := make(map[string]types.AttributeValue)
	expressionAttributeNames := make(map[string]string)

	var dummy T
	tType := reflect.TypeOf(dummy)
	if tType.Kind() == reflect.Ptr {
		tType = tType.Elem()
	}

	i := 0
	for field, value := range filters {
		attrName := field
		if structField, ok := tType.FieldByName(field); ok {
			if tag := structField.Tag.Get("json"); tag != "" {
				parts := strings.Split(tag, ",")
				if parts[0] != "" && parts[0] != "-" {
					attrName = parts[0]
				}
			} else if tag := structField.Tag.Get("dynamodbav"); tag != "" {
				parts := strings.Split(tag, ",")
				if parts[0] != "" && parts[0] != "-" {
					attrName = parts[0]
				}
			}
		}

		placeholderName := fmt.Sprintf("#f%d", i)
		expressionAttributeNames[placeholderName] = attrName

		if opMap, ok := value.(map[string]interface{}); ok {
			for op, opValue := range opMap {
				placeholderVal := fmt.Sprintf(":v%d", i)

				if t, ok := opValue.(time.Time); ok {
					opValue = t.Unix()
				}

				av, err := attributevalue.MarshalWithOptions(opValue, func(options *attributevalue.EncoderOptions) {
					options.TagKey = "json"
				})
				if err != nil {
					return nil, nil, nil, err
				}

				expressionAttributeValues[placeholderVal] = av

				switch op {
				case "$gte":
					expressions = append(expressions, fmt.Sprintf("%s >= %s", placeholderName, placeholderVal))
				case "$gt":
					expressions = append(expressions, fmt.Sprintf("%s > %s", placeholderName, placeholderVal))
				case "$lte":
					expressions = append(expressions, fmt.Sprintf("%s <= %s", placeholderName, placeholderVal))
				case "$lt":
					expressions = append(expressions, fmt.Sprintf("%s < %s", placeholderName, placeholderVal))
				case "$ne":
					expressions = append(expressions, fmt.Sprintf("%s <> %s", placeholderName, placeholderVal))
				default:
					expressions = append(expressions, fmt.Sprintf("%s = %s", placeholderName, placeholderVal))
				}
				i++
			}
		} else {
			placeholderVal := fmt.Sprintf(":v%d", i)
			if t, ok := value.(time.Time); ok {
				value = t.Unix()
			}

			av, err := attributevalue.MarshalWithOptions(value, func(options *attributevalue.EncoderOptions) {
				options.TagKey = "json"
			})
			if err != nil {
				return nil, nil, nil, err
			}
			expressionAttributeValues[placeholderVal] = av
			expressions = append(expressions, fmt.Sprintf("%s = %s", placeholderName, placeholderVal))
			i++
		}
	}

	expr := strings.Join(expressions, " AND ")
	return &expr, expressionAttributeValues, expressionAttributeNames, nil
}

func (r *DynamoDBRepository[T]) FindOneBy(field string, value interface{}, partitionKey string) (T, error) {
	filters := map[string]interface{}{field: value}
	return r.FindOneByFilters(filters, partitionKey)
}

func (r *DynamoDBRepository[T]) FindOneByFilters(filters map[string]interface{}, partitionKey string) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey

	filterExpr, exprVals, exprNames, err := r.buildFilterExpression(filters)
	if err != nil {
		return entity, err
	}

	if exprVals == nil {
		exprVals = make(map[string]types.AttributeValue)
	}
	exprVals[":pk"] = &types.AttributeValueMemberS{Value: pk}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(dynamoConfig.TableName),
		IndexName:                 aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression:    aws.String("pk = :pk"),
		ExpressionAttributeValues: exprVals,
		ScanIndexForward:          aws.Bool(false),
		Limit:                     aws.Int32(1),
	}

	if filterExpr != nil {
		input.FilterExpression = filterExpr
		input.ExpressionAttributeNames = exprNames
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return entity, err
		}

		if len(output.Items) > 0 {
			var temp T
			err = UnmarshalLegacyOrNative(output.Items[0], &temp)
			if err != nil {
				return entity, err
			}
			return temp, nil
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return entity, errors.New("document not found")
}

func (r *DynamoDBRepository[T]) FindBy(field string, value interface{}, partitionKey string) ([]T, error) {
	filters := map[string]interface{}{field: value}
	return r.FindByFilters(filters, partitionKey)
}

func (r *DynamoDBRepository[T]) FindByFilters(filters map[string]interface{}, partitionKey string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var results []T
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey

	filterExpr, exprVals, exprNames, err := r.buildFilterExpression(filters)
	if err != nil {
		return nil, err
	}

	if exprVals == nil {
		exprVals = make(map[string]types.AttributeValue)
	}
	exprVals[":pk"] = &types.AttributeValueMemberS{Value: pk}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(dynamoConfig.TableName),
		IndexName:                 aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression:    aws.String("pk = :pk"),
		ExpressionAttributeValues: exprVals,
		ScanIndexForward:          aws.Bool(false),
	}

	if filterExpr != nil {
		input.FilterExpression = filterExpr
		input.ExpressionAttributeNames = exprNames
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

	if results == nil {
		results = make([]T, 0)
	}
	return results, nil
}

func (r *DynamoDBRepository[T]) CountBy(field string, value interface{}, partitionKey string) (int64, error) {
	filters := map[string]interface{}{field: value}
	return r.CountByFilters(filters, partitionKey)
}

func (r *DynamoDBRepository[T]) CountByFilters(filters map[string]interface{}, partitionKey string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var count int64
	var entity T
	pk := r.getPK(entity) + "#" + partitionKey

	filterExpr, exprVals, exprNames, err := r.buildFilterExpression(filters)
	if err != nil {
		return 0, err
	}

	if exprVals == nil {
		exprVals = make(map[string]types.AttributeValue)
	}
	exprVals[":pk"] = &types.AttributeValueMemberS{Value: pk}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(dynamoConfig.TableName),
		IndexName:                 aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression:    aws.String("pk = :pk"),
		ExpressionAttributeValues: exprVals,
		Select:                    types.SelectCount,
	}

	if filterExpr != nil {
		input.FilterExpression = filterExpr
		input.ExpressionAttributeNames = exprNames
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return 0, err
		}

		count += int64(output.Count)

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return count, nil
}

func (r *DynamoDBRepository[T]) ExistsBy(field string, value interface{}, partitionKey string) (bool, error) {
	filters := map[string]interface{}{field: value}
	return r.ExistsByFilters(filters, partitionKey)
}

func (r *DynamoDBRepository[T]) ExistsByFilters(filters map[string]interface{}, partitionKey string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var entity T
	pk := r.getPK(entity) + "#" + partitionKey

	filterExpr, exprVals, exprNames, err := r.buildFilterExpression(filters)
	if err != nil {
		return false, err
	}

	if exprVals == nil {
		exprVals = make(map[string]types.AttributeValue)
	}
	exprVals[":pk"] = &types.AttributeValueMemberS{Value: pk}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(dynamoConfig.TableName),
		IndexName:                 aws.String(PKCreatedAtSortIndex),
		KeyConditionExpression:    aws.String("pk = :pk"),
		ExpressionAttributeValues: exprVals,
		Select:                    types.SelectCount,
		Limit:                     aws.Int32(1),
	}

	if filterExpr != nil {
		input.FilterExpression = filterExpr
		input.ExpressionAttributeNames = exprNames
	}

	for {
		output, err := r.client.Query(ctx, input)
		if err != nil {
			return false, err
		}

		if output.Count > 0 {
			return true, nil
		}

		if output.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return false, nil
}

func (r *DynamoDBRepository[T]) DeleteBy(field string, value interface{}, partitionKey string) error {
	filters := map[string]interface{}{field: value}
	return r.DeleteByFilters(filters, partitionKey)
}

func (r *DynamoDBRepository[T]) DeleteByFilters(filters map[string]interface{}, partitionKey string) error {
	items, err := r.FindByFilters(filters, partitionKey)
	if err != nil {
		return err
	}

	var ids []string
	for _, item := range items {
		id, err := r.getGinbootId(item)
		if err == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return nil
	}

	return r.DeleteAll(ids, partitionKey)
}
