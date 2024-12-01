package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/controller"
	"github.com/klass-lk/ginboot/example/internal/middleware"
	"github.com/klass-lk/ginboot/example/internal/repository"
	"github.com/klass-lk/ginboot/example/internal/service"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Connect to MongoDB
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database("blog")

	// Initialize repositories
	postRepo := repository.NewPostRepository(db)

	// Initialize services
	postService := service.NewPostService(postRepo)

	// Initialize controllers
	postController := controller.NewPostController(postService)

	// Create a new server instance
	server := ginboot.New()

	// Configure CORS with custom settings
	server.CustomCORS(
		[]string{"http://localhost:3000", "https://yourdomain.com"},  // Allow specific origins
		[]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},          // Allow specific methods
		[]string{"Origin", "Content-Type", "Authorization", "Accept"}, // Allow specific headers
		24*time.Hour,                                                 // Max age of preflight requests
	)

	// Register controllers with their base paths
	server.RegisterControllers(postController)

	apiGroup := ginboot.RouterGroup{
		Path:       "/api/v1",
		Middleware: []gin.HandlerFunc{middleware.AuthMiddleware()},
		Controllers: []ginboot.Controller{
			&controller.PostController{},
		},
	}

	server.RegisterGroups(apiGroup)

	// Start server
	if err := server.Start(8080); err != nil {
		log.Fatal(err)
	}
}
