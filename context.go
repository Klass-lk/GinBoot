package ginboot

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/service"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type AuthContext struct {
	UserID    string
	UserEmail string
	Roles     []string
	Claims    map[string]interface{}
}

type Context struct {
	*gin.Context
	fileService   FileService
	logger        Logger
	serviceClient service.ServiceClient
}

func NewContext(c *gin.Context, fileService FileService, logger Logger, serviceClient service.ServiceClient) *Context {
	return &Context{
		Context:       c,
		fileService:   fileService,
		logger:        logger,
		serviceClient: serviceClient,
	}
}

func (c *Context) GetFileService() FileService {
	return c.fileService
}

// Logger returns a Logger bound to the current request context.
func (c *Context) Logger() Logger {
	if c.logger != nil {
		return c.logger.WithContext(c.Request.Context())
	}
	// Fallback if no logger was registered
	return NewSlogLogger(nil).WithContext(c.Request.Context())
}

// GetAuthContext returns the current auth context
func (c *Context) GetAuthContext() (AuthContext, error) {
	userId, exists := c.Get("user_id")
	if !exists {
		c.AbortWithStatus(http.StatusUnauthorized)
		return AuthContext{}, errors.New("operation not permitted")
	}
	role, exists := c.Get("role")
	if !exists {
		c.AbortWithStatus(http.StatusUnauthorized)
		return AuthContext{}, errors.New("operation not permitted")
	}
	return AuthContext{
		UserID: userId.(string),
		Roles:  []string{role.(string)},
	}, nil
}

func (c *Context) GetRequest(request interface{}) error {
	if err := c.ShouldBind(request); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return ApiError{
			ErrorCode: "BAD_REQUEST",
			Message:   "bad request: " + err.Error(),
		}
	}
	return nil
}

func (c *Context) GetPageRequest() PageRequest {
	pageString := c.DefaultQuery("page", "1")
	sizeString := c.DefaultQuery("size", "10")
	sortString := c.DefaultQuery("sort", "_id,asc")
	page, err := strconv.ParseInt(pageString, 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
	}
	size, err := strconv.ParseInt(sizeString, 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
	}
	sortSplit := strings.Split(sortString, ",")
	var sort SortField
	if len(sortSplit) > 1 {
		direction := 1
		if sortSplit[1] == "desc" {
			direction = -1
		}
		sort = SortField{
			Field:     sortSplit[0],
			Direction: direction,
		}
	} else {
		sort = SortField{
			Field:     sortSplit[0],
			Direction: 1,
		}
	}

	return PageRequest{Page: int(page), Size: int(size), Sort: sort}
}

// Span returns the current OpenTelemetry span from the request context.
func (c *Context) Span() trace.Span {
	return trace.SpanFromContext(c.Request.Context())
}

// RecordError records an error on the current span and sets the span status to Error.
func (c *Context) RecordError(err error) {
	if err == nil {
		return
	}
	span := c.Span()
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func (c *Context) SendError(err error) {
	c.RecordError(err)
	var customErr ApiError
	if errors.As(err, &customErr) {
		statusCode := http.StatusBadRequest
		if code, convErr := strconv.Atoi(customErr.ErrorCode); convErr == nil && code >= 400 && code < 600 {
			statusCode = code
		}
		c.JSON(statusCode, gin.H{
			"error_code": customErr.ErrorCode,
			"message":    customErr.Message,
		})
		return
	}
	// Handle other types of errors here
	c.JSON(http.StatusInternalServerError, gin.H{
		"error_code": "Internal Server Error",
		"message":    err.Error(),
	})
}

// ServiceClient returns the configured ServiceClient instance
func (c *Context) ServiceClient() service.ServiceClient {
	if c.serviceClient != nil {
		return c.serviceClient
	}
	return service.NewServiceClient(nil)
}

// CallService performs a synchronous request-reply service-to-service call
func (c *Context) CallService(serviceName string, action string, payload interface{}, target interface{}) error {
	reqCtx := c.Request.Context()
	return c.ServiceClient().Call(reqCtx, serviceName, action, payload, target)
}

// CallServiceAsync performs a non-blocking fire-and-forget service-to-service call
func (c *Context) CallServiceAsync(serviceName string, action string, payload interface{}) error {
	reqCtx := c.Request.Context()
	return c.ServiceClient().CallAsync(reqCtx, serviceName, action, payload)
}

// CallServiceWithMethod performs a synchronous service call with a specific HTTP method
func (c *Context) CallServiceWithMethod(method string, serviceName string, action string, payload interface{}, target interface{}) error {
	reqCtx := c.Request.Context()
	return c.ServiceClient().CallWithMethod(reqCtx, method, serviceName, action, payload, target)
}

// CallServiceAsyncWithMethod performs an async service call with a specific HTTP method
func (c *Context) CallServiceAsyncWithMethod(method string, serviceName string, action string, payload interface{}) error {
	reqCtx := c.Request.Context()
	return c.ServiceClient().CallAsyncWithMethod(reqCtx, method, serviceName, action, payload)
}
