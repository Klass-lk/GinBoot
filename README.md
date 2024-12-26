# Ginboot Framework

A lightweight and powerful Go web framework built on top of Gin, designed for building scalable web applications with MongoDB integration and AWS Lambda support.

## Setup

### Prerequisites
- Go 1.21 or later
- MongoDB (for local development)
- AWS SAM CLI (for deployment)
- AWS credentials configured

### Installation

1. Install the Ginboot CLI tool:
```bash
go install github.com/klass-lk/ginboot-cli@latest
```

2. Create a new project:
```bash
# Create a new project
ginboot new myproject

# Navigate to project directory
cd myproject

# Initialize dependencies
go mod tidy
```

3. Run locally:
```bash
go run main.go
```
Your API will be available at `http://localhost:8080/api/v1`

### Build and Deploy

To deploy your application to AWS Lambda:

```bash
# Build the project for AWS Lambda
ginboot build

# Deploy to AWS
ginboot deploy
```

On first deployment, you'll be prompted for:
- Stack name (defaults to project name)
- AWS Region
- S3 bucket configuration

These settings will be saved in `ginboot-app.yml` for future deployments.

## Features

- **Database Operations**: Built-in support for MongoDB through a generic repository interface, enabling common CRUD operations with minimal code.
- **API Request Handling**: Simplified API request and authentication context extraction.
- **Error Handling**: Easily define and manage business errors.
- **Password Encoding**: Inbuilt password hashing and matching utility for secure authentication.
- **CORS Configuration**: Flexible CORS setup with both default and custom configurations.

## Installation

To install GinBoot, add it to your project:

```bash
go get github.com/klass-lk/ginboot
```

## Usage

### MongoDB Repository

GinBoot provides a generic MongoDB repository that simplifies database operations. Here's how to use it:

1. Define your document struct:
```go
type User struct {
    ID   string `bson:"_id" ginboot:"_id"`
    Name string `bson:"name"`
}
```

2. Create a repository:
```go
// Create a repository instance
repo := ginboot.NewMongoRepository[User](db, "users")

// Or wrap it in your own repository struct for additional methods
type UserRepository struct {
    *ginboot.MongoRepository[User]
}

func NewUserRepository(db *mongo.Database) *UserRepository {
    return &UserRepository{
        MongoRepository: ginboot.NewMongoRepository[User](db, "users"),
    }
}
```

3. Use the repository:
```go
// Create
user := User{ID: "1", Name: "John"}
err := repo.SaveOrUpdate(user)

// Read
user, err := repo.FindById("1")

// Update
user.Name = "John Doe"
err = repo.Update(user)

// Delete
err = repo.Delete("1")

// Find with filters
users, err := repo.FindByFilters(map[string]interface{}{
    "name": "John",
})

// Paginated query
response, err := repo.FindAllPaginated(PageRequest{
    Page: 1,
    Size: 10,
})
```

The repository provides a comprehensive set of methods for database operations:
- Basic CRUD operations
- Batch operations (SaveAll, FindAllById)
- Filtering and querying
- Pagination support
- Count and existence checks

### API Request Context

GinBoot provides a custom Context wrapper around Gin's context that simplifies request handling and authentication. The context provides these key utilities:

```go
// Get authentication context
authContext, err := ctx.GetAuthContext()

// Parse and validate request body
var request YourRequestType
err := ctx.GetRequest(&request)

// Get paginated request parameters
pageRequest := ctx.GetPageRequest()
```

### Example Usage

Here are examples of different handler patterns supported by GinBoot:

```go
// Pattern 1: Context Handler
// Use when you need direct access to context utilities
func (c *Controller) ListApiKeys(ctx *ginboot.Context) (*ApiKeyList, error) {
    // Get auth context
    authContext, err := ctx.GetAuthContext()
    if err != nil {
        return nil, err
    }

    // Get pagination parameters
    pageRequest := ctx.GetPageRequest()
    
    return c.service.ListApiKeys(authContext, pageRequest)
}

// Pattern 2: Request Model Handler
// Use when you only need the request body
func (c *Controller) CreateApiKey(request models.CreateApiKeyRequest) (*ApiKey, error) {
    // Request is automatically parsed and validated
    return c.service.CreateApiKey(request)
}

// Pattern 3: No Input Handler
// Use for simple endpoints that don't need request data
func (c *Controller) GetApiKeyStats() (*ApiKeyStats, error) {
    return c.service.GetApiKeyStats()
}

// Register routes
func (c *Controller) Register(group *ginboot.ControllerGroup) {
    group.GET("/api-keys", c.ListApiKeys)
    group.POST("/api-keys", c.CreateApiKey)
    group.GET("/api-keys/stats", c.GetApiKeyStats)
}
```

The framework will automatically:
- Handle request parsing and validation
- Manage authentication context
- Process pagination parameters
- Convert responses to JSON
- Handle errors appropriately

### Business Error Handling

Define and manage business errors with GinBoot's ApiError type, which allows custom error codes and messages.

```go
var (
    TokenExpired        = ginboot.ApiError{"TOKEN_EXPIRED", "Token expired"}
    SomethingWentWrong  = ginboot.ApiError{"SOMETHING_WENT_WRONG", "Something went wrong"}
    UserNotFound        = ginboot.ApiError{"USER_NOT_FOUND", "User %s not found"}
    ConfigAlreadyExists = ginboot.ApiError{"CONFIG_ALREADY_EXISTS", "Config %s already exists"}
    ConfigNotFound      = ginboot.ApiError{"CONFIG_NOT_FOUND", "Config %s not found"}
    TeamNotFound        = ginboot.ApiError{"TEAM_NOT_FOUND", "Team %s not found"}
)
```

### Password Encoding

```go
type PasswordEncoder interface {
    GetPasswordHash(password string) (string, error)
    IsMatching(hash, password string) bool
}

```

## CORS Configuration

GinBoot provides flexible CORS configuration options through the Server struct. You can use either default settings or customize them according to your needs.

#### Default CORS Configuration

For quick setup with sensible defaults:

```go
server := ginboot.New()
server.DefaultCORS() // Allows all origins with common methods and headers
```

The default configuration:
- Allows all origins
- Allows common methods: GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS
- Allows common headers: Origin, Content-Length, Content-Type, Authorization
- Sets preflight cache to 12 hours

#### Custom CORS Configuration

For more control over CORS settings:

```go
server := ginboot.New()
server.CustomCORS(
    []string{"http://localhost:3000", "https://yourdomain.com"},  // Allowed origins
    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},          // Allowed methods
    []string{"Origin", "Content-Type", "Authorization", "Accept"}, // Allowed headers
    24*time.Hour,                                                 // Preflight cache duration
)
```

You can customize:
- **Origins**: Specify which domains can access your API
- **Methods**: Control which HTTP methods are allowed
- **Headers**: Define which headers can be included in requests
- **Max Age**: Set how long browsers should cache preflight results

#### Advanced CORS Configuration

For complete control over CORS settings:

```go
import "github.com/gin-contrib/cors"

server := ginboot.New()
config := cors.Config{
    AllowOrigins:     []string{"http://localhost:3000"},
    AllowMethods:     []string{"GET", "POST"},
    AllowHeaders:     []string{"Origin"},
    ExposeHeaders:    []string{"Content-Length"},
    AllowCredentials: true,
    MaxAge:           12 * time.Hour,
}
server.WithCORS(&config)
```

This gives you access to all CORS configuration options provided by gin-contrib/cors.

## Routing

GinBoot provides a flexible and intuitive routing system that follows Gin's style while adding powerful controller-based routing capabilities.

### Base Path Configuration

You can set a base path for all routes in your application:

```go
server := ginboot.New()
server.SetBasePath("/api/v1") // All routes will be prefixed with /api/v1
```

### Controller Registration

Controllers implement the `Controller` interface which requires a `Register` method:

```go
type Controller interface {
    Register(group *ControllerGroup)
}
```

Example controller implementation:

```go
type UserController struct {
    userService *service.UserService
}

func (c *UserController) Register(group *ginboot.ControllerGroup) {
    // Public routes
    group.GET("", c.ListUsers)
    group.GET("/:id", c.GetUser)
    
    // Protected routes
    protected := group.Group("", middleware.Auth())
    {
        protected.POST("", c.CreateUser)
        protected.PUT("/:id", c.UpdateUser)
        protected.DELETE("/:id", c.DeleteUser)
    }
}
```

### Registering Controllers

Register controllers with their base paths:

```go
// Initialize controllers
userController := NewUserController(userService)
postController := NewPostController(postService)

// Register with paths
server.RegisterController("/users", userController) // -> /api/v1/users
server.RegisterController("/posts", postController) // -> /api/v1/posts
```

### Route Groups

Create route groups with shared middleware:

```go
// Create a group with middleware
adminGroup := server.Group("/admin", middleware.Auth(), middleware.AdminOnly())
{
    adminGroup.GET("/stats", adminController.GetStats)
    adminGroup.POST("/settings", adminController.UpdateSettings)
}

// Nested groups
apiGroup := server.Group("/api")
v1Group := apiGroup.Group("/v1")
{
    v1Group.GET("/health", healthCheck)
}
```

### HTTP Methods

GinBoot supports all standard HTTP methods:

```go
group.GET("", handler)      // GET request
group.POST("", handler)     // POST request
group.PUT("", handler)      // PUT request
group.DELETE("", handler)   // DELETE request
group.PATCH("", handler)    // PATCH request
group.OPTIONS("", handler)  // OPTIONS request
group.HEAD("", handler)     // HEAD request
```

### Middleware

Add middleware at different levels:

```go
// Server-wide middleware
server.Use(middleware.Logger())

// Group middleware
group := server.Group("/admin", middleware.Auth())

// Route-specific middleware
group.GET("/users", middleware.Cache(), controller.ListUsers)
```

### Path Parameters

Use Gin's path parameter syntax:

```go
group.GET("/:id", controller.GetUser)           // /users/123
group.GET("/:type/*path", controller.GetFile)   // /files/image/avatar.png
```

### Full Example

```go
func main() {
    server := ginboot.New()
    
    // Set base path for all routes
    server.SetBasePath("/api/v1")
    
    // Global middleware
    server.Use(middleware.Logger())
    
    // Initialize controllers
    userController := NewUserController(userService)
    postController := NewPostController(postService)
    
    // Register controllers
    server.RegisterController("/users", userController)
    server.RegisterController("/posts", postController)
    
    // Create admin group
    adminGroup := server.Group("/admin", middleware.Auth(), middleware.AdminOnly())
    adminController := NewAdminController(adminService)
    adminController.Register(adminGroup)
    
    // Start server
    server.Start(8080)
}
```

This setup creates a clean, maintainable API structure with routes like:
- GET /api/v1/users
- POST /api/v1/posts
- GET /api/v1/admin/stats

The routing system combines the simplicity of Gin's routing with the power of controller-based organization, making it easy to structure and maintain your API endpoints.

## Handler Function Signatures

GinBoot supports multiple handler function signatures for flexibility. You can write your controller methods in any of the following formats:

### 1. Context Only Handler
```go
func (c *Controller) HandleRequest(ctx *ginboot.Context) (Response, error) {
    // Access auth context, request body, and other utilities through ctx
    authContext, err := ctx.GetAuthContext()
    if err != nil {
        return nil, err
    }
    return response, nil
}
```

### 2. Request Model Handler
```go
func (c *Controller) CreateItem(request models.CreateItemRequest) (Response, error) {
    // Request is automatically parsed and validated
    // Auth context can be accessed through middleware if needed
    return response, nil
}
```

### 3. No Input Handler
```go
func (c *Controller) GetStatus() (Response, error) {
    // Simple handlers with no input parameters
    return response, nil
}
```

### Example Controller

```go
type UserController struct {
    service *UserService
}

// Context handler example
func (c *UserController) GetUser(ctx *ginboot.Context) (*User, error) {
    authContext, err := ctx.GetAuthContext()
    if err != nil {
        return nil, err
    }
    return c.service.GetUser(authContext.UserID)
}

// Request model handler example
func (c *UserController) CreateUser(request models.CreateUserRequest) (*User, error) {
    return c.service.CreateUser(request)
}

// No input handler example
func (c *UserController) GetStats() (*Stats, error) {
    return c.service.GetStats()
}

func (c *UserController) Register(group *ginboot.ControllerGroup) {
    group.GET("/user", c.GetUser)
    group.POST("/user", c.CreateUser)
    group.GET("/stats", c.GetStats)
}
```

All handlers must return two values:
1. A response value (can be any type)
2. An error value

The framework will automatically:
- Parse and validate request bodies
- Handle errors appropriately
- Convert responses to JSON
- Manage HTTP status codes based on errors

## Server Configuration

GinBoot provides a flexible server configuration that supports both HTTP and AWS Lambda runtimes.

### Basic HTTP Server

```go
// Create a new server
server := ginboot.New()

// Add your routes
userController := &UserController{}
server.RegisterControllers(userController)

// Start the server on port 8080
err := server.Start(8080)
if err != nil {
    log.Fatal(err)
}
```

### AWS Lambda Support

```go
// Set LAMBDA_RUNTIME=true environment variable for Lambda mode
server := ginboot.New()
server.RegisterControllers(userController)
server.Start(0) // port is ignored in Lambda mode
```

## Route Registration

GinBoot provides a clean way to organize your routes using controllers.

### Basic Controller

```go
type UserController struct {
    userService *UserService
}

func (c *UserController) Routes() []ginboot.Route {
    return []ginboot.Route{
        {
            Method:  "GET",
            Path:    "/users",
            Handler: c.ListUsers,
        },
        {
            Method:  "POST",
            Path:    "/users",
            Handler: c.CreateUser,
            Middleware: []gin.HandlerFunc{
                validateUserMiddleware, // Route-specific middleware
            },
        },
    }
}
```

### Route Groups with Middleware

```go
server := ginboot.New()

// API group with authentication
apiGroup := ginboot.RouterGroup{
    Path: "/api/v1",
    Middleware: []gin.HandlerFunc{authMiddleware},
    Controllers: []ginboot.Controller{
        &UserController{},
        &ProductController{},
    },
}

// Admin group with additional middleware
adminGroup := ginboot.RouterGroup{
    Path: "/admin",
    Middleware: []gin.HandlerFunc{authMiddleware, adminMiddleware},
    Controllers: []ginboot.Controller{
        &AdminController{},
    },
}

server.RegisterGroups(apiGroup, adminGroup)
```

## SQL Database Support

GinBoot provides a generic repository interface for SQL databases.

### SQL Configuration

```go
// Create SQL configuration
config := ginboot.NewSQLConfig().
    WithDriver("postgres").
    WithDSN("host=localhost port=5432 user=myuser password=mypass dbname=mydb sslmode=disable")

// Connect to database
db, err := config.Connect()
if err != nil {
    log.Fatal(err)
}
```

### SQL Repository Example

```go
type User struct {
    ID        string    `db:"id"`
    Name      string    `db:"name"`
    Email     string    `db:"email"`
    CreatedAt time.Time `db:"created_at"`
}

type UserRepository struct {
    *ginboot.SQLRepository[User]
}

// Create a new repository
userRepo := ginboot.NewSQLRepository[User](db, "users")

// Use the repository
user := User{
    Name:  "John Doe",
    Email: "john@example.com",
}

// Basic CRUD operations
err = userRepo.Save(&user)
users, err := userRepo.FindAll()
user, err := userRepo.FindById("123")
err = userRepo.Delete("123")

// Custom queries
users, err := userRepo.FindByField("email", "john@example.com")
count, err := userRepo.CountByField("name", "John")
```

## DynamoDB Support

GinBoot provides DynamoDB support with a similar interface to other databases.

### DynamoDB Configuration

```go
// Create DynamoDB configuration
config := ginboot.NewDynamoConfig().
    WithRegion("us-west-2").
    WithCredentials(aws.NewCredentials("access_key", "secret_key"))

// Connect to DynamoDB
db, err := config.Connect()
if err != nil {
    log.Fatal(err)
}
```

### DynamoDB Repository Example

```go
type Product struct {
    ID          string  `dynamodbav:"id"`
    Name        string  `dynamodbav:"name"`
    Price       float64 `dynamodbav:"price"`
    CategoryID  string  `dynamodbav:"category_id"`
}

type ProductRepository struct {
    *ginboot.DynamoRepository[Product]
}

// Create a new repository
productRepo := ginboot.NewDynamoRepository[Product](db, "products")

// Use the repository
product := Product{
    Name:       "Awesome Product",
    Price:      99.99,
    CategoryID: "electronics",
}

// Basic CRUD operations
err = productRepo.Save(&product)
products, err := productRepo.FindAll()
product, err := productRepo.FindById("123")
err = productRepo.Delete("123")

// Query operations
products, err := productRepo.Query("category_id", "electronics")
count, err := productRepo.CountByField("category_id", "electronics")

// Batch operations
err = productRepo.BatchSave([]Product{product1, product2})
products, err := productRepo.BatchGet([]string{"id1", "id2"})
```

## Contributing
Contributions are welcome! Please read our contributing guidelines for more details.

## License
This project is licensed under the MIT License. See the LICENSE file for details.
