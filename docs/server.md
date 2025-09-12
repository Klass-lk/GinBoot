# Server Configuration

This document provides detailed information on configuring and managing the Ginboot server, including how to start it, configure CORS, and apply middleware.

## Initializing the Server

The `ginboot.New()` function creates a new `Server` instance. It automatically detects if it's running in an AWS Lambda environment by checking the `LAMBDA_TASK_ROOT` environment variable. If detected, the runtime is set to `RuntimeLambda`; otherwise, it defaults to `RuntimeHTTP`.

```go
import "github.com/klass-lk/ginboot"

// Create a new server instance
server := ginboot.New()
```

## Starting the Server

The `Start` method initiates the server. The behavior depends on the detected or explicitly set runtime.

### Basic HTTP Server

To start the server as a standard HTTP application, simply call `Start` with the desired port.

```go
import (
    "log"
    "github.com/klass-lk/ginboot"
)

func main() {
    server := ginboot.New()
    // ... register controllers and middleware ...

    // Start the server on port 8080
    err := server.Start(8080)
    if err != nil {
        log.Fatal(err)
    }
}
```

### AWS Lambda Support

Ginboot seamlessly integrates with AWS Lambda. When running in a Lambda environment (detected by `LAMBDA_TASK_ROOT`), the `Start` method will automatically configure the server to handle API Gateway proxy requests. The `port` argument is ignored in Lambda mode.

```go
import (
    "github.com/klass-lk/ginboot"
    // Ensure LAMBDA_RUNTIME=true environment variable is set for Lambda mode
)

func main() {
    server := ginboot.New()
    // ... register controllers and middleware ...

    // Start the server (port is ignored in Lambda mode)
    server.Start(0)
}
```

You can also explicitly set the runtime using `SetRuntime`:

```go
server := ginboot.New()
server.SetRuntime(ginboot.RuntimeLambda)
server.Start(0)
```

## Base Path Configuration

You can set a base path for all routes in your application using the `SetBasePath` method. All registered routes will be prefixed with this path.

```go
server := ginboot.New()
server.SetBasePath("/api/v1") // All routes will be prefixed with /api/v1
```

## CORS Configuration

Ginboot provides flexible CORS (Cross-Origin Resource Sharing) configuration options through the `Server` struct, leveraging `github.com/gin-contrib/cors`.

### Default CORS Configuration

For quick setup with sensible defaults, use `DefaultCORS()`. This configuration allows all origins, common HTTP methods, and common headers, with a preflight cache of 12 hours.

```go
server := ginboot.New()
server.DefaultCORS() // Allows all origins with common methods and headers
```

The default configuration includes:
-   **Allowed Origins**: `*` (all origins)
-   **Allowed Methods**: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`
-   **Allowed Headers**: `Origin`, `Content-Length`, `Content-Type`, `Authorization`
-   **Max Age**: `12 * time.Hour` (preflight cache duration)

### Custom CORS Configuration

For more control, use `CustomCORS` to specify allowed origins, methods, headers, and max age.

```go
import (
    "time"
    "github.com/klass-lk/ginboot"
)

server := ginboot.New()
server.CustomCORS(
    []string{"http://localhost:3000", "https://yourdomain.com"},  // Allowed origins
    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},          // Allowed methods
    []string{"Origin", "Content-Type", "Authorization", "Accept"}, // Allowed headers
    24*time.Hour,                                                 // Preflight cache duration
)
```

### Advanced CORS Configuration

For complete control over all CORS settings provided by `gin-contrib/cors`, use the `WithCORS` method and pass a `cors.Config` struct.

```go
import (
    "time"
    "github.com/gin-contrib/cors"
    "github.com/klass-lk/ginboot"
)

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

## Middleware

Ginboot allows you to apply middleware at different levels: globally (server-wide), to route groups, or to individual routes.

### Server-Wide Middleware

To apply middleware globally to all routes handled by the server, use the `Use` method on the `Server` instance.

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/klass-lk/ginboot"
    // Assuming you have a middleware package
    "your-project/internal/middleware" 
)

func main() {
    server := ginboot.New()
    
    // Apply a logger middleware globally
    server.Use(gin.Logger())
    server.Use(middleware.SomeCustomGlobalMiddleware())

    // ... register controllers and start server ...
}
```

For group-specific or route-specific middleware, refer to the [Routing Documentation](./routing.md).