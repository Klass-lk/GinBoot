# Authentication

Ginboot provides robust tools for handling authentication, including JWT management, password encoding, and a custom `AuthContext` for easy access to authenticated user information.

## API Request Context and Authentication

The `ginboot.Context` extends Gin's context with utilities to simplify authentication-related tasks. The `GetAuthContext()` method allows you to retrieve details about the authenticated user.

### `AuthContext` Structure

```go
type AuthContext struct {
	UserID    string
	UserEmail string
	Roles     []string
	Claims    map[string]interface{}
}
```

### Retrieving `AuthContext`

To use `GetAuthContext()`, an authentication middleware must first populate the underlying `gin.Context` with `user_id` and `role` values. If these are not found, `GetAuthContext()` will return an error and the request will be aborted with a `401 Unauthorized` status.

```go
func (c *Controller) GetProtectedData(ctx *ginboot.Context) (interface{}, error) {
    authContext, err := ctx.GetAuthContext()
    if err != nil {
        // Error already handled by SendError in wrapHandler
        return nil, err 
    }

    fmt.Printf("Authenticated User ID: %s, Role: %v\n", authContext.UserID, authContext.Roles)
    // ... use authContext.UserID or authContext.Roles ...
    return gin.H{"message": "Protected data for " + authContext.UserID}, nil
}
```

## JWT (JSON Web Token) Management

Ginboot includes utilities in the `jwt.go` package for generating, parsing, and validating JWTs. These functions rely on environment variables for secret keys.

### Environment Variables

*   `JWT_SECRET`: Secret key for signing and verifying access tokens.
*   `JWT_REFRESH_SECRET`: Secret key for signing and verifying refresh tokens.

### Generating Tokens

Use `GenerateTokens` to create a pair of access and refresh tokens for a given user ID and role.

```go
import (
	"fmt"
	"github.com/klass-lk/ginboot"
	os
)

func init() {
	// Set environment variables for demonstration
	os.Setenv("JWT_SECRET", "supersecretaccesskey")
	os.Setenv("JWT_REFRESH_SECRET", "supersecretrefreshkey")
}

func main() {
	accessToken, refreshToken, err := ginboot.GenerateTokens("user123", "admin")
	if err != nil {
		fmt.Println("Error generating tokens:", err)
		return
	}
	fmt.Println("Access Token:", accessToken)
	fmt.Println("Refresh Token:", refreshToken)
}
```

### Parsing and Extracting Claims

You can parse tokens and extract their claims to retrieve user information.

```go
import (
	"fmt"
	"github.com/klass-lk/ginboot"
	os
)

func init() {
	// Set environment variables for demonstration
	os.Setenv("JWT_SECRET", "supersecretaccesskey")
	os.Setenv("JWT_REFRESH_SECRET", "supersecretrefreshkey")
}

func main() {
	accessToken, _, _ := ginboot.GenerateTokens("user123", "admin")

	parsedToken, err := ginboot.ParseAccessToken(accessToken)
	if err != nil {
		fmt.Println("Error parsing token:", err)
		return
	}

	claims, err := ginboot.ExtractClaims(parsedToken)
	if err != nil {
		fmt.Println("Error extracting claims:", err)
		return
	}

	userID := ginboot.ExtractUserId(claims)
	role := ginboot.ExtractRole(claims)
	fmt.Printf("Extracted User ID: %s, Role: %s\n", userID, role)

	if ginboot.IsExpired(claims) {
		fmt.Println("Token is expired")
	} else {
		fmt.Println("Token is valid")
	}
}
```

## Password Encoding

Ginboot provides a `PasswordEncoder` interface and a `PBKDF2Encoder` implementation for secure password hashing and verification.

### `PasswordEncoder` Interface

```go
type PasswordEncoder interface {
    GetPasswordHash(password string) (string, error)
    IsMatching(hash, password string) bool
}
```

### `PBKDF2Encoder`

This implementation uses PBKDF2 with SHA512 for strong password hashing. It requires specific environment variables for configuration.

### Environment Variables

*   `PBKDF2_ENCODER_SECRET`: A secret string used as a salt for hashing.
*   `PBKDF2_ENCODER_ITERATION`: The number of iterations for the PBKDF2 algorithm (e.g., `10000`).
*   `PBKDF2_ENCODER_KEY_LENGTH`: The desired length of the derived key (e.g., `32`).

### Usage Example

```go
import (
	"fmt"
	"github.com/klass-lk/ginboot"
	os
)

func init() {
	// Set environment variables for demonstration
	os.Setenv("PBKDF2_ENCODER_SECRET", "randomsaltstring")
	os.Setenv("PBKDF2_ENCODER_ITERATION", "10000")
	os.Setenv("PBKDF2_ENCODER_KEY_LENGTH", "32")
}

func main() {
	encoder := ginboot.NewPBKDF2Encoder()

	password := "mySecurePassword123"
	hashedPassword, err := encoder.GetPasswordHash(password)
	if err != nil {
		fmt.Println("Error hashing password:", err)
		return
	}
	fmt.Println("Hashed Password:", hashedPassword)

	// Verify a matching password
	if encoder.IsMatching(hashedPassword, password) {
		fmt.Println("Password matches!")
	} else {
		fmt.Println("Password does NOT match.")
	}

	// Verify a non-matching password
	if encoder.IsMatching(hashedPassword, "wrongpassword") {
		fmt.Println("Wrong password matches (ERROR)!")
	} else {
		fmt.Println("Wrong password does not match (CORRECT).")
	}
}
```

## Integrating Custom Authentication Middleware

To integrate authentication into your Ginboot application, you typically create a Gin middleware that processes authentication credentials (e.g., JWTs from headers) and populates the `gin.Context` with user information. This information can then be accessed via `ginboot.Context.GetAuthContext()`.

Here's an example of a simple JWT authentication middleware:

```go
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
)

func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Bearer token not found"})
			return
		}

		token, err := ginboot.ParseAccessToken(tokenString)
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		claims, err := ginboot.ExtractClaims(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			return
		}

		// Set user information in Gin context for ginboot.Context.GetAuthContext()
		c.Set("user_id", ginboot.ExtractUserId(claims))
		c.Set("role", ginboot.ExtractRole(claims))
		// Optionally set other claims or user details
		// c.Set("user_email", claims["email"])
		// c.Set("claims", claims)

		c.Next()
	}
}
```

This middleware can then be applied globally, to a group, or to specific routes as described in the [Routing Documentation](./routing.md).