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

Ginboot provides a `PasswordEncoder` interface and a `PBKDF2Encoder` implementation for password hashing and verification.

### `PasswordEncoder` Interface

```go
type PasswordEncoder interface {
    GetPasswordHash(password string) (string, error)
    IsMatching(hash, password string) bool
}
```

### `PBKDF2Encoder`

This implementation uses PBKDF2-HMAC-SHA512 with a random 16-byte salt for every password. Each hash records its own parameters:

```
pbkdf2-sha512$<iterations>$<salt>$<key>
```

Two users with the same password get different hashes, and changing the iteration count or key length never invalidates a stored hash.

### Environment Variables

*   `PBKDF2_ENCODER_ITERATION`: iterations for new hashes. Defaults to `210000`, OWASP's recommendation for PBKDF2-SHA512.
*   `PBKDF2_ENCODER_KEY_LENGTH`: derived key length in bytes. Defaults to `32`.
*   `PBKDF2_ENCODER_SECRET`: only needed to verify hashes written by Ginboot versions before per-password salts, which used this value as one salt shared by every password. New hashes never use it.

A variable that is set but is not a positive integer panics at startup rather than falling back to a default.

### Usage Example

```go
encoder := ginboot.NewPBKDF2Encoder()

hash, err := encoder.GetPasswordHash("mySecurePassword123")
if err != nil {
	return err
}

encoder.IsMatching(hash, "mySecurePassword123") // true
encoder.IsMatching(hash, "wrongpassword")       // false
```

### Upgrading stored hashes

`NeedsRehash` reports whether a stored hash uses the old shared salt or weaker parameters than the encoder is configured with. Check it after a successful login, while you have the plain password, and store a fresh hash:

```go
if encoder.IsMatching(user.PasswordHash, req.Password) {
	if encoder.NeedsRehash(user.PasswordHash) {
		if hash, err := encoder.GetPasswordHash(req.Password); err == nil {
			user.PasswordHash = hash
			_ = repo.Update(user)
		}
	}
	// ... issue tokens
}
```

Users move to the new format as they sign in, with no password reset. Keep `PBKDF2_ENCODER_SECRET` set until no legacy hashes remain.

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