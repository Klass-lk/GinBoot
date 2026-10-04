package mongo

import (
	"bytes"
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
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

// InsertJSON inserts documents written as MongoDB Extended JSON (relaxed or
// canonical): one object, or an array of objects. Types JSON cannot express
// directly use their Extended JSON forms, e.g. {"$date": "2026-01-02T00:00:00Z"}.
func (a *MongoAdapter) InsertJSON(collection string, data []byte) error {
	data = bytes.TrimSpace(data)
	var docs []interface{}
	if len(data) > 0 && data[0] == '[' {
		var wrapper bson.D
		if err := bson.UnmarshalExtJSON(append(append([]byte(`{"d":`), data...), '}'), false, &wrapper); err != nil {
			return fmt.Errorf("mongo: invalid Extended JSON: %w", err)
		}
		arr, ok := wrapper[0].Value.(bson.A)
		if !ok {
			return fmt.Errorf("mongo: expected an array of documents")
		}
		docs = arr
	} else {
		var doc bson.D
		if err := bson.UnmarshalExtJSON(data, false, &doc); err != nil {
			return fmt.Errorf("mongo: invalid Extended JSON: %w", err)
		}
		docs = []interface{}{doc}
	}
	if len(docs) == 0 {
		return nil
	}
	_, err := a.DB.Collection(collection).InsertMany(context.Background(), docs)
	return err
}
