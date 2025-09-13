package ginboot

import "sync"

var (
	once   sync.Once
	config *DynamoDBConfig
)

type DynamoDBConfig struct {
	TableName string
}

func NewDynamoDBConfig() *DynamoDBConfig {
	once.Do(func() {
		config = &DynamoDBConfig{}
	})
	return config
}

func (c *DynamoDBConfig) WithTableName(name string) *DynamoDBConfig {
	c.TableName = name
	return c
}
