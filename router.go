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

// GET registers a GET route
func (g *ControllerGroup) GET(relativePath string, handler gin.HandlerFunc, middleware ...gin.HandlerFunc) {
	handlers := append(middleware, handler)
	g.group.GET(relativePath, handlers...)
}

// POST registers a POST route
func (g *ControllerGroup) POST(relativePath string, handler gin.HandlerFunc, middleware ...gin.HandlerFunc) {
	handlers := append(middleware, handler)
	g.group.POST(relativePath, handlers...)
}

// PUT registers a PUT route
func (g *ControllerGroup) PUT(relativePath string, handler gin.HandlerFunc, middleware ...gin.HandlerFunc) {
	handlers := append(middleware, handler)
	g.group.PUT(relativePath, handlers...)
}

// DELETE registers a DELETE route
func (g *ControllerGroup) DELETE(relativePath string, handler gin.HandlerFunc, middleware ...gin.HandlerFunc) {
	handlers := append(middleware, handler)
	g.group.DELETE(relativePath, handlers...)
}

// PATCH registers a PATCH route
func (g *ControllerGroup) PATCH(relativePath string, handler gin.HandlerFunc, middleware ...gin.HandlerFunc) {
	handlers := append(middleware, handler)
	g.group.PATCH(relativePath, handlers...)
}

// OPTIONS registers an OPTIONS route
func (g *ControllerGroup) OPTIONS(relativePath string, handler gin.HandlerFunc, middleware ...gin.HandlerFunc) {
	handlers := append(middleware, handler)
	g.group.OPTIONS(relativePath, handlers...)
}

// HEAD registers a HEAD route
func (g *ControllerGroup) HEAD(relativePath string, handler gin.HandlerFunc, middleware ...gin.HandlerFunc) {
	handlers := append(middleware, handler)
	g.group.HEAD(relativePath, handlers...)
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
