package ginboot

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"golang.org/x/net/context"
)

type DynamoConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string
	Profile         string
}

func NewDynamoConfig() *DynamoConfig {
	return &DynamoConfig{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	}
}

func (c *DynamoConfig) WithRegion(region string) *DynamoConfig {
	c.Region = region
	return c
}

func (c *DynamoConfig) WithCredentials(accessKeyID, secretAccessKey string) *DynamoConfig {
	c.AccessKeyID = accessKeyID
	c.SecretAccessKey = secretAccessKey
	return c
}

func (c *DynamoConfig) WithEndpoint(endpoint string) *DynamoConfig {
	c.Endpoint = endpoint
	return c
}

func (c *DynamoConfig) WithProfile(profile string) *DynamoConfig {
	c.Profile = profile
	return c
}

func (c *DynamoConfig) Connect() (*dynamodb.Client, error) {
	ctx := context.Background()
	var cfg aws.Config
	var err error

	if c.Profile != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(c.Region),
			config.WithSharedConfigProfile(c.Profile),
		)
	} else if c.AccessKeyID != "" && c.SecretAccessKey != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(c.Region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				c.AccessKeyID,
				c.SecretAccessKey,
				"",
			)),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(c.Region))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	options := &dynamodb.Options{
		Credentials: cfg.Credentials,
		Region:      cfg.Region,
	}

	if c.Endpoint != "" {
		options.EndpointResolver = dynamodb.EndpointResolverFromURL(c.Endpoint)
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		*o = *options
	}), nil
}
