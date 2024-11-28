# GinBoot Library for Gin Framework

GinBoot is a utility library for the [Gin Web Framework](https://github.com/gin-gonic/gin) that simplifies common tasks in web application development, including database operations, API request handling, and error management. The library is designed to enhance productivity by providing reusable, customizable components.

## Features

- **Database Operations**: Built-in support for MongoDB through a generic repository interface, enabling common CRUD operations with minimal code.
- **API Request Handling**: Simplified API request and authentication context extraction.
- **Error Handling**: Easily define and manage business errors.
- **Password Encoding**: Inbuilt password hashing and matching utility for secure authentication.

## Installation

To install GinBoot, add it to your project:

```bash
go get github.com/klass-lk/ginboot
```

## Usage

### Database Operations

GinBoot provides a `GenericRepository` interface for MongoDB, which offers a variety of methods to simplify data access operations.

```go
type GenericRepository[T Document] interface {
    Query() *mongo.Collection
    FindById(id interface{}) (T, error)
    FindAllById(idList []string) ([]T, error)
    Save(doc T) error
    SaveOrUpdate(doc T) error
    SaveAll(sms []T) error
    Update(doc T) error
    Delete(id string) error
    FindOneBy(field string, value interface{}) (T, error)
    FindOneByFilters(filters map[string]interface{}) (T, error)
    FindBy(field string, value interface{}) ([]T, error)
    FindByFilters(filters map[string]interface{}) ([]T, error)
    FindAll(opts ...*options.FindOptions) ([]T, error)
    FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error)
    FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error)
    CountBy(field string, value interface{}) (int64, error)
    CountByFilters(filters map[string]interface{}) (int64, error)
    ExistsBy(field string, value interface{}) (bool, error)
    ExistsByFilters(filters map[string]interface{}) (bool, error)
}
```

### Example

```go
// Define a repository for your data type
type MyRepository struct {
    repository.GenericRepository[MyDocument]
}
```

### MongoDB Configuration

GinBoot provides a flexible MongoDB configuration system through the `MongoConfig` struct. Here's how to use it:

```go
// Create a new MongoDB configuration
config := ginboot.NewMongoConfig().
    WithHost("localhost", 27017).
    WithCredentials("username", "password").
    WithDatabase("mydb").
    WithOption("authSource", "admin")

// Connect to MongoDB
db, err := config.Connect()
if err != nil {
    log.Fatal(err)
}

// Create a repository for your model
type User struct {
    ID   string `bson:"_id"`
    Name string `bson:"name"`
}

func (u User) GetID() interface{}     { return u.ID }
func (u User) SetID(id interface{})   { u.ID = id.(string) }
func (u *User) GetCollectionName() string { return "users" }

// Initialize the repository
userRepo := ginboot.NewMongoRepository[User](db)

// Use the repository
user := User{Name: "John Doe"}
err = userRepo.Save(user)
```

For more secure handling of credentials, you can use environment variables:

```go
config := ginboot.NewMongoConfig().
    WithHost(os.Getenv("MONGO_HOST"), 27017).
    WithCredentials(
        os.Getenv("MONGO_USERNAME"),
        os.Getenv("MONGO_PASSWORD"),
    ).
    WithDatabase(os.Getenv("MONGO_DATABASE"))
```

## API Request Context

GinBoot simplifies the extraction of request and authentication context from the Gin context, making it easier to handle requests in controllers.

```go
func BuildAuthRequestContext[T interface{}](c *gin.Context) (T, AuthContext, error) {}
```

### Example Usage

```go
func (controller *ApiKeyController) Create(c *gin.Context) {
    request, authContext, err := ginboot.BuildAuthRequestContext[domain.CreateApiKeyRequest](c)
    if err != nil {
        ginboot.SendError(c, err)
        return
    }
    apiKey, err := controller.service.Create(authContext, request)
    if err != nil {
        ginboot.SendError(c, err)
        return
    }
    c.JSON(http.StatusOK, apiKey)
}

```
For retrieving only the authentication context:

```go
func GetAuthContext(c *gin.Context) (AuthContext, error) {}
```

### Example

```go
func (controller *ApiKeyController) GetApiKeys(c *gin.Context) {
    authContext, err := ginboot.GetAuthContext(c)
    if err != nil {
        ginboot.SendError(c, err)
        return
    }
    apiKeys, err := controller.service.GetApiKeys(authContext)
    if err != nil {
        ginboot.SendError(c, err)
        return
    }
    c.JSON(http.StatusOK, apiKeys)
}
```

## Business Error Handling

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
