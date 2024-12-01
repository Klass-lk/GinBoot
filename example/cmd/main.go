package main

import (
	"log"
	"time"

	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/controller"
	"github.com/klass-lk/ginboot/example/internal/repository"
	"github.com/klass-lk/ginboot/example/internal/service"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Initialize MongoDB client
	client, err := mongo.Connect(nil, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(nil)

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

	// Initialize and register controllers
	postController := controller.NewPostController(postService)

	server.RegisterController("/posts", postController)

	if err := server.Start(8080); err != nil {
		log.Fatal(err)
	}
}
