package ginboot

import (
	"path"

	"github.com/gin-gonic/gin"
)

// ControllerGroup represents a group of routes with common middleware and path prefix
type ControllerGroup struct {
	group *gin.RouterGroup
}

// Controller interface defines methods that controllers must implement
type Controller interface {
	Register(group *ControllerGroup)
}

// Group creates a new route group with the given path and middleware
func (s *Server) Group(relativePath string, middleware ...gin.HandlerFunc) *ControllerGroup {
	fullPath := path.Join(s.basePath, relativePath)
	return &ControllerGroup{
		group: s.engine.Group(fullPath, middleware...),
	}
}

// Handle wraps gin handler to use custom context
func (g *ControllerGroup) Handle(httpMethod, relativePath string, handler func(*Context), middleware ...gin.HandlerFunc) {
	wrappedHandler := func(c *gin.Context) {
		ctx := NewContext(c)
		handler(ctx)
	}
	handlers := append(middleware, wrappedHandler)
	g.group.Handle(httpMethod, relativePath, handlers...)
}

// GET registers a GET route
func (g *ControllerGroup) GET(relativePath string, handler func(*Context), middleware ...gin.HandlerFunc) {
	g.Handle("GET", relativePath, handler, middleware...)
}

// POST registers a POST route
func (g *ControllerGroup) POST(relativePath string, handler func(*Context), middleware ...gin.HandlerFunc) {
	g.Handle("POST", relativePath, handler, middleware...)
}

// PUT registers a PUT route
func (g *ControllerGroup) PUT(relativePath string, handler func(*Context), middleware ...gin.HandlerFunc) {
	g.Handle("PUT", relativePath, handler, middleware...)
}

// DELETE registers a DELETE route
func (g *ControllerGroup) DELETE(relativePath string, handler func(*Context), middleware ...gin.HandlerFunc) {
	g.Handle("DELETE", relativePath, handler, middleware...)
}

// PATCH registers a PATCH route
func (g *ControllerGroup) PATCH(relativePath string, handler func(*Context), middleware ...gin.HandlerFunc) {
	g.Handle("PATCH", relativePath, handler, middleware...)
}

// OPTIONS registers an OPTIONS route
func (g *ControllerGroup) OPTIONS(relativePath string, handler func(*Context), middleware ...gin.HandlerFunc) {
	g.Handle("OPTIONS", relativePath, handler, middleware...)
}

// HEAD registers a HEAD route
func (g *ControllerGroup) HEAD(relativePath string, handler func(*Context), middleware ...gin.HandlerFunc) {
	g.Handle("HEAD", relativePath, handler, middleware...)
}

// Group creates a new sub-group with the given path and middleware
func (g *ControllerGroup) Group(relativePath string, middleware ...gin.HandlerFunc) *ControllerGroup {
	return &ControllerGroup{
		group: g.group.Group(relativePath, middleware...),
	}
}

// Use adds middleware to the group
func (g *ControllerGroup) Use(middleware ...gin.HandlerFunc) {
	g.group.Use(middleware...)
}

// RegisterController registers a controller with the group at the specified path
func (s *Server) RegisterController(relativePath string, controller Controller) {
	fullPath := path.Join(s.basePath, relativePath)
	controller.Register(&ControllerGroup{group: s.engine.Group(fullPath)})
}

// RegisterControllers registers multiple controllers at the specified path
func (s *Server) RegisterControllers(relativePath string, controllers ...Controller) {
	for _, controller := range controllers {
		s.RegisterController(relativePath, controller)
	}
}
