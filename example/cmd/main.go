package main

import (
	"context"
	"log"

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

	// Setup Gin router
	r := gin.Default()

	// Create a new server instance
	server := ginboot.New()

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
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
