package ginboot

import "sync"

var (
	once         sync.Once
	dynamoConfig *DynamoDBConfig
)

type DynamoDBConfig struct {
	TableName         string
	SkipTableCreation bool
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
