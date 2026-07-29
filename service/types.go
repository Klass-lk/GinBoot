package service

import (
	"context"
	"time"
)

// ServiceEndpoint represents the target protocol and address for a service invocation.
type ServiceEndpoint struct {
	Protocol string            `json:"protocol"` // e.g. "http", "aws-lambda", "grpc"
	Target   string            `json:"target"`   // e.g. "http://user-service:8080" or "https://..."
	Timeout  time.Duration     `json:"timeout"`
	Headers  map[string]string `json:"headers"` // Additional transport headers
}

// ServiceRequest encapsulates all details required for a service-to-service call.
type ServiceRequest struct {
	ServiceName string                 `json:"service_name"`
	Action      string                 `json:"action"` // HTTP path (e.g. "/api/v1/users") or event key
	Method      string                 `json:"method"` // HTTP method (GET, POST, PUT, DELETE, etc.)
	Headers     map[string]string      `json:"headers"`
	Payload     interface{}            `json:"payload"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ServiceResponse encapsulates the result of a service invocation.
type ServiceResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

// ServiceResolver maps logical service names to physical ServiceEndpoints.
type ServiceResolver interface {
	ResolveEndpoint(serviceName string) (ServiceEndpoint, error)
}

// ServiceTransport executes network requests over a specific protocol.
type ServiceTransport interface {
	// Protocol returns the protocol identifier (e.g., "http")
	Protocol() string

	// Call performs a synchronous request-response call
	Call(ctx context.Context, endpoint ServiceEndpoint, req ServiceRequest) (*ServiceResponse, error)

	// CallAsync performs a non-blocking fire-and-forget call
	CallAsync(ctx context.Context, endpoint ServiceEndpoint, req ServiceRequest) error
}

// ServiceClient is the main facade for making service-to-service calls.
type ServiceClient interface {
	Call(ctx context.Context, serviceName string, action string, payload interface{}, target interface{}) error
	CallWithMethod(ctx context.Context, method string, serviceName string, action string, payload interface{}, target interface{}) error
	CallAsync(ctx context.Context, serviceName string, action string, payload interface{}) error
	CallAsyncWithMethod(ctx context.Context, method string, serviceName string, action string, payload interface{}) error
	RegisterTransport(transport ServiceTransport)
	SetResolver(resolver ServiceResolver)
}
