package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	memory "github.com/klass-lk/ginboot/db/inmemory"
	dbMongo "github.com/klass-lk/ginboot/db/mongo"
	"github.com/klass-lk/ginboot/example/internal/controller"
	"github.com/klass-lk/ginboot/example/internal/model"
	"github.com/klass-lk/ginboot/example/internal/service"
	"github.com/klass-lk/ginboot/storage/s3"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Initialize server (automatically loads .env, ginboot.yml, and configures telemetry)
	server := ginboot.New()
	cfg := server.Config()

	log.Printf("[Ginboot App] Started - Port: %d, BasePath: %s", cfg.Ginboot.Server.Port, cfg.Ginboot.Server.BasePath)

	// Set base path from loaded config
	if cfg.Ginboot.Server.BasePath != "" {
		server.SetBasePath(cfg.Ginboot.Server.BasePath)
	}

	// Initialize MongoDB client for caching
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.TODO())

	// Initialize repositories & services
	postRepo := memory.NewInMemoryRepository[model.Post]()
	postService := service.NewPostService(postRepo)

	// Configure CORS with custom settings
	server.CustomCORS(
		[]string{"http://localhost:3000", "https://yourdomain.com"},
		[]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		[]string{"Origin", "Content-Type", "Authorization", "Accept"},
		24*time.Hour,
	)

	// Initialize Cache Service (Mongo)
	cacheRepo := dbMongo.NewMongoRepository[ginboot.CacheEntry](client.Database("example"), "cache_entries")
	cacheService := dbMongo.NewMongoCacheService(cacheRepo)

	tagGen := func(c *gin.Context) []string {
		return []string{"posts"}
	}

	cacheMiddleware := ginboot.CacheMiddleware(cacheService, time.Minute, tagGen, nil)

	// Initialize and register controllers
	postController := controller.NewPostController(postService, cacheService, cacheMiddleware)
	server.RegisterController("/posts", postController)

	fileService := s3.NewS3FileService(context.Background(), "example-bucket", "./local", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", "3600")
	server.BindFileService(fileService)

	if err := server.Start(cfg.Ginboot.Server.Port); err != nil {
		log.Fatal(err)
	}
}
