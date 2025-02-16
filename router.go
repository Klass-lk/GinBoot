package ginboot

import (
	"errors"
	"net/http"
	"path"
	"reflect"

	"github.com/gin-gonic/gin"
)

// ControllerGroup represents a group of routes with common middleware and path prefix
type ControllerGroup struct {
	group       *gin.RouterGroup
	fileService FileService
}

// Controller interface defines methods that controllers must implement
type Controller interface {
	Register(group *ControllerGroup)
}

// Group creates a new route group with the given path and middleware
func (s *Server) Group(relativePath string, middleware ...gin.HandlerFunc) *ControllerGroup {
	fullPath := path.Join(s.basePath, relativePath)
	return &ControllerGroup{
		group:       s.engine.Group(fullPath, middleware...),
		fileService: s.fileService,
	}
}

// Internal handler wrapper
func wrapHandler(handler interface{}, service FileService) gin.HandlerFunc {
	handlerType := reflect.TypeOf(handler)
	if handlerType.Kind() != reflect.Func {
		panic("handler must be a function")
	}

	numIn := handlerType.NumIn()
	numOut := handlerType.NumOut()

	if numOut != 2 {
		panic("handler must return (response, error)")
	}

	// Validate error type
	if !handlerType.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		panic("second return value must be error")
	}

	return func(c *gin.Context) {
		ctx := NewContext(c, service)

		// Prepare arguments based on handler signature
		var args []reflect.Value

		switch numIn {
		case 0: // func() (Response, error)
			args = []reflect.Value{}

		case 1: // func(*Context) (Response, error) or func(Request) (Response, error)
			firstArg := handlerType.In(0)
			if firstArg == reflect.TypeOf(&Context{}) {
				// Handler wants context
				args = []reflect.Value{reflect.ValueOf(ctx)}
			} else {
				// Handler wants request
				reqValue := reflect.New(firstArg)
				if err := ctx.GetRequest(reqValue.Interface()); err != nil {
					ctx.SendError(err)
					return
				}
				args = []reflect.Value{reqValue.Elem()}
			}

		case 2: // func(*Context, Request) (Response, error)
			if handlerType.In(0) != reflect.TypeOf(&Context{}) {
				panic("first argument must be *Context when using two arguments")
			}
			reqType := handlerType.In(1)
			reqValue := reflect.New(reqType)
			if err := ctx.GetRequest(reqValue.Interface()); err != nil {
				ctx.SendError(err)
				return
			}
			args = []reflect.Value{reflect.ValueOf(ctx), reqValue.Elem()}

		default:
			panic("handler must have 0-2 arguments")
		}

		// Call handler
		results := reflect.ValueOf(handler).Call(args)

		// Check error
		if !results[1].IsNil() {
			err := results[1].Interface().(error)
			var apiErr ApiError
			if errors.As(err, &apiErr) {
				ctx.SendError(apiErr)
				return
			}
			ctx.SendError(err)
			return
		}

		// Send response
		response := results[0].Interface()
		if response != nil {
			ctx.JSON(http.StatusOK, response)
		} else {
			ctx.Status(http.StatusOK)
		}
	}
}

// RegisterController registers a controller with the given path
func (s *Server) RegisterController(path string, controller Controller) {
	group := s.Group(path)
	controller.Register(group)
}

// Handle wraps gin handler to use custom context
func (g *ControllerGroup) Handle(httpMethod, relativePath string, handler interface{}, middleware ...gin.HandlerFunc) {
	wrappedHandler := wrapHandler(handler, g.fileService)
	handlers := append(middleware, wrappedHandler)
	g.group.Handle(httpMethod, relativePath, handlers...)
}

// GET registers a GET handler
func (g *ControllerGroup) GET(path string, handler interface{}, middleware ...gin.HandlerFunc) {
	g.Handle("GET", path, handler, middleware...)
}

// POST registers a POST handler
func (g *ControllerGroup) POST(path string, handler interface{}, middleware ...gin.HandlerFunc) {
	g.Handle("POST", path, handler, middleware...)
}

// PUT registers a PUT handler
func (g *ControllerGroup) PUT(path string, handler interface{}, middleware ...gin.HandlerFunc) {
	g.Handle("PUT", path, handler, middleware...)
}

// DELETE registers a DELETE handler
func (g *ControllerGroup) DELETE(path string, handler interface{}, middleware ...gin.HandlerFunc) {
	g.Handle("DELETE", path, handler, middleware...)
}

// PATCH registers a PATCH route
func (g *ControllerGroup) PATCH(relativePath string, handler interface{}, middleware ...gin.HandlerFunc) {
	g.Handle("PATCH", relativePath, handler, middleware...)
}

// OPTIONS registers an OPTIONS route
func (g *ControllerGroup) OPTIONS(relativePath string, handler interface{}, middleware ...gin.HandlerFunc) {
	g.Handle("OPTIONS", relativePath, handler, middleware...)
}

// HEAD registers a HEAD route
func (g *ControllerGroup) HEAD(relativePath string, handler interface{}, middleware ...gin.HandlerFunc) {
	g.Handle("HEAD", relativePath, handler, middleware...)
}

// Group creates a new sub-group with the given path and middleware
func (g *ControllerGroup) Group(relativePath string, middleware ...gin.HandlerFunc) *ControllerGroup {
	return &ControllerGroup{
		group:       g.group.Group(relativePath, middleware...),
		fileService: g.fileService,
	}
}

// Use adds middleware to the group
func (g *ControllerGroup) Use(middleware ...gin.HandlerFunc) {
	g.group.Use(middleware...)
}
