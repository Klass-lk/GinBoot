package example

import (
	"context"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/controller"
	"github.com/klass-lk/ginboot/example/internal/model"
	"github.com/klass-lk/ginboot/example/internal/repository"
	"github.com/klass-lk/ginboot/example/internal/service"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func setupRouter(ctx context.Context) (*gin.Engine, func()) {
	// Start a MongoDB container
	mongodbContainer, err := mongodb.RunContainer(ctx, testcontainers.WithImage("mongo:6"))
	if err != nil {
		panic(err)
	}

	// Get the connection string
	endpoint, err := mongodbContainer.ConnectionString(ctx)
	if err != nil {
		panic(err)
	}

	// Create a new mongo client
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(endpoint))
	if err != nil {
		panic(err)
	}

	// Create a new repository
	postRepo := repository.NewPostRepository(client.Database("test"))

	// Create a new service
	postService := service.NewPostService(postRepo)

	// Create a new controller
	postController := controller.NewPostController(postService)

	// Create a new server
	server := ginboot.New()

	// Register the controller
	server.RegisterController("/posts", postController)

	// Return the router and a cleanup function
	return server.Engine(), func() {
		if err := mongodbContainer.Terminate(ctx); err != nil {
			panic(err)
		}
	}
}

func TestFeatures(t *testing.T) {
	ctx := context.Background()
	router, cleanup := setupRouter(ctx)
	defer cleanup()

	testSuite := &ginboot.TestSuite{T: t, DbSeeders: make(map[string]ginboot.DBSeeder)}

	// Create a generic seeder
	seeder := ginboot.NewGenericDBSeeder()

	// Register your document types with the seeder
	seeder.Register("posts", func() interface{} { return &model.Post{} })

	// Register the seeder with the test suite
	testSuite.RegisterDBSeeder("posts", seeder)

	// Set up the router
	testSuite.Router = router

	opts := godog.Options{
		Format:    "pretty",
		Output:    colors.Colored(&ginboot.TestLogger{T: t}),
		Paths:     []string{"features"},
		Strict:    true,
		Randomize: 0,
	}

	godog.TestSuite{
		Name:                 "ginboot-example",
		TestSuiteInitializer: testSuite.InitializeTestSuite,
		ScenarioInitializer:  testSuite.InitializeScenario,
		Options:              &opts,
	}.Run()
}
