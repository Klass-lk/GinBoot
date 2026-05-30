package dynamodb

import "sync"

var (
	once         sync.Once
	dynamoConfig *DynamoDBConfig
)

type GSIConfig struct {
	IndexName string
	HashKey   string
	SortKey   string
}

type DynamoDBConfig struct {
	TableName         string
	SkipTableCreation bool
	GSIs              []GSIConfig
}

func NewDynamoDBConfig() *DynamoDBConfig {
	once.Do(func() {
		dynamoConfig = &DynamoDBConfig{}
	})
	return dynamoConfig
}

func (c *DynamoDBConfig) WithTableName(name string) *DynamoDBConfig {
	c.TableName = name
	return c
}

func (c *DynamoDBConfig) WithSkipTableCreation(skip bool) *DynamoDBConfig {
	c.SkipTableCreation = skip
	return c
}

func (c *DynamoDBConfig) WithGSI(indexName, hashKey, sortKey string) *DynamoDBConfig {
	c.GSIs = append(c.GSIs, GSIConfig{IndexName: indexName, HashKey: hashKey, SortKey: sortKey})
	return c
}

