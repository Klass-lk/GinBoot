package ginboot

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// Route describes one operation of a service contract: the HTTP method and the
// path it is served on, relative to the contract's BasePath. Paths use gin
// syntax for parameters (":id", "*rest").
type Route struct {
	Method string
	Path   string
}

// Contract is the shared description of a service's API. It is declared once,
// in a package both the owning service and its consumers import, alongside the
// Go interface it describes and the request/response types that interface uses.
//
// Routes maps interface method names to their routes, which is what keeps the
// two sides from drifting: the owning service registers its routes *from* this
// table (see Server.RegisterContract), and consumers call through it (see
// Invoke). Renaming a path is then a single edit that both sides pick up, and
// a method that exists in the interface but not in the table — or in the table
// but not on the implementation — fails at startup rather than in production.
type Contract struct {
	// Service is the logical name resolved by service.ServiceResolver, e.g.
	// "user-service". Consumers use it; the owning service ignores it.
	Service string

	// BasePath is the path prefix all routes hang off, e.g. "/api/v1/users".
	BasePath string

	// Routes maps interface method name -> route.
	Routes map[string]Route
}

// Lookup returns the route registered for an interface method.
func (c Contract) Lookup(method string) (Route, error) {
	route, ok := c.Routes[method]
	if !ok {
		return Route{}, fmt.Errorf("contract %q has no route for method %q", c.Service, method)
	}
	return route, nil
}

// RouteMiddleware attaches gin middleware to individual contract methods, keyed
// by interface method name. Middleware stays on the server side deliberately:
// the contract package is imported by consumers, and authorization is not
// theirs to see or to depend on.
type RouteMiddleware map[string][]gin.HandlerFunc

// contextKey carries the request-scoped *Context through a plain
// context.Context, so a contract implementation written against
// context.Context can still reach request state when it happens to be running
// in-process behind an HTTP handler.
type contextKey struct{}

// FromContext returns the *Context behind a contract call, if the call arrived
// over HTTP in this process. It returns false for a direct in-process call and
// for the calling side of a remote call.
func FromContext(ctx context.Context) (*Context, bool) {
	c, ok := ctx.Value(contextKey{}).(*Context)
	return c, ok
}

// AuthFrom returns the authenticated caller behind a contract call. Unlike
// Context.GetAuthContext it never aborts the request — a contract
// implementation may legitimately run with no HTTP request behind it.
func AuthFrom(ctx context.Context) (AuthContext, bool) {
	c, ok := FromContext(ctx)
	if !ok {
		return AuthContext{}, false
	}
	userID, exists := c.Get("user_id")
	if !exists {
		return AuthContext{}, false
	}
	auth := AuthContext{}
	if s, ok := userID.(string); ok {
		auth.UserID = s
	}
	if role, exists := c.Get("role"); exists {
		if s, ok := role.(string); ok {
			auth.Roles = []string{s}
		}
	}
	return auth, true
}

// RegisterContract serves a contract from impl, mounting it at the contract's
// BasePath. impl must have a method for every entry in contract.Routes, each
// shaped func(context.Context, Request) (Response, error) or
// func(context.Context) (Response, error) — the same shape the shared
// interface declares, so `var _ userapi.UserService = (*UserController)(nil)`
// is all it takes to check the implementation against the contract at compile
// time.
//
// A missing or mis-shaped method panics at registration: contract mismatches
// belong at startup, not in a 404 under load.
func (s *Server) RegisterContract(contract Contract, impl any, middleware ...RouteMiddleware) {
	group := s.Group(contract.BasePath)
	group.registerContract(contract, impl, "", middleware...)
}

// RegisterContract serves a contract from impl on an existing group, so the
// group's own prefix and middleware (auth, tenancy) apply to every route. The
// contract's BasePath is appended to the group's prefix.
func (g *ControllerGroup) RegisterContract(contract Contract, impl any, middleware ...RouteMiddleware) {
	g.registerContract(contract, impl, contract.BasePath, middleware...)
}

func (g *ControllerGroup) registerContract(contract Contract, impl any, prefix string, middleware ...RouteMiddleware) {
	implValue := reflect.ValueOf(impl)
	if !implValue.IsValid() {
		panic(fmt.Sprintf("ginboot: contract %q registered with a nil implementation", contract.Service))
	}

	perRoute := RouteMiddleware{}
	for _, mw := range middleware {
		for name, handlers := range mw {
			perRoute[name] = append(perRoute[name], handlers...)
		}
	}
	for name := range perRoute {
		if _, ok := contract.Routes[name]; !ok {
			panic(fmt.Sprintf("ginboot: contract %q has middleware for method %q, which is not in the contract", contract.Service, name))
		}
	}

	// Sorted so route registration order is stable across runs.
	names := make([]string, 0, len(contract.Routes))
	for name := range contract.Routes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		route := contract.Routes[name]

		method := implValue.MethodByName(name)
		if !method.IsValid() {
			panic(fmt.Sprintf("ginboot: contract %q requires method %s on %T, which does not implement it",
				contract.Service, name, impl))
		}

		reqType, resType, err := contractMethodShape(method.Type())
		if err != nil {
			panic(fmt.Sprintf("ginboot: contract %q method %s: %v", contract.Service, name, err))
		}

		httpMethod := strings.ToUpper(route.Method)
		relPath := path.Join(prefix, route.Path)
		if relPath == "" {
			relPath = "/"
		}

		registerOpenAPIRoute(httpMethod, path.Join(g.group.BasePath(), relPath), openAPIShape(reqType, resType))

		handlers := append(append([]gin.HandlerFunc{}, perRoute[name]...),
			g.contractHandler(method, reqType))
		g.group.Handle(httpMethod, relPath, handlers...)
	}
}

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// contractMethodShape validates a contract method signature and reports its
// request type (nil when the method takes none) and response type.
func contractMethodShape(t reflect.Type) (reqType, resType reflect.Type, err error) {
	if t.NumOut() != 2 || !t.Out(1).Implements(errorType) {
		return nil, nil, fmt.Errorf("must return (response, error)")
	}
	switch t.NumIn() {
	case 1:
		if t.In(0) != contextType {
			return nil, nil, fmt.Errorf("first argument must be context.Context, got %s", t.In(0))
		}
		return nil, t.Out(0), nil
	case 2:
		if t.In(0) != contextType {
			return nil, nil, fmt.Errorf("first argument must be context.Context, got %s", t.In(0))
		}
		return t.In(1), t.Out(0), nil
	default:
		return nil, nil, fmt.Errorf("must take (context.Context) or (context.Context, request)")
	}
}

// openAPIShape synthesises the handler signature the OpenAPI collector expects,
// so contract routes land in the exported spec next to hand-registered ones.
func openAPIShape(reqType, resType reflect.Type) reflect.Type {
	in := []reflect.Type{reflect.TypeOf(&Context{})}
	if reqType != nil {
		in = append(in, reqType)
	}
	return reflect.FuncOf(in, []reflect.Type{resType, errorType}, false)
}

func (g *ControllerGroup) contractHandler(method reflect.Value, reqType reflect.Type) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := NewContext(c, g.fileService, g.logger, g.serviceClient)

		// The request context, not the *Context, so deadlines and the active
		// span propagate; the *Context rides along as a value for handlers
		// that need request state (see FromContext / AuthFrom).
		callCtx := context.WithValue(c.Request.Context(), contextKey{}, ctx)

		args := []reflect.Value{reflect.ValueOf(callCtx)}
		if reqType != nil {
			reqValue := reflect.New(reqType)
			if err := bindContractRequest(ctx, reqValue.Interface()); err != nil {
				ctx.SendError(err)
				return
			}
			args = append(args, reqValue.Elem())
		}

		results := method.Call(args)

		if !results[1].IsNil() {
			ctx.SendError(results[1].Interface().(error))
			return
		}
		if c.Writer.Written() {
			return
		}

		response := results[0].Interface()
		if response == nil {
			ctx.Status(http.StatusOK)
			return
		}
		if str, ok := response.(string); ok {
			ctx.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(str))
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}

// bindContractRequest fills a contract request from the HTTP request. Path
// parameters are bound last so a ":id" in the URL always wins over an id in
// the body — the URL is the one the router matched on.
func bindContractRequest(ctx *Context, req any) error {
	if hasBody(ctx.Request.Method) {
		if ctx.Request.ContentLength != 0 {
			if err := ctx.ShouldBindJSON(req); err != nil {
				return badRequest(err)
			}
		}
	} else if err := ctx.ShouldBindQuery(req); err != nil {
		return badRequest(err)
	}

	if len(ctx.Params) > 0 {
		if err := ctx.ShouldBindUri(req); err != nil {
			return badRequest(err)
		}
	}
	return nil
}

func hasBody(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func badRequest(err error) ApiError {
	return ApiError{ErrorCode: "BAD_REQUEST", Message: "bad request: " + err.Error()}
}
