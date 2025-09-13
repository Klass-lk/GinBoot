# Database Support

Ginboot provides a powerful and flexible multi-database support system through a generic repository interface. This allows you to interact with different database systems (MongoDB, SQL, DynamoDB) using a consistent API, making your application more modular, testable, and adaptable to various data storage needs.

## Generic Repository Interface

The core of Ginboot's database abstraction is the `GenericRepository[T any]` interface. This interface defines a comprehensive set of common data access operations, ensuring a uniform way to interact with different database types.

```go
type GenericRepository[T any] interface {
	FindById(id string) (T, error)
	FindAllById(ids []string) ([]T, error)
	Save(doc T) error
	SaveOrUpdate(doc T) error
	SaveAll(docs []T) error
	Update(doc T) error
	Delete(id string) error
	FindOneBy(field string, value interface{}) (T, error)
	FindOneByFilters(filters map[string]interface{}) (T, error)
	FindBy(field string, value interface{}) ([]T, error)
	FindByFilters(filters map[string]interface{}) ([]T, error)
	FindAll(options ...interface{}) ([]T, error)
	FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error)
	FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error)
	CountBy(field string, value interface{}) (int64, error)
	CountByFilters(filters map[string]interface{}) (int64, error)
	ExistsBy(field string, value interface{}) (bool, error)
	ExistsByFilters(filters map[string]interface{}) (bool, error)
}
```

### `Document` Interface

For SQL and DynamoDB repositories, your data models must implement the `Document` interface, which provides the table/collection name.

```go
type Document interface {
	GetTableName() string
}
```

### Pagination Structures

Ginboot provides standardized structures for handling pagination requests and responses.

```go
type SortField struct {
	Field     string `json:"field"`
	Direction int    `json:"direction"` // 1 for ascending, -1 for descending
}

type PageRequest struct {
	Page int       `json:"page"`
	Size int       `json:"size"`
	Sort SortField `json:"sort"`
}

type PageResponse[T interface{}] struct {
	Contents         []T         `json:"content"`
	NumberOfElements int         `json:"numberOfElements"`
	Pageable         PageRequest `json:"pageable"`
	TotalPages       int         `json:"totalPages"`
	TotalElements    int         `json:"totalElements"`
}
```

## MongoDB Database Support

Ginboot offers robust support for MongoDB through `MongoConfig` for connection management and `MongoRepository` for data operations.

### MongoDB Configuration

Use `ginboot.NewMongoConfig()` to build your MongoDB connection string. You can specify host, port, credentials, database name, and additional options.

```go
import (
	"log"
	"github.com/klass-lk/ginboot"
)

func connectMongo() *mongo.Database {
	config := ginboot.NewMongoConfig().
		WithHost("localhost", 27017).
		WithDatabase("mydatabase").
		WithCredentials("myuser", "mypassword").
		WithOption("authSource", "admin")

	db, err := config.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	fmt.Println("Connected to MongoDB!")
	return db
}
```

### MongoDB Repository Example

Define your document struct with `bson` tags for MongoDB field mapping and a `ginboot:"_id"` tag for the primary key if it's not named `ID`.

```go
import (
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
	"github.com/klass-lk/ginboot"
)

type User struct {
    ID   string `bson:"_id" ginboot:"_id"` // ginboot:_id helps the repository identify the ID field
    Name string `bson:"name"`
    Age  int    `bson:"age"`
}

// NewMongoRepository creates a new MongoDB repository instance.
// The collection name is typically the plural of your entity name.
func NewUserRepository(db *mongo.Database) *UserRepository {
    return &UserRepository{
        MongoRepository: ginboot.NewMongoRepository[User](db, "users"),
    }
}

// Example usage of the MongoDB repository
func main() {
	db := connectMongo() // Assume connectMongo() returns *mongo.Database
	repo := ginboot.NewMongoRepository[User](db, "users")

	// Save a new user
	user := User{ID: "1", Name: "John Doe", Age: 30}
	err := repo.Save(user)
	if err != nil { log.Fatal(err) }
	fmt.Println("User saved:", user.Name)

	// Find user by ID
	foundUser, err := repo.FindById("1")
	if err != nil { log.Fatal(err) }
	fmt.Println("Found user:", foundUser.Name)

	// Update user
	foundUser.Age = 31
	err = repo.Update(foundUser)
	if err != nil { log.Fatal(err) }
	fmt.Println("User updated:", foundUser.Name)

	// Find users by filter
	filters := map[string]interface{}{"age": 31}
	users, err := repo.FindByFilters(filters)
	if err != nil { log.Fatal(err) }
	fmt.Println("Users with age 31:", len(users))

	// Paginated query
	pageRequest := ginboot.PageRequest{Page: 1, Size: 10, Sort: ginboot.SortField{Field: "name", Direction: 1}}
	pageResponse, err := repo.FindAllPaginated(pageRequest)
	if err != nil { log.Fatal(err) }
	fmt.Println("Paginated results:", len(pageResponse.Contents))
}
```

## SQL Database Support

Ginboot provides a generic repository interface for SQL databases, allowing you to interact with relational databases like PostgreSQL or MySQL using a consistent API.

### SQL Configuration

Use `ginboot.NewSQLConfig()` to configure your SQL connection. You need to specify the `Driver` (e.g., "postgres", "mysql"), host, port, credentials, and database name.

```go
import (
	"log"
	"database/sql"
	"github.com/klass-lk/ginboot"
	_ "github.com/lib/pq" // Import the PostgreSQL driver
)

func connectSQL() *sql.DB {
	config := ginboot.NewSQLConfig().
		WithDriver("postgres").
		WithHost("localhost", 5432).
		WithDatabase("testdb").
		WithCredentials("postgres", "password").
		WithOption("sslmode", "disable")

	db, err := config.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	fmt.Println("Connected to PostgreSQL!")
	return db
}
```

### SQL Repository Example

Your SQL document struct must implement the `Document` interface and use `db` tags to map fields to database columns. The `ID` field is assumed to be the primary key.

```go
import (
	"fmt"
	"log"
	"database/sql"
	"time"
	"github.com/klass-lk/ginboot"
)

type Product struct {
    ID        string    `db:"id"`
    Name      string    `db:"name"`
    Price     float64   `db:"price"`
    CreatedAt time.Time `db:"created_at"`
}

func (p Product) GetTableName() string {
	return "products"
}

// Example usage of the SQL repository
func main() {
	db := connectSQL() // Assume connectSQL() returns *sql.DB
	repo := ginboot.NewSQLRepository[Product](db)

	// Ensure the table exists (optional, can be done once at startup)
	err := repo.CreateTable()
	if err != nil { log.Fatal(err) }

	// Save a new product
	product := Product{ID: "p1", Name: "Laptop", Price: 1200.00, CreatedAt: time.Now()}
	err = repo.Save(product)
	if err != nil { log.Fatal(err) }
	fmt.Println("Product saved:", product.Name)

	// Find product by ID
	foundProduct, err := repo.FindById("p1")
	if err != nil { log.Fatal(err) }
	fmt.Println("Found product:", foundProduct.Name)

	// Update product
	foundProduct.Price = 1150.00
	err = repo.Update(foundProduct)
	if err != nil { log.Fatal(err) }
	fmt.Println("Product updated:", foundProduct.Name)

	// Find products by filter
	filters := map[string]interface{}{"name": "Laptop"}
	products, err := repo.FindByFilters(filters)
	if err != nil { log.Fatal(err) }
	fmt.Println("Products named Laptop:", len(products))

	// Delete product
	err = repo.Delete("p1")
	if err != nil { log.Fatal(err) }
	fmt.Println("Product deleted.")
}
```

## DynamoDB Support

Ginboot provides robust support for AWS DynamoDB, offering a similar generic repository interface for interacting with NoSQL tables.

### DynamoDB Configuration

Use `ginboot.NewDynamoConfig()` to configure your DynamoDB client. You can specify the AWS region, credentials (access key and secret key), a custom endpoint (useful for local DynamoDB), or an AWS profile.

```go
import (
	"log"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/klass-lk/ginboot"
)

func connectDynamoDB() *dynamodb.Client {
	config := ginboot.NewDynamoConfig().
		WithRegion("us-east-1").
		WithEndpoint("http://localhost:8000") // For local DynamoDB
		// .WithCredentials("your-access-key", "your-secret-key")
		// .WithProfile("your-aws-profile")

	client, err := config.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to DynamoDB: %v", err)
	}
	fmt.Println("Connected to DynamoDB!")
	return client
}
```

### DynamoDB Repository Example

Your DynamoDB document struct must implement the `Document` interface and use `dynamodbav` tags to map fields to DynamoDB attributes. The `ID` field is assumed to be the primary key.

```go
import (
	"fmt"
	"log"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/klass-lk/ginboot"
)

type Order struct {
    ID         string  `ginboot:"id"`
    CustomerID string  `dynamodbav:"customer_id"`
    Amount     float64 `dynamodbav:"amount"`
    Status     string  `dynamodbav:"status"`
}

func (o Order) GetTableName() string {
	return "orders"
}

// Example usage of the DynamoDB repository
func main() {
	client := connectDynamoDB() // Assume connectDynamoDB() returns *dynamodb.Client
	// The last parameter (skipTableCreation) can be set to true if you manage table creation externally
	repo := ginboot.NewDynamoDBRepository[Order](client, "orders", false)

	// Save a new order
	order := Order{ID: "o1", CustomerID: "cust123", Amount: 99.99, Status: "PENDING"}
	err := repo.Save(order)
	if err != nil { log.Fatal(err) }
	fmt.Println("Order saved:", order.ID)

	// Find order by ID
	foundOrder, err := repo.FindById("o1")
	if err != nil { log.Fatal(err) }
	fmt.Println("Found order:", foundOrder.ID, "Status:", foundOrder.Status)

	// Update order
	foundOrder.Status = "COMPLETED"
	err = repo.Update(foundOrder)
	if err != nil { log.Fatal(err) }
	fmt.Println("Order updated:", foundOrder.ID, "Status:", foundOrder.Status)

	// Find orders by filter (Note: DynamoDB Scan operations can be inefficient for large tables)
	filters := map[string]interface{}{"customer_id": "cust123"}
	orders, err := repo.FindByFilters(filters)
	if err != nil { log.Fatal(err) }
	fmt.Println("Orders for customer cust123:", len(orders))

	// Delete order
	err = repo.Delete("o1")
	if err != nil { log.Fatal(err) }
	fmt.Println("Order deleted.")
}
```

### Considerations for DynamoDB Performance

*   **Scan vs. Query:** Methods like `FindOneBy`, `FindOneByFilters`, `FindBy`, `FindByFilters`, `CountBy`, `CountByFilters`, and pagination methods (`FindAllPaginated`, `FindByPaginated`) internally use DynamoDB's `Scan` operation when filtering on non-primary key attributes. `Scan` operations read every item in the table and can be inefficient and costly for large tables.
*   **Global Secondary Indexes (GSIs):** For better performance on frequently queried non-primary key fields, consider defining Global Secondary Indexes (GSIs) on your DynamoDB tables. Ginboot's generic repository methods do not automatically leverage GSIs; you would typically use the underlying `*dynamodb.Client` directly for GSI-based queries or extend the repository to include GSI-aware methods.
*   **Pagination:** DynamoDB's native pagination uses `ExclusiveStartKey` rather than traditional offset/limit. Ginboot's pagination methods simulate offset/limit by performing multiple `Scan` operations and discarding items, which can be inefficient for deep pagination. For optimal performance with large datasets, consider implementing cursor-based pagination directly using DynamoDB's `ExclusiveStartKey`.

## Customizing Repositories

You can easily extend Ginboot's generic repositories to add database-specific methods or custom business logic. This is done by embedding the generic repository within your own custom repository struct.

```go
import (
	"go.mongodb.org/mongo-driver/mongo"
	"github.com/klass-lk/ginboot"
)

type UserRepository struct {
    *ginboot.MongoRepository[User] // Embed the generic repository
}

func NewUserRepository(db *mongo.Database) *UserRepository {
    return &UserRepository{
        MongoRepository: ginboot.NewMongoRepository[User](db, "users"),
    }
}

// Add a custom method specific to UserRepository
func (r *UserRepository) FindUsersByStatus(status string) ([]User, error) {
    // You can use the embedded generic repository methods
    return r.FindBy("status", status)
}

// Or implement a completely custom query
func (r *UserRepository) GetActiveUsersCount() (int64, error) {
    // Access the underlying collection directly if needed
    // return r.collection.CountDocuments(context.Background(), bson.M{"status": "active"})
    return r.CountBy("status", "active")
}
```