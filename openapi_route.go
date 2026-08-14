package ginboot

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// The specification is built as routes are registered, so a running server
// already knows it. Serving it is what lets whoever needs it — a console, a
// client generator, a developer — ask the application that is actually running.
//
// The alternative, and what this replaces, is exporting the specification at
// build time by running the application with GINBOOT_EXPORT_SWAGGER set. That
// runs an application's main on a build machine, where there is no database, no
// cache and no secrets — so every application has to notice it is being built
// and take a different path through its own startup. A build concern leaking
// into user code, and one that fails as a hang rather than an error when an
// application does not know to handle it.
const (
	// OpenAPIPublic serves the specification to anyone who asks. The default,
	// because a developer running locally wants it and has nothing to protect.
	OpenAPIPublic = "public"
	// OpenAPIToken serves it only to a caller presenting the shared secret.
	OpenAPIToken = "token"
	// OpenAPIDisabled does not register the route at all, so the path is a 404
	// like any other unrouted path and nothing advertises that a specification
	// exists.
	OpenAPIDisabled = "disabled"

	defaultOpenAPIPath = "/openapi.json"

	// OpenAPITokenHeader carries the shared secret. A dedicated header rather
	// than Authorization, so presenting it cannot be confused with — or logged
	// alongside — an end user's own credentials.
	OpenAPITokenHeader = "X-Ginboot-OpenAPI-Token"
)

// registerOpenAPIRoute mounts the specification endpoint according to
// configuration. Called from Start, beside the health routes.
func (s *Server) registerOpenAPIEndpoint() {
	if s.config == nil {
		return
	}
	cfg := s.config.Ginboot.OpenAPI

	access := strings.ToLower(strings.TrimSpace(cfg.Access))
	if access == "" {
		access = OpenAPIPublic
	}

	if access == OpenAPIDisabled {
		return
	}

	// Fail closed. "token" with no token is a configuration that was started and
	// not finished, and the safe reading of it is that someone meant to restrict
	// the specification — not that they meant to publish it.
	if access == OpenAPIToken && cfg.Token == "" {
		fmt.Println("[ginboot] openapi.access is \"token\" but no token is set; " +
			"not serving the specification. Set GINBOOT_OPENAPI_TOKEN.")
		return
	}

	if access != OpenAPIPublic && access != OpenAPIToken {
		fmt.Printf("[ginboot] openapi.access %q is not one of %q, %q or %q; not serving the specification\n",
			cfg.Access, OpenAPIPublic, OpenAPIToken, OpenAPIDisabled)
		return
	}

	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		path = defaultOpenAPIPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	token := cfg.Token
	s.engine.GET(path, func(c *gin.Context) {
		if access == OpenAPIToken {
			// Constant time, because a comparison that returns early leaks the
			// secret one byte at a time to anyone willing to measure.
			presented := c.GetHeader(OpenAPITokenHeader)
			if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "the OpenAPI specification requires a token",
				})
				return
			}
		}
		c.JSON(http.StatusOK, openApiSpec)
	})
}
