# Testing with Ginboot

Ginboot provides a powerful and flexible test suite based on [Godog](https://github.com/cucumber/godog), the official Cucumber BDD framework for Go. This allows you to write human-readable feature files to test your application's behavior.

## Overview

The test suite is designed to be database-agnostic, allowing you to test your application with different databases like MongoDB, MySQL, or DynamoDB. It uses [testcontainers-go](https://github.com/testcontainers/testcontainers-go) to spin up database instances in Docker containers for your tests, providing a clean and isolated environment for each test run.

## Setting up the Test Environment

To start writing tests for your Ginboot application, you need to set up a test file and configure the test environment.

### 1. Create a Test File

Create a `*_test.go` file in your application's directory (e.g., `main_test.go`). This file will be the entry point for running your tests.

### 2. Write Feature Files

Create a `features` directory in your application's directory and write your feature files using Gherkin syntax. These files describe your application's behavior in a plain, human-readable format.

**Example: `features/posts.feature`**

```gherkin
Feature: Posts
  Manage posts in the application

  Background: Setup
    Given document "posts" has the following items
      | id | title  | content   |
      | 1  | Post 1 | Content 1 |

  Scenario: Create a new post
    When I send a POST request to "/posts" with body
      | title    | content     |
      | New Post | New Content |
    Then the response status should be 201
    And the response should contain an item with
      | title    | content     |
      | New Post | New Content |
```

### Predefined Steps

The Ginboot test suite provides a set of predefined steps that you can use in your feature files:

*   `Given document "<document_name>" has the following items`
*   `When I send a POST request to "<path>" with body`
*   `When I send a GET request to "<path>"`
*   `When I send a PUT request to "<path>" with body`
*   `When I send a DELETE request to "<path>"`
*   `Then the response status should be <status_code>`
*   `And the response should contain an item with`

## Writing Test Code

In your `*_test.go` file, you'll need to initialize the Ginboot `TestSuite` and configure it for your application.

### 1. Initialize the TestSuite

Create a `TestFeatures` function and initialize the `ginboot.TestSuite`:

```go
func TestFeatures(t *testing.T) {
    testSuite := &ginboot.TestSuite{T: t, DbSeeders: make(map[string]ginboot.DBSeeder)}
    // ...
}
```

### 2. Set up the Database

The test suite uses `testcontainers-go` to manage database containers. You'll need to create a function to set up your database and another to set up your router.

```go
func setupTestDB(ctx context.Context) *mongo.Database {
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

    return client.Database("test")
}

func setupRouter(ctx context.Context, db *mongo.Database) (*gin.Engine, func()) {
    // Create a new repository
    postRepo := repository.NewPostRepository(db)

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
        if err := db.Client().Disconnect(ctx); err != nil {
            panic(err)
        }
    }
}
```

### 3. Configure the DBAdapter and Seeder

To make the database interactions generic, the test suite uses a `DBAdapter` interface. You'll need to create an adapter for your database and pass it to the `GenericDBSeeder`.

```go
// Create a generic seeder
adapter := &ginboot.MongoAdapter{DB: db}
seeder := ginboot.NewGenericDBSeeder(adapter)

// Register your document types with the seeder
seeder.Register("posts", func() interface{} { return &model.Post{} })

// Register the seeder with the test suite
testSuite.RegisterDBSeeder("posts", seeder)
```

### 4. Run the Test Suite

Finally, configure the `godog.Options` and run the `godog.TestSuite`.

```go
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
```

## Complete Example

Here's a complete example based on the `example` project:

```go
package main

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

func setupTestDB(ctx context.Context) *mongo.Database {
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

	return client.Database("test")
}

func setupRouter(ctx context.Context, db *mongo.Database) (*gin.Engine, func()) {
	// Create a new repository
	postRepo := repository.NewPostRepository(db)

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
		if err := db.Client().Disconnect(ctx); err != nil {
			panic(err)
		}
	}
}

func TestFeatures(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(ctx)
	router, cleanup := setupRouter(ctx, db)
	defer cleanup()

	testSuite := &ginboot.TestSuite{T: t, DbSeeders: make(map[string]ginboot.DBSeeder)}

	adapter := &ginboot.MongoAdapter{DB: db}
	seeder := ginboot.NewGenericDBSeeder(adapter)

	seeder.Register("posts", func() interface{} { return &model.Post{} })
	testSuite.RegisterDBSeeder("posts", seeder)

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
