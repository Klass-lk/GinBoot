package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/klass-lk/ginboot"
	dbMongo "github.com/klass-lk/ginboot/db/mongo"
	"github.com/klass-lk/ginboot/example/internal/controller"
	"github.com/klass-lk/ginboot/example/internal/model"
	"github.com/klass-lk/ginboot/example/internal/service"
	"github.com/klass-lk/ginboot/storage/s3"
	"github.com/klass-lk/ginboot/telemetry"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Load environment variables from .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found or failed to load: %v", err)
	}

	// Initialize OpenTelemetry
	// By default, this uses standard OTLP environment variables.
	// You can configure Grafana Cloud endpoint via OTEL_EXPORTER_OTLP_ENDPOINT
	shutdown, err := telemetry.Setup(context.Background(), "ginboot-example", "v1.0.0")
	if err != nil {
		log.Printf("Failed to setup telemetry: %v", err)
	}
	defer func() {
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
	}()

	// Initialize MongoDB client
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.TODO())

	// Initialize repositories
	postRepo := dbMongo.NewMongoRepository[model.Post](client.Database("example"), "posts")

	// Initialize services
	postService := service.NewPostService(postRepo)

	// Initialize server with telemetry
	server := ginboot.New()
	telemetry.Instrument(server, "ginboot-example", nil)

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
	cacheRepo := dbMongo.NewMongoRepository[ginboot.CacheEntry](client.Database("example"), "cache_entries")
	cacheService := dbMongo.NewMongoCacheService(cacheRepo)

	// Tag Generator: Tag all requests to /posts as "posts"
	// In a real app, this would be more sophisticated (e.g. tagging by ID)
	tagGen := func(c *gin.Context) []string {
		return []string{"posts"}
	}

	cacheMiddleware := ginboot.CacheMiddleware(cacheService, time.Minute, tagGen, nil) // nil keyGen use default

	// Initialize and register controllers
	postController := controller.NewPostController(postService, cacheService, cacheMiddleware)

	server.RegisterController("/posts", postController)

	fileService := s3.NewS3FileService(context.Background(), "example-bucket", "./local", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", "3600")
	server.BindFileService(fileService)

	if err := server.Start(8080); err != nil {
		log.Fatal(err)
	}
}
