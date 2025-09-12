# Routing

Ginboot provides a flexible and intuitive routing system built on top of Gin, enhancing it with controller-based organization, flexible handler signatures, and integrated context utilities.

## Core Concepts

### Controller Interface

Controllers in Ginboot are responsible for grouping related routes and their handlers. Any struct intended to be a controller must implement the `Controller` interface, which requires a `Register` method:

```go
type Controller interface {
    Register(group *ControllerGroup)
}
```

The `Register` method is where you define all the routes and their associated handlers for that controller.

### ControllerGroup

`ControllerGroup` is a wrapper around Gin's `*gin.RouterGroup`. It provides methods for registering routes (`GET`, `POST`, etc.) and creating nested sub-groups, while also integrating with Ginboot's custom `Context` and `FileService`.

## Registering Controllers and Routes

### Server-Level Registration

You register controllers with the main `Server` instance using `RegisterController`. This method automatically creates a `ControllerGroup` for your controller's base path and calls its `Register` method.

```go
package main

import (
	"log"
	"github.com/klass-lk/ginboot"
	"your-project/internal/controller"
	"your-project/internal/service"
)

// Example UserController
type UserController struct {
	service *service.UserService
}

func NewUserController(s *service.UserService) *UserController {
	return &UserController{service: s}
}

func (c *UserController) ListUsers(ctx *ginboot.Context) ([]string, error) {
	// ... logic to list users ...
	return []string{"user1", "user2"}, nil
}

func (c *UserController) GetUser(ctx *ginboot.Context) (string, error) {
	userID := ctx.Param("id")
	// ... logic to get user by ID ...
	return fmt.Sprintf("User: %s", userID), nil
}

func (c *UserController) Register(group *ginboot.ControllerGroup) {
	group.GET("", c.ListUsers)       // GET /users
	group.GET("/:id", c.GetUser)     // GET /users/:id
}

func main() {
	server := ginboot.New()
	server.SetBasePath("/api/v1")

	userService := service.NewUserService() // Assume this exists
	userController := NewUserController(userService)

	// Register the UserController with a base path of "/users"
	server.RegisterController("/users", userController) // Routes will be /api/v1/users, /api/v1/users/:id

	log.Fatal(server.Start(8080))
}
```

### Base Path Configuration

You can set a global base path for all routes registered with the server using `server.SetBasePath()`. This path will prefix all controller and group paths.

```go
server := ginboot.New()
server.SetBasePath("/api/v1") // All routes will be prefixed with /api/v1
```

### Route Groups

Ginboot allows you to organize routes into groups, which can share a common path prefix and middleware. You can create groups at the server level or nested within other `ControllerGroup`s.

#### Server-Level Groups

Use `server.Group()` to create a new `ControllerGroup`.

```go
package main

import (
	"log"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	"your-project/internal/middleware"
)

func healthCheck() (string, error) {
	return "OK", nil
}

func main() {
	server := ginboot.New()

	// Create an API group with shared middleware
	apiGroup := server.Group("/api", middleware.Auth())
	{
		v1Group := apiGroup.Group("/v1")
		{
			v1Group.GET("/health", healthCheck)
		}
	}

	log.Fatal(server.Start(8080))
}
```

#### Nested Groups

You can create nested groups using the `Group` method on an existing `ControllerGroup`.

```go
protected := group.Group("", middleware.Auth())
{
    protected.POST("", c.CreateUser)
    protected.PUT("/:id", c.UpdateUser)
    protected.DELETE("/:id", c.DeleteUser)
}
```

### HTTP Methods

`ControllerGroup` provides methods for all standard HTTP verbs:

```go
group.GET("", handler)      // GET request
group.POST("", handler)     // POST request
group.PUT("", handler)      // PUT request
group.DELETE("", handler)   // DELETE request
group.PATCH("", handler)    // PATCH request
group.OPTIONS("", handler)  // OPTIONS request
group.HEAD("", handler)     // HEAD request
```

### Path Parameters

Ginboot supports Gin's path parameter syntax, allowing you to capture values from the URL.

```go
group.GET("/:id", controller.GetUser)           // Matches /users/123, :id captures "123"
group.GET("/:type/*path", controller.GetFile)   // Matches /files/image/avatar.png, :type captures "image", *path captures "avatar.png"
```

## Handler Function Signatures

Ginboot offers flexibility in defining your handler functions. The framework's internal `wrapHandler` mechanism automatically adapts your handler's signature to Gin's requirements, handling request parsing, context injection, and error management. All handlers must return two values: a response value (can be any type) and an error value.

### 1. Context Only Handler

Use this pattern when your handler needs direct access to Ginboot's custom `Context` utilities (e.g., `GetAuthContext`, `GetPageRequest`, `Param`).

```go
func (c *Controller) ListApiKeys(ctx *ginboot.Context) (*ApiKeyList, error) {
    authContext, err := ctx.GetAuthContext()
    if err != nil {
        return nil, err
    }
    pageRequest := ctx.GetPageRequest()
    // ... use authContext and pageRequest ...
    return &ApiKeyList{}, nil
}
```

### 2. Request Model Handler

This pattern is ideal when your handler primarily processes a request body. Ginboot will automatically parse and validate the request body into the provided struct.

```go
type CreateApiKeyRequest struct {
    Name string `json:"name" binding:"required"`
}

func (c *Controller) CreateApiKey(request CreateApiKeyRequest) (*ApiKey, error) {
    // Request is automatically parsed and validated
    // Auth context can be accessed through middleware if needed
    return &ApiKey{}, nil
}
```

### 3. No Input Handler

For simple endpoints that don't require any input parameters or custom context, you can use this concise signature.

```go
func (c *Controller) GetApiKeyStats() (*ApiKeyStats, error) {
    // Simple handlers with no input parameters
    return &ApiKeyStats{}, nil
}
```

### 4. Context and Request Model Handler

This pattern combines the benefits of both context and request model handlers, allowing access to `ginboot.Context` utilities and automatic request body parsing.

```go
type UpdateUserRequest struct {
    Name string `json:"name" binding:"required"`
}

func (c *Controller) UpdateUser(ctx *ginboot.Context, request UpdateUserRequest) (*User, error) {
    userID := ctx.Param("id")
    // ... use userID from context and data from request ...
    return &User{}, nil
}
```

## Middleware

Middleware can be applied at different levels to intercept requests and perform actions like authentication, logging, or data transformation.

### Group Middleware

Apply middleware to an entire `ControllerGroup` when creating it. This is useful for protecting a set of routes with common logic, such as authentication.

```go
import (
    "github.com/klass-lk/ginboot"
    "your-project/internal/middleware"
)

// Assuming middleware.Auth() and middleware.AdminOnly() are gin.HandlerFunc
adminGroup := server.Group("/admin", middleware.Auth(), middleware.AdminOnly())
{
    // All routes within this group will use Auth() and AdminOnly() middleware
    adminGroup.GET("/stats", adminController.GetStats)
    adminGroup.POST("/settings", adminController.UpdateSettings)
}
```

### Route-Specific Middleware

You can also apply middleware to individual routes by passing them as additional arguments to the HTTP method functions.

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/klass-lk/ginboot"
    "your-project/internal/middleware"
)

// Assuming middleware.Cache() is a gin.HandlerFunc
group.GET("/users", middleware.Cache(), controller.ListUsers)
```

For server-wide middleware, refer to the [Server Configuration Documentation](./server.md).