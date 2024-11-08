package GinBoot

import (
	"context"
	"fmt"
	"github.com/klass-lk/ginboot/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"math"
	"time"
)

type MongoRepository[T types.Document] struct {
	collection *mongo.Collection
}

func NewMongoRepository[T types.Document](db *mongo.Database) *MongoRepository[T] {
	var doc T
	return &MongoRepository[T]{
		collection: db.Collection(doc.GetCollectionName()),
	}
}

func (r *MongoRepository[T]) Query() *mongo.Collection {
	return r.collection
}

func (r *MongoRepository[T]) FindById(id interface{}) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result T
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&result)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (r *MongoRepository[T]) FindAllById(idList []string) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	filter := bson.M{"_id": bson.M{"$in": idList}}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var result []T
	for cursor.Next(ctx) {
		var doc T
		err := cursor.Decode(&doc)
		if err != nil {
			return nil, err
		}
		result = append(result, doc)
	}
	return result, nil
}

func (r *MongoRepository[T]) Save(doc T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.collection.InsertOne(ctx, doc)
	return err
}

func (r *MongoRepository[T]) SaveOrUpdate(doc T) error {
	exists, err := r.ExistsBy("_id", doc.GetID())
	if err != nil {
		return err
	}
	if exists {
		return r.Update(doc)
	}
	return r.Save(doc)
}

func (r *MongoRepository[T]) SaveAll(sms []T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var documents []interface{}
	for _, sm := range sms {
		documents = append(documents, sm)
	}
	_, err := r.collection.InsertMany(ctx, documents)
	if err != nil {
		return err
	}
	return nil
}

func (r *MongoRepository[T]) Update(doc T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": doc.GetID()}
	update := bson.M{"$set": doc}
	_, err := r.collection.UpdateOne(ctx, filter, update)
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

	filter := bson.M{}
	for field, value := range filters {
		filter[field] = value
	}

	var result T
	err := r.collection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (r *MongoRepository[T]) FindBy(field string, value interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{field: value}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []T
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository[T]) FindByMultiple(filters map[string]interface{}) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{}
	for field, value := range filters {
		filter[field] = value
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []T
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository[T]) FindAll(opts ...*options.FindOptions) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := r.collection.Find(ctx, bson.M{}, opts...)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []T
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository[T]) FindAllPaginated(pageRequest types.PageRequest) (types.PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	skip := bson.D{{"$skip", (pageRequest.Page - 1) * pageRequest.Size}}
	limit := bson.D{{"$limit", pageRequest.Size}}
	sort := bson.D{{"$sort", bson.D{{pageRequest.Sort.Field, pageRequest.Sort.Direction}}}}
	aggregationPipeline := mongo.Pipeline{
		skip, limit, sort,
	}

	cursor, err := r.Query().Aggregate(ctx, aggregationPipeline)
	if err != nil {
		fmt.Println("Error running aggregation:", err)
		return types.PageResponse[T]{}, err
	}
	defer cursor.Close(ctx)

	var results []T
	if err = cursor.All(ctx, &results); err != nil {
		fmt.Println("Error retrieving results:", err)
		return types.PageResponse[T]{}, err
	}
	count, err := r.Query().CountDocuments(ctx, bson.M{})
	if err != nil {
		return types.PageResponse[T]{}, err
	}
	totalPages := int(math.Ceil(float64(count) / float64(pageRequest.Size)))
	pageResponse := types.PageResponse[T]{
		Contents:         results,
		NumberOfElements: pageRequest.Size,
		Pageable:         pageRequest,
		TotalPages:       totalPages,
		TotalElements:    int(count),
	}
	return pageResponse, nil
}

func (r *MongoRepository[T]) FindByPaginated(pageRequest types.PageRequest, filters map[string]interface{}) (types.PageResponse[T], error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{}
	for field, value := range filters {
		filter[field] = value
	}

	match := bson.D{{"$match", filter}}
	skip := bson.D{{"$skip", (pageRequest.Page - 1) * pageRequest.Size}}
	limit := bson.D{{"$limit", pageRequest.Size}}
	sort := bson.D{{"$sort", bson.D{{pageRequest.Sort.Field, pageRequest.Sort.Direction}}}}
	aggregationPipeline := mongo.Pipeline{
		match, sort, skip, limit,
	}

	cursor, err := r.Query().Aggregate(ctx, aggregationPipeline)
	if err != nil {
		fmt.Println("Error running aggregation:", err)
		return types.PageResponse[T]{}, err
	}
	defer cursor.Close(ctx)

	var results []T
	if err = cursor.All(ctx, &results); err != nil {
		fmt.Println("Error retrieving results:", err)
		return types.PageResponse[T]{}, err
	}
	count, err := r.Query().CountDocuments(ctx, filter)
	if err != nil {
		return types.PageResponse[T]{}, err
	}
	totalPages := int(math.Ceil(float64(count) / float64(pageRequest.Size)))
	pageResponse := types.PageResponse[T]{
		Contents:         results,
		NumberOfElements: pageRequest.Size,
		Pageable:         pageRequest,
		TotalPages:       totalPages,
		TotalElements:    int(count),
	}
	return pageResponse, nil
}

func (r *MongoRepository[T]) CountBy(field string, value interface{}) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := r.collection.CountDocuments(ctx, bson.M{field: value})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MongoRepository[T]) CountByFilters(filters map[string]interface{}) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{}
	for field, value := range filters {
		filter[field] = value
	}
	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MongoRepository[T]) ExistsBy(field string, value interface{}) (bool, error) {
	count, err := r.CountBy(field, value)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *MongoRepository[T]) ExistsByFilters(filters map[string]interface{}) (bool, error) {
	count, err := r.CountByFilters(filters)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
