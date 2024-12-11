package ginboot

import (
	"context"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoRepository[T Document] struct {
	collection *mongo.Collection
}

func NewMongoRepository[T Document](db *mongo.Database) *MongoRepository[T] {
	var doc T
	return &MongoRepository[T]{
		collection: db.Collection(doc.GetCollectionName()),
	}
}

func (r *MongoRepository[T]) FindById(id string) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&result)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (r *MongoRepository[T]) FindAllById(ids []string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	filter := bson.M{"_id": bson.M{"$in": ids}}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []T
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *MongoRepository[T]) Save(doc T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.collection.InsertOne(ctx, doc)
	return err
}

func (r *MongoRepository[T]) SaveOrUpdate(doc T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": getDocumentID(doc)}, doc, options.Replace().SetUpsert(true))
	return err
}

func (r *MongoRepository[T]) SaveAll(docs []T) error {
	if len(docs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var operations []mongo.WriteModel
	for _, doc := range docs {
		operation := mongo.NewReplaceOneModel().SetFilter(bson.M{"_id": getDocumentID(doc)}).SetReplacement(doc).SetUpsert(true)
		operations = append(operations, operation)
	}
	_, err := r.collection.BulkWrite(ctx, operations)
	return err
}

func (r *MongoRepository[T]) Update(doc T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": getDocumentID(doc)}, doc)
	return err
}

func (r *MongoRepository[T]) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *MongoRepository[T]) FindOneBy(field string, value interface{}) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	err := r.collection.FindOne(ctx, bson.M{field: value}).Decode(&result)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (r *MongoRepository[T]) FindOneByFilters(filters map[string]interface{}) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	err := r.collection.FindOne(ctx, filters).Decode(&result)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (r *MongoRepository[T]) FindBy(field string, value interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := r.collection.Find(ctx, bson.M{field: value})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []T
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *MongoRepository[T]) FindByFilters(filters map[string]interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := r.collection.Find(ctx, filters)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []T
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *MongoRepository[T]) FindAll(findOpts ...interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mongoFindOpts []*options.FindOptions
	for _, opt := range findOpts {
		if fo, ok := opt.(*options.FindOptions); ok {
			mongoFindOpts = append(mongoFindOpts, fo)
		}
	}

	cursor, err := r.collection.Find(ctx, bson.M{}, mongoFindOpts...)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []T
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *MongoRepository[T]) FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	skip := int64((pageRequest.Page - 1) * pageRequest.Size)
	limit := int64(pageRequest.Size)

	total, err := r.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return PageResponse[T]{}, err
	}

	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit)

	if pageRequest.Sort.Field != "" {
		direction := 1
		if pageRequest.Sort.Direction < 0 {
			direction = -1
		}
		opts.SetSort(bson.D{{Key: pageRequest.Sort.Field, Value: direction}})
	}

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return PageResponse[T]{}, err
	}
	defer cursor.Close(ctx)

	var items []T
	if err = cursor.All(ctx, &items); err != nil {
		return PageResponse[T]{}, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageRequest.Size)))

	return PageResponse[T]{
		Contents:         items,
		NumberOfElements: len(items),
		Pageable:         pageRequest,
		TotalElements:    int(total),
		TotalPages:       totalPages,
	}, nil
}

func (r *MongoRepository[T]) FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	skip := int64((pageRequest.Page - 1) * pageRequest.Size)
	limit := int64(pageRequest.Size)

	total, err := r.collection.CountDocuments(ctx, filters)
	if err != nil {
		return PageResponse[T]{}, err
	}

	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit)

	if pageRequest.Sort.Field != "" {
		direction := 1
		if pageRequest.Sort.Direction < 0 {
			direction = -1
		}
		opts.SetSort(bson.D{{Key: pageRequest.Sort.Field, Value: direction}})
	}

	cursor, err := r.collection.Find(ctx, filters, opts)
	if err != nil {
		return PageResponse[T]{}, err
	}
	defer cursor.Close(ctx)

	var items []T
	if err = cursor.All(ctx, &items); err != nil {
		return PageResponse[T]{}, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageRequest.Size)))

	return PageResponse[T]{
		Contents:         items,
		NumberOfElements: len(items),
		Pageable:         pageRequest,
		TotalElements:    int(total),
		TotalPages:       totalPages,
	}, nil
}

func (r *MongoRepository[T]) CountBy(field string, value interface{}) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return r.collection.CountDocuments(ctx, bson.M{field: value})
}

func (r *MongoRepository[T]) CountByFilters(filters map[string]interface{}) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return r.collection.CountDocuments(ctx, filters)
}

func (r *MongoRepository[T]) ExistsBy(field string, value interface{}) (bool, error) {
	count, err := r.CountBy(field, value)
	return count > 0, err
}

func (r *MongoRepository[T]) ExistsByFilters(filters map[string]interface{}) (bool, error) {
	count, err := r.CountByFilters(filters)
	return count > 0, err
}

func (r *MongoRepository[T]) Query() *mongo.Collection {
	return r.collection
}
