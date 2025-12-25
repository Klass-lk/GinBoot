package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/controller"
	"github.com/klass-lk/ginboot/example/internal/repository"
	"github.com/klass-lk/ginboot/example/internal/service"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Initialize MongoDB client
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.TODO())

	// Initialize repositories
	postRepo := repository.NewPostRepository(client.Database("example"))

	// Initialize services
	postService := service.NewPostService(postRepo)

	// Initialize server
	server := ginboot.New()

	// Set base path for all routes
	server.SetBasePath("/api/v1")

	// Configure CORS with custom settings
	server.CustomCORS(
		[]string{"http://localhost:3000", "https://yourdomain.com"},   // Allow specific origins
		[]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},           // Allow specific methods
		[]string{"Origin", "Content-Type", "Authorization", "Accept"}, // Allow specific headers
		24*time.Hour, // Max age of preflight requests
	)

	// Initialize Cache Service (Mongo)
	cacheRepo := ginboot.NewMongoRepository[ginboot.CacheEntry](client.Database("example"), "cache_entries")
	cacheService := ginboot.NewMongoCacheService(cacheRepo)

	// Tag Generator: Tag all requests to /posts as "posts"
	// In a real app, this would be more sophisticated (e.g. tagging by ID)
	tagGen := func(c *gin.Context) []string {
		return []string{"posts"}
	}

	cacheMiddleware := ginboot.CacheMiddleware(cacheService, time.Minute, tagGen, nil) // nil keyGen use default

	// Initialize and register controllers
	postController := controller.NewPostController(postService, cacheService, cacheMiddleware)
	cacheController := controller.NewCacheController(cacheService)

	server.RegisterController("/posts", postController)
	server.RegisterController("/cache", cacheController)

	fileService := ginboot.NewS3FileService(context.Background(), "example-bucket", "./local", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", "3600")
	server.BindFileService(fileService)

	if err := server.Start(8080); err != nil {
		log.Fatal(err)
	}
}
