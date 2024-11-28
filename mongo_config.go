package ginboot

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Database string
	Options  map[string]string
}

func NewMongoConfig() *MongoConfig {
	return &MongoConfig{
		Host:    "localhost",
		Port:    27017,
		Options: make(map[string]string),
	}
}

func (c *MongoConfig) WithCredentials(username, password string) *MongoConfig {
	c.Username = username
	c.Password = password
	return c
}

func (c *MongoConfig) WithHost(host string, port int) *MongoConfig {
	c.Host = host
	c.Port = port
	return c
}

func (c *MongoConfig) WithDatabase(database string) *MongoConfig {
	c.Database = database
	return c
}

func (c *MongoConfig) WithOption(key, value string) *MongoConfig {
	c.Options[key] = value
	return c
}

func (c *MongoConfig) BuildURI() string {
	var auth string
	if c.Username != "" && c.Password != "" {
		auth = fmt.Sprintf("%s:%s@", c.Username, c.Password)
	}

	uri := fmt.Sprintf("mongodb://%s%s:%d", auth, c.Host, c.Port)

	if len(c.Options) > 0 {
		uri += "?"
		first := true
		for key, value := range c.Options {
			if !first {
				uri += "&"
			}
			uri += fmt.Sprintf("%s=%s", key, value)
			first = false
		}
	}

	return uri
}

func (c *MongoConfig) Connect() (*mongo.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(c.BuildURI())

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	return client.Database(c.Database), nil
}
