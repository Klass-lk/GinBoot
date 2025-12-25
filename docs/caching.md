# Caching Support

Ginboot provides a unified caching layer that supports **DynamoDB**, **SQL**, and **MongoDB** backends. It integrates seamlessly with Gin middleware, offering automatic response caching and tag-based invalidation.

## Architecture

The caching system consists of the following components:

*   **`CacheEntry`**: The data structure stored in the database.
*   **`CacheService`**: Interface for `Set`, `Get`, and `Invalidate` operations.
*   **`CacheMiddleware`**: Gin middleware that automatically caches GET responses.
*   **Generic Repository Enhancements**: Repositories now support bulk operations like `DeleteBy` to facilitate efficient invalidation.

## supported Backends

### 1. DynamoDB
Requires a `CacheEntry` table and a `TagEntry` table (using single-table design principles).

```go
client := ginboot.NewDynamoDBClient(cfg)
cacheService := ginboot.NewDynamoDBCacheService(client)
```

**Schema**:
*   `CacheEntry`: PK=`CACHE#<key>`, SK=`DATA`
*   `TagEntry`: PK=`TAG#<tag>`, SK=`CACHE#<key>`

### 2. SQL
Requires `cache_entries` and `cache_tags` tables.

```go
cacheRepo := ginboot.NewSQLRepository[ginboot.CacheEntry](db)
tagRepo := ginboot.NewSQLRepository[ginboot.TagEntry](db)
cacheService := ginboot.NewSQLCacheService(cacheRepo, tagRepo)
```

### 3. MongoDB
Requires a single collection (default: `cache_entries`) with a `tags` array field.

```go
cacheRepo := ginboot.NewMongoRepository[ginboot.CacheEntry](db, "cache_entries")
cacheService := ginboot.NewMongoCacheService(cacheRepo)
```

## Usage

### Middleware Setup

Use `CacheMiddleware` to cache responses for GET requests. You can define a custom `TagGenerator` to tag cache entries for later invalidation.

```go
// Define a Tag Generator
tagGen := func(c *gin.Context) []string {
    // Tag by resource type or ID
    return []string{"posts"}
}

// Initialize Middleware
cacheMiddleware := ginboot.CacheMiddleware(
    cacheService,
    10 * time.Minute, // TTL
    tagGen,           // Tag Generator
    nil,              // Default Key Generator
)

// Apply to routes
router.GET("/posts", cacheMiddleware, postController.GetPosts)
```

### Automatic Invalidation

Invalidate cache entries when data changes (e.g., in `Create`, `Update`, `Delete` handlers/services).

```go
func (c *PostController) UpdatePost(ctx *ginboot.Context, post model.Post) {
    // ... update logic
    
    // Invalidate all entries tagged with "posts"
    c.cacheService.Invalidate(ctx, "posts")
}
```

### Manual Invalidation Controller

You can expose an endpoint to manually invalidate tags.

```go
func (c *CacheController) Invalidate(ctx *ginboot.Context) (ginboot.EmptyResponse, error) {
    tag := ctx.Query("tag")
    if tag == "" {
        return ginboot.EmptyResponse{}, ginboot.ApiError{ErrorCode: "BAD_REQUEST", Message: "Tag is required"}
    }
    
    // Invalidate
    err := c.cacheService.Invalidate(context.Background(), tag)
    return ginboot.EmptyResponse{}, err
}
```
