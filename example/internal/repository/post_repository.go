package repository

import (
	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/model"
	"go.mongodb.org/mongo-driver/mongo"
)

type PostRepository struct {
	*ginboot.MongoRepository[model.Post]
}

func NewPostRepository(database *mongo.Database) *PostRepository {
	return &PostRepository{
		MongoRepository: ginboot.NewMongoRepository[model.Post](database, "posts"),
	}
}
