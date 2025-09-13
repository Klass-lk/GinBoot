package ginboot

import (
	"context"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// NewDynamoDBClient creates and returns a new DynamoDB client.
// It takes a region as input and loads AWS configuration.
func NewDynamoDBClient(region string) (*dynamodb.Client, error) {
	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(), awsConfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return dynamodb.NewFromConfig(cfg), nil
}
