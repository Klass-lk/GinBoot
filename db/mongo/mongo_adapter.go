package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
)

// MongoAdapter is a DBAdapter for MongoDB.
type MongoAdapter struct {
	DB *mongo.Database
}

func (a *MongoAdapter) Insert(collection string, doc interface{}) error {
	_, err := a.DB.Collection(collection).InsertOne(context.Background(), doc)
	return err
}

func (a *MongoAdapter) Clear(collection string) error {
	return a.DB.Collection(collection).Drop(context.Background())
}
