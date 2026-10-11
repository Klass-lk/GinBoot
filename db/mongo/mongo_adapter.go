package mongo

import (
	"bytes"
	"context"
	"encoding/json"
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

// FindJSON returns the documents of a collection matching filter, written
// as MongoDB Extended JSON ({} or empty for all), as relaxed Extended JSON
// decoded into plain values: dates become {"$date": "..."} objects.
func (a *MongoAdapter) FindJSON(collection string, filter []byte) ([]any, error) {
	query := bson.D{}
	if f := bytes.TrimSpace(filter); len(f) > 0 {
		if err := bson.UnmarshalExtJSON(f, false, &query); err != nil {
			return nil, fmt.Errorf("mongo: invalid Extended JSON filter: %w", err)
		}
	}
	cur, err := a.DB.Collection(collection).Find(context.Background(), query)
	if err != nil {
		return nil, err
	}
	var raws []bson.Raw
	if err := cur.All(context.Background(), &raws); err != nil {
		return nil, err
	}
	out := make([]any, 0, len(raws))
	for _, raw := range raws {
		ext, err := bson.MarshalExtJSON(raw, false, false)
		if err != nil {
			return nil, err
		}
		var doc any
		if err := json.Unmarshal(ext, &doc); err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}
